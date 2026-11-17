#!/usr/bin/env python3
"""Loopback-only TCP forwarder for the Windows<->WSL 5GC acceptance.

OPTIONAL acceptance helper, not a product dependency
====================================================

This script exists only to support the WSL2 *mirrored-networking* acceptance
topology. The OpenAMSM NFs listen on loopback aliases inside WSL (for example
``10.30.20.4:7777``). With WSL2 in mirrored networking mode Windows and WSL
share ``127.0.0.1``, but the WSL loopback aliases are not visible to Windows.
This forwarder exposes exactly the required services on loopback-only ports:

    WSL 127.0.0.1:17777  ->  SMF SBI      (default 10.30.20.4:7777)
    WSL 127.0.0.1:19090  ->  AMF metrics  (default 10.30.20.5:9090)
    WSL 127.0.0.1:29090  ->  SMF metrics  (default 10.30.20.4:9090)

The product/client configuration must not contain any WSL field. Without WSL
(or without mirrored networking) simply configure a real remote SBI origin;
this helper is never part of the shipped deployment.

Security boundary -- hardcoded loopback listener
================================================

The listener address is the module constant ``LISTEN_HOST = "127.0.0.1"``.
There is deliberately **no** CLI flag, environment variable or configuration
file that can change it. The forwarder's job is to cross the WSL/Windows
loopback boundary in mirrored mode; binding anything else would widen the
network exposure of the acceptance host:

  * ``0.0.0.0`` / ``::``             -- wildcard, reachable from the LAN;
  * ``::1``                          -- not used by this mirrored-networking
                                        topology and not the documented token;
  * ``192.168.x.x`` / any routed IP  -- LAN/routed exposure;
  * any non-loopback IPv4/IPv6       -- out of scope.

As defense in depth ``validate_listen_host`` rejects every address other than
``127.0.0.1`` *before* any listener is created, so a future accidental
``--bind``-style mutation (or a monkeypatched constant) fails closed with a
clear non-zero exit and no open socket. ``main_async`` also refuses to run
unless the service table is exactly the three declared acceptance services;
there is no fourth listener and no general proxy behavior.

Implementation notes:

* Python 3 standard library only; no Docker, no iptables/nftables, no
  Windows Firewall or netsh changes.
* It binds loopback only, so it does not widen the network exposure.
* It is byte-transparent (no HTTP parsing, no TLS termination), so the SBI
  HTTP/2 prior-knowledge (h2c) stream passes through unchanged.
* The listen ports can be overridden with CLI flags; the wrapper script
  ``wsl-tcp-forward.sh`` passes the documented ``CA_WSL_FORWARD_*_PORT``
  environment variables (used by the lifecycle test so the live acceptance
  forwarder on the default ports is never disturbed).

Usage:
    python3 wsl-tcp-forward.py --pidfile <path> [--logfile <path>]
        [--sbi-listen 17777] [--metrics-amf-listen 19090]
        [--metrics-smf-listen 29090]
        [--smf-sbi host:port] [--amf-metrics host:port] [--smf-metrics host:port]

Exit codes: 0 clean stop, 2 configuration/bind-rejection failure.
"""

import argparse
import asyncio
import ipaddress
import os
import signal
import sys
import time

#: The one and only permitted listener address. See the security boundary
#: note in the module docstring: mirrored-networking acceptance only, loopback
#: exposure only. Never make this configurable.
#:
#: ``_ALLOWED_LISTEN_HOST`` is the immutable literal used by the validation
#: comparison; validation never trusts the mutable ``LISTEN_HOST`` symbol as
#: its own reference point (that would make the check circular and would allow
#: a mutated constant to widen the bind).
_ALLOWED_LISTEN_HOST = "127.0.0.1"
LISTEN_HOST = _ALLOWED_LISTEN_HOST

#: Exactly the three declared acceptance services. There is no fourth
#: listener and no general proxy behavior.
SERVICE_SPECS = (
    ("smf-sbi", 17777, "SMF SBI h2c"),
    ("amf-metrics", 19090, "AMF metrics"),
    ("smf-metrics", 29090, "SMF metrics"),
)
EXPECTED_SERVICES = tuple(name for name, _port, _description in SERVICE_SPECS)

DEFAULT_TARGETS = {
    "smf-sbi": "10.30.20.4:7777",
    "amf-metrics": "10.30.20.5:9090",
    "smf-metrics": "10.30.20.4:9090",
}
DEFAULT_PORTS = {name: port for name, port, _description in SERVICE_SPECS}

_conn_counter = 0


def log(message: str) -> None:
    print(f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {message}", flush=True)


def validate_listen_host(host) -> str:
    """Return the accepted loopback listener address or raise ValueError.

    Defense in depth for the listening security boundary: only the literal
    ``127.0.0.1`` is accepted. Wildcard, IPv6 (including ``::1``), LAN/routed
    IPv4 and every other address are rejected before any socket is opened.
    """
    candidate = host.strip() if isinstance(host, str) else ""
    if candidate == _ALLOWED_LISTEN_HOST:
        return _ALLOWED_LISTEN_HOST

    try:
        address = ipaddress.ip_address(candidate)
    except ValueError:
        detail = "not an IP literal (only 127.0.0.1 is accepted)"
    else:
        if address.version == 4 and address.is_unspecified:
            detail = "wildcard IPv4 would expose the acceptance host on every interface"
        elif address.version == 6 and address.is_unspecified:
            detail = "wildcard IPv6 would expose the acceptance host on every interface"
        elif address.version == 6 and address.is_loopback:
            detail = "IPv6 loopback is not used by the mirrored-networking acceptance"
        elif address.is_loopback:
            detail = "only 127.0.0.1 is exposed; other loopback forms are not accepted"
        elif address.version == 4:
            detail = "non-loopback IPv4 would expose the acceptance host to the network"
        else:
            detail = "non-loopback IPv6 would expose the acceptance host to the network"
    raise ValueError(f"refusing listener address {host!r}: {detail}")


def parse_target(raw: str) -> tuple:
    host, separator, port = raw.strip().rpartition(":")
    if not separator or not host or not port.isdigit():
        raise ValueError(f"invalid host:port target {raw!r}")
    port_number = int(port)
    if not 0 < port_number < 65536:
        raise ValueError(f"invalid port in target {raw!r}")
    return host, port_number


async def _pipe(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
    try:
        while True:
            data = await reader.read(65536)
            if not data:
                break
            writer.write(data)
            await writer.drain()
    except (ConnectionError, asyncio.IncompleteReadError, OSError):
        pass
    finally:
        try:
            writer.close()
        except Exception:
            pass


def _make_handler(name: str, dst_host: str, dst_port: int):
    async def handler(creader: asyncio.StreamReader, cwriter: asyncio.StreamWriter):
        global _conn_counter
        _conn_counter += 1
        cid = _conn_counter
        peer = cwriter.get_extra_info("peername")
        log(f"{name} conn#{cid} open from {peer} -> {dst_host}:{dst_port}")
        try:
            upstream_reader, upstream_writer = await asyncio.wait_for(
                asyncio.open_connection(dst_host, dst_port), timeout=5.0
            )
        except Exception as exc:  # upstream unavailable: close the client
            log(f"{name} conn#{cid} upstream {dst_host}:{dst_port} failed: {exc!r}")
            try:
                cwriter.close()
            except Exception:
                pass
            return
        try:
            await asyncio.gather(
                _pipe(creader, upstream_writer),
                _pipe(upstream_reader, cwriter),
            )
        finally:
            for writer in (upstream_writer, cwriter):
                try:
                    writer.close()
                except Exception:
                    pass
            log(f"{name} conn#{cid} closed")

    return handler


async def main_async(mappings: list, pidfile: str) -> int:
    # Fail closed BEFORE creating any listener if the boundary was changed.
    try:
        listen_host = validate_listen_host(LISTEN_HOST)
    except ValueError as exc:
        log(f"FATAL bind rejected: {exc}")
        return 2

    if len(mappings) != len(SERVICE_SPECS) or tuple(m[0] for m in mappings) != EXPECTED_SERVICES:
        log(
            "FATAL forwarder service table is not exactly the three declared "
            f"acceptance services {EXPECTED_SERVICES}"
        )
        return 2

    servers = []
    for name, listen_port, target in mappings:
        dst_host, dst_port = target
        try:
            server = await asyncio.start_server(
                _make_handler(name, dst_host, dst_port),
                host=listen_host,
                port=listen_port,
                reuse_address=True,
            )
        except OSError as exc:
            log(f"FATAL bind {listen_host}:{listen_port} ({name}) failed: {exc!r}")
            for running in servers:
                running.close()
            return 2
        servers.append(server)
        log(f"listening {listen_host}:{listen_port} -> {dst_host}:{dst_port} ({name})")

    pid = os.getpid()
    with open(pidfile, "w", encoding="ascii") as handle:
        handle.write(f"{pid}\n")
    log(f"ready pid={pid} pidfile={pidfile}")

    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        try:
            loop.add_signal_handler(sig, stop.set)
        except NotImplementedError:
            pass

    await stop.wait()
    log("shutdown requested")
    for server in servers:
        server.close()
    await asyncio.sleep(0.2)
    try:
        os.unlink(pidfile)
    except FileNotFoundError:
        pass
    log("stopped")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Loopback-only acceptance forwarder for exactly three OpenAMSM "
            "services. The listener address is hardcoded to 127.0.0.1; there "
            "is no --bind option (mirrored-networking acceptance only)."
        )
    )
    parser.add_argument("--pidfile", required=True)
    parser.add_argument("--logfile", default="")
    # No --bind option: the listener address is the hardcoded LISTEN_HOST
    # constant. CA_WSL_FORWARD_BIND is intentionally not read at all.
    parser.add_argument("--sbi-listen", type=int, default=int(os.environ.get("CA_WSL_FORWARD_SBI_PORT", str(DEFAULT_PORTS["smf-sbi"]))))
    parser.add_argument("--metrics-amf-listen", type=int, default=int(os.environ.get("CA_WSL_FORWARD_AMF_METRICS_PORT", str(DEFAULT_PORTS["amf-metrics"]))))
    parser.add_argument("--metrics-smf-listen", type=int, default=int(os.environ.get("CA_WSL_FORWARD_SMF_METRICS_PORT", str(DEFAULT_PORTS["smf-metrics"]))))
    parser.add_argument("--smf-sbi", default=os.environ.get("CA_WSL_FORWARD_SMF_SBI_TARGET", DEFAULT_TARGETS["smf-sbi"]))
    parser.add_argument("--amf-metrics", default=os.environ.get("CA_WSL_FORWARD_AMF_METRICS_TARGET", DEFAULT_TARGETS["amf-metrics"]))
    parser.add_argument("--smf-metrics", default=os.environ.get("CA_WSL_FORWARD_SMF_METRICS_TARGET", DEFAULT_TARGETS["smf-metrics"]))
    args = parser.parse_args()

    try:
        mappings = [
            ("smf-sbi", args.sbi_listen, parse_target(args.smf_sbi)),
            ("amf-metrics", args.metrics_amf_listen, parse_target(args.amf_metrics)),
            ("smf-metrics", args.metrics_smf_listen, parse_target(args.smf_metrics)),
        ]
    except ValueError as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        return 2

    try:
        validate_listen_host(LISTEN_HOST)
    except ValueError as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        return 2

    if args.logfile:
        handle = open(args.logfile, "a", buffering=1, encoding="utf-8")
        sys.stdout = handle
        sys.stderr = handle
    try:
        return asyncio.run(main_async(mappings, args.pidfile))
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
