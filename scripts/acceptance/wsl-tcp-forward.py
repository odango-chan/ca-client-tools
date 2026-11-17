#!/usr/bin/env python3
"""Minimal loopback-only TCP forwarder for the Windows<->WSL 5GC acceptance.

The OpenAMSM NFs listen on loopback aliases inside WSL (for example
``10.30.20.4:7777``). With WSL2 in mirrored networking mode, Windows and WSL
share ``127.0.0.1``, but the WSL loopback aliases are not visible to Windows.
This forwarder exposes exactly the required services on loopback-only ports:

    WSL 127.0.0.1:17777  ->  SMF SBI      (default 10.30.20.4:7777)
    WSL 127.0.0.1:19090  ->  AMF metrics  (default 10.30.20.5:9090)
    WSL 127.0.0.1:29090  ->  SMF metrics  (default 10.30.20.4:9090)

Implementation notes:

* Python 3 standard library only; no Docker, no iptables/nftables, no
  Windows Firewall or netsh changes.
* It binds loopback only, so it does not widen the network exposure.
* It is byte-transparent (no HTTP parsing, no TLS termination), so the SBI
  HTTP/2 prior-knowledge (h2c) stream passes through unchanged.
* The services/ports can be overridden with CLI flags; the wrapper script
  ``wsl-tcp-forward.sh`` passes the documented environment variables.

Usage:
    python3 wsl-tcp-forward.py --pidfile <path> [--logfile <path>]
        [--bind 127.0.0.1]
        [--sbi-listen 17777] [--metrics-amf-listen 19090] [--metrics-smf-listen 29090]
        [--smf-sbi host:port] [--amf-metrics host:port] [--smf-metrics host:port]
"""

import argparse
import asyncio
import os
import signal
import sys
import time

DEFAULT_TARGETS = {
    "smf-sbi": "10.30.20.4:7777",
    "amf-metrics": "10.30.20.5:9090",
    "smf-metrics": "10.30.20.4:9090",
}
DEFAULT_PORTS = {
    "smf-sbi": 17777,
    "amf-metrics": 19090,
    "smf-metrics": 29090,
}

_conn_counter = 0


def log(message: str) -> None:
    print(f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {message}", flush=True)


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


async def main_async(bind: str, mappings: list, pidfile: str) -> int:
    servers = []
    for name, listen_port, target in mappings:
        dst_host, dst_port = target
        try:
            server = await asyncio.start_server(
                _make_handler(name, dst_host, dst_port),
                host=bind,
                port=listen_port,
                reuse_address=True,
            )
        except OSError as exc:
            log(f"FATAL bind {bind}:{listen_port} ({name}) failed: {exc!r}")
            for running in servers:
                running.close()
            return 2
        servers.append(server)
        log(f"listening {bind}:{listen_port} -> {dst_host}:{dst_port} ({name})")

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
    parser = argparse.ArgumentParser()
    parser.add_argument("--pidfile", required=True)
    parser.add_argument("--logfile", default="")
    parser.add_argument("--bind", default=os.environ.get("CA_WSL_FORWARD_BIND", "127.0.0.1"))
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

    if args.logfile:
        handle = open(args.logfile, "a", buffering=1, encoding="utf-8")
        sys.stdout = handle
        sys.stderr = handle
    try:
        return asyncio.run(main_async(args.bind, mappings, args.pidfile))
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
