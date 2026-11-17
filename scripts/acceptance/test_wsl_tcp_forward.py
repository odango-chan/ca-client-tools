#!/usr/bin/env python3
"""Offline binding-boundary tests for ``wsl-tcp-forward.py``.

Stdlib ``unittest`` only: no network access, no Docker, no WSL required. The
suite spawns the forwarder on ephemeral loopback ports so it never touches the
default acceptance ports (17777/19090/29090) or a live acceptance forwarder.

It proves the PR #3 listening security boundary:

* the listener address is the hardcoded constant ``127.0.0.1``;
* ``--bind`` no longer exists as an option and ``CA_WSL_FORWARD_BIND`` no
  longer influences the bind (setting it to ``0.0.0.0`` still yields exactly
  three ``127.0.0.1`` listeners);
* every non-``127.0.0.1`` address is rejected before a listener is created
  (``0.0.0.0``, non-loopback IPv4, ``::``, ``::1`` and non-loopback IPv6);
* the service table is exactly the three declared services and no fourth
  listener exists.

Run from this directory on Windows or Linux:

    python -m unittest test_wsl_tcp_forward -v
"""

import asyncio
import contextlib
import importlib.util
import os
import pathlib
import re
import socket
import subprocess
import sys
import tempfile
import time
import unittest

HERE = pathlib.Path(__file__).resolve().parent
FORWARDER = HERE / "wsl-tcp-forward.py"
MODULE_NAME = "ca_acceptance_wsl_tcp_forward"

SERVICE_NAMES = ("smf-sbi", "amf-metrics", "smf-metrics")
DEFAULT_PORTS = {"smf-sbi": 17777, "amf-metrics": 19090, "smf-metrics": 29090}


def load_forwarder_module():
    spec = importlib.util.spec_from_file_location(MODULE_NAME, str(FORWARDER))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def allocate_free_ports(count):
    """Reserve ``count`` distinct loopback ports, then release them."""
    holders = []
    ports = []
    try:
        for _ in range(count):
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.bind(("127.0.0.1", 0))
            holders.append(sock)
            ports.append(sock.getsockname()[1])
    finally:
        for sock in holders:
            sock.close()
    return ports


def _split_endpoint(endpoint):
    endpoint = endpoint.strip()
    if endpoint.startswith("["):
        host, _, port = endpoint[1:].partition("]:")
        return host, int(port) if port.isdigit() else None
    host, separator, port = endpoint.rpartition(":")
    if not separator or not port.isdigit():
        return endpoint, None
    return host, int(port)


def _windows_listener_bindings(pid):
    try:
        completed = subprocess.run(
            ["netstat", "-ano", "-p", "tcp"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (OSError, subprocess.SubprocessError) as exc:  # pragma: no cover
        raise unittest.SkipTest(f"netstat unavailable: {exc!r}")
    if completed.returncode != 0:  # pragma: no cover
        raise unittest.SkipTest(f"netstat failed: {completed.returncode}")
    bindings = []
    for line in completed.stdout.splitlines():
        fields = line.split()
        if len(fields) < 5 or fields[0].upper() != "TCP":
            continue
        local, foreign, owner = fields[1], fields[2], fields[4]
        # A listening socket has foreign address 0.0.0.0:0 or [::]:0, which is
        # numeric and therefore independent of the localized State column.
        if not foreign.endswith(":0"):
            continue
        if owner != str(pid):
            continue
        host, port = _split_endpoint(local)
        if port is None:
            continue
        bindings.append((host, port))
    return bindings


def _linux_listener_bindings(pid):
    # /proc/<pid>/net/tcp is namespace-wide, not per-process, so use `ss -p`
    # and filter by the child PID instead.
    try:
        completed = subprocess.run(
            ["ss", "-ltnpH"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (OSError, subprocess.SubprocessError) as exc:  # pragma: no cover
        raise unittest.SkipTest(f"ss unavailable: {exc!r}")
    if completed.returncode != 0:  # pragma: no cover
        raise unittest.SkipTest(f"ss failed: {completed.returncode}")
    marker = f"pid={pid},"
    bindings = []
    for line in completed.stdout.splitlines():
        if marker not in line:
            continue
        fields = line.split()
        if len(fields) < 4:
            continue
        host, port = _split_endpoint(fields[3])
        if port is None:
            continue
        bindings.append((host, port))
    return bindings


def listener_bindings(pid):
    if os.name == "nt":
        return _windows_listener_bindings(pid)
    return _linux_listener_bindings(pid)


def _forwarder_environment(ports, extra_env=None):
    env = os.environ.copy()
    # CA_WSL_FORWARD_BIND must be irrelevant; callers may still set it.
    env.pop("CA_WSL_FORWARD_BIND", None)
    env["CA_WSL_FORWARD_SBI_PORT"] = str(ports[0])
    env["CA_WSL_FORWARD_AMF_METRICS_PORT"] = str(ports[1])
    env["CA_WSL_FORWARD_SMF_METRICS_PORT"] = str(ports[2])
    # Backends are irrelevant for binding: point them at a closed loopback
    # port so no test ever touches a real NF service.
    env["CA_WSL_FORWARD_SMF_SBI_TARGET"] = "127.0.0.1:1"
    env["CA_WSL_FORWARD_AMF_METRICS_TARGET"] = "127.0.0.1:1"
    env["CA_WSL_FORWARD_SMF_METRICS_TARGET"] = "127.0.0.1:1"
    if extra_env:
        env.update(extra_env)
    return env


def _forwarder_command(pidfile, ports):
    return [
        sys.executable,
        "-u",
        str(FORWARDER),
        "--pidfile",
        str(pidfile),
        "--sbi-listen",
        str(ports[0]),
        "--metrics-amf-listen",
        str(ports[1]),
        "--metrics-smf-listen",
        str(ports[2]),
    ]


@contextlib.contextmanager
def running_forwarder(ports, extra_env=None):
    """Start the forwarder on ``ports`` and guarantee cleanup."""
    with tempfile.TemporaryDirectory(prefix="ca-wsl-fwd-test-") as tmp:
        pidfile = pathlib.Path(tmp) / "forwarder.pid"
        logfile = pathlib.Path(tmp) / "forwarder.log"
        command = _forwarder_command(pidfile, ports) + ["--logfile", str(logfile)]
        process = subprocess.Popen(
            command,
            env=_forwarder_environment(ports, extra_env),
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        try:
            deadline = time.monotonic() + 15.0
            while time.monotonic() < deadline:
                if pidfile.exists():
                    break
                if process.poll() is not None:
                    output = process.stdout.read() if process.stdout else ""
                    raise AssertionError(
                        f"forwarder exited early with {process.returncode}: {output}"
                    )
                time.sleep(0.05)
            else:
                raise AssertionError("forwarder did not create its pidfile in 15s")
            recorded = int(pidfile.read_text(encoding="ascii").strip())
            yield process, recorded
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:  # pragma: no cover
                    process.kill()
                    process.wait(timeout=10)
            if process.stdout is not None:
                process.stdout.close()


class ListenHostValidationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_forwarder_module()

    def test_loopback_constant_is_hardcoded_loopback(self):
        self.assertEqual(self.module.LISTEN_HOST, "127.0.0.1")
        self.assertEqual(self.module.validate_listen_host("127.0.0.1"), "127.0.0.1")
        self.assertEqual(self.module.validate_listen_host("  127.0.0.1  "), "127.0.0.1")

    def test_wildcard_ipv4_rejected(self):
        for host in ("0.0.0.0", "0", "0.0.0.0/0"):
            with self.subTest(host=host):
                with self.assertRaises(ValueError):
                    self.module.validate_listen_host(host)

    def test_non_loopback_ipv4_rejected(self):
        for host in ("192.168.10.5", "10.30.20.4", "172.16.0.1", "8.8.8.8"):
            with self.subTest(host=host):
                with self.assertRaises(ValueError):
                    self.module.validate_listen_host(host)

    def test_ipv6_unspecified_and_loopback_rejected(self):
        for host in ("::", "::1", "[::]", "[::1]", "0:0:0:0:0:0:0:1"):
            with self.subTest(host=host):
                with self.assertRaises(ValueError):
                    self.module.validate_listen_host(host)

    def test_non_loopback_ipv6_rejected(self):
        for host in ("2001:db8::1", "fe80::1", "fd00::1"):
            with self.subTest(host=host):
                with self.assertRaises(ValueError):
                    self.module.validate_listen_host(host)

    def test_hostname_and_empty_values_rejected(self):
        for host in ("localhost", "", "   ", None, 123, "127.0.0.1:17777"):
            with self.subTest(host=host):
                with self.assertRaises(ValueError):
                    self.module.validate_listen_host(host)

    def test_exactly_three_declared_services(self):
        self.assertEqual(self.module.EXPECTED_SERVICES, SERVICE_NAMES)
        self.assertEqual(len(self.module.SERVICE_SPECS), 3)
        self.assertEqual(self.module.DEFAULT_PORTS, DEFAULT_PORTS)
        for name, port, _description in self.module.SERVICE_SPECS:
            self.assertEqual(DEFAULT_PORTS[name], port)


class BindEntryPointRemovalTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_forwarder_module()

    def test_bind_flag_is_not_an_option(self):
        with tempfile.TemporaryDirectory(prefix="ca-wsl-fwd-cli-") as tmp:
            pidfile = pathlib.Path(tmp) / "forwarder.pid"
            completed = subprocess.run(
                [
                    sys.executable,
                    str(FORWARDER),
                    "--pidfile",
                    str(pidfile),
                    "--bind",
                    "0.0.0.0",
                ],
                capture_output=True,
                text=True,
                timeout=30,
            )
            self.assertEqual(completed.returncode, 2, completed.stderr)
            self.assertIn("unrecognized arguments", completed.stderr)
            self.assertIn("--bind", completed.stderr)
            self.assertFalse(pidfile.exists())

    def test_help_documents_hardcoded_loopback_and_no_bind(self):
        completed = subprocess.run(
            [sys.executable, str(FORWARDER), "--help"],
            capture_output=True,
            text=True,
            timeout=30,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertIn("127.0.0.1", completed.stdout)
        # The help text may *say* there is no --bind option, but it must not
        # expose one in the option list.
        self.assertIsNone(
            re.search(r"(?m)^\s+--bind\b", completed.stdout),
            "--help must not advertise a --bind option",
        )

    def test_bind_env_var_does_not_influence_bind(self):
        ports = allocate_free_ports(3)
        with running_forwarder(ports, extra_env={"CA_WSL_FORWARD_BIND": "0.0.0.0"}) as (
            process,
            pid,
        ):
            bindings = listener_bindings(pid)
            self.assertEqual(
                sorted(port for _host, port in bindings),
                sorted(ports),
                f"expected exactly the three declared listeners, got {bindings}",
            )
            for host, port in bindings:
                self.assertEqual(host, "127.0.0.1", f"port {port} bound to {host}")

            for port in ports:
                with socket.create_connection(("127.0.0.1", port), timeout=5):
                    pass

    def test_rejected_bind_constant_creates_no_listener(self):
        # Defense in depth: even if the constant is mutated, main_async must
        # fail closed before any listener is created.
        ports = allocate_free_ports(3)
        mappings = [
            ("smf-sbi", ports[0], ("127.0.0.1", 1)),
            ("amf-metrics", ports[1], ("127.0.0.1", 1)),
            ("smf-metrics", ports[2], ("127.0.0.1", 1)),
        ]
        with tempfile.TemporaryDirectory(prefix="ca-wsl-fwd-reject-") as tmp:
            pidfile = pathlib.Path(tmp) / "forwarder.pid"
            original = self.module.LISTEN_HOST
            self.module.LISTEN_HOST = "0.0.0.0"
            try:
                result = asyncio.run(
                    asyncio.wait_for(
                        self.module.main_async(mappings, str(pidfile)), timeout=10
                    )
                )
            finally:
                self.module.LISTEN_HOST = original
            self.assertEqual(result, 2)
            self.assertFalse(pidfile.exists())
            # No socket was left behind: we can bind all three ports ourselves.
            holders = []
            try:
                for port in ports:
                    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                    sock.bind(("127.0.0.1", port))
                    holders.append(sock)
            finally:
                for sock in holders:
                    sock.close()

    def test_service_table_mismatch_fails_closed(self):
        mappings = [
            ("smf-sbi", 0, ("127.0.0.1", 1)),
            ("amf-metrics", 0, ("127.0.0.1", 1)),
            ("smf-metrics", 0, ("127.0.0.1", 1)),
            ("fourth-service", 0, ("127.0.0.1", 1)),
        ]
        with tempfile.TemporaryDirectory(prefix="ca-wsl-fwd-table-") as tmp:
            pidfile = pathlib.Path(tmp) / "forwarder.pid"
            result = asyncio.run(
                asyncio.wait_for(
                    self.module.main_async(mappings, str(pidfile)), timeout=10
                )
            )
            self.assertEqual(result, 2)
            self.assertFalse(pidfile.exists())


class ForwarderListenerTests(unittest.TestCase):
    def test_three_loopback_listeners_and_clean_shutdown(self):
        ports = allocate_free_ports(3)
        with running_forwarder(ports) as (process, pid):
            bindings = listener_bindings(pid)
            self.assertEqual(
                sorted(port for _host, port in bindings),
                sorted(ports),
                f"expected exactly the three declared listeners, got {bindings}",
            )
            self.assertEqual(
                {host for host, _port in bindings},
                {"127.0.0.1"},
                f"all declared listeners must be loopback-only, got {bindings}",
            )
            for port in ports:
                with socket.create_connection(("127.0.0.1", port), timeout=5):
                    pass
        self.assertIsNotNone(process.returncode)


if __name__ == "__main__":
    unittest.main(verbosity=2)
