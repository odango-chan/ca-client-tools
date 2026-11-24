"""Long-lived CA codec worker client.

The worker is the existing ``ca-client-tools codec worker`` process. This module
owns the process lifecycle and the framed stdin/stdout transport. It performs no
protocol decoding itself: JSON and raw binary are passed to the same Go facade
used by native Go consumers.
"""

from __future__ import annotations

import json
import os
import queue
import struct
import subprocess
import threading
import uuid
from dataclasses import dataclass
from typing import Any, Iterable, Mapping

PROTOCOL_VERSION = 1
DEFAULT_MAX_FRAME = 64 << 20
MAX_JSON = 4 << 20


class CodecWorkerError(Exception):
    """Base class for worker transport and process errors."""


class CodecWorkerExited(CodecWorkerError):
    """The worker process exited or closed its output."""


class CodecWorkerTimeout(CodecWorkerError):
    """A request did not complete before its deadline; the worker was stopped."""


class CodecWorkerProtocolError(CodecWorkerError):
    """The worker emitted an invalid frame."""


@dataclass
class WorkerResponse:
    request_id: str
    status: str
    operation: str
    raw: Mapping[str, Any]
    binary: bytes

    @property
    def qualified(self) -> bool:
        return bool(self.raw.get("qualified"))

    @property
    def reencode_eligible(self) -> bool:
        return bool(self.raw.get("reencodeEligible"))

    @property
    def message(self) -> Any:
        return self.raw.get("message")


class CodecWorker:
    """One serial, long-lived codec worker process.

    ``command`` defaults to the installed ``ca-client-tools codec worker``. A
    caller may pass an explicit argv (for tests or deployment wrappers). No
    network endpoint is used.
    """

    def __init__(
        self,
        executable: str | None = None,
        *,
        command: Iterable[str] | None = None,
        max_frame: int = DEFAULT_MAX_FRAME,
        cwd: str | None = None,
        env: Mapping[str, str] | None = None,
    ) -> None:
        if command is None:
            binary = executable or os.environ.get("CA_CLIENT_TOOLS_BIN") or "ca-client-tools"
            command = [binary, "codec", "worker"]
        self._command = list(command)
        self._max_frame = int(max_frame)
        self._cwd = cwd
        self._env = dict(env) if env is not None else None
        self._process: subprocess.Popen[bytes] | None = None
        self._frames: queue.Queue[bytes | BaseException] = queue.Queue()
        self._reader: threading.Thread | None = None
        self._write_lock = threading.Lock()
        self._request_counter = 0

    def __enter__(self) -> "CodecWorker":
        self.start()
        return self

    def __exit__(self, exc_type: object, exc: object, tb: object) -> None:
        self.close()

    @property
    def pid(self) -> int | None:
        return self._process.pid if self._process is not None else None

    @property
    def running(self) -> bool:
        return self._process is not None and self._process.poll() is None

    def start(self) -> "CodecWorker":
        if self.running:
            return self
        self._stop_process()
        self._frames = queue.Queue()
        try:
            self._process = subprocess.Popen(
                self._command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
                cwd=self._cwd,
                env=self._env,
            )
        except OSError as exc:
            self._process = None
            raise CodecWorkerError(f"could not start codec worker: {exc}") from exc
        self._reader = threading.Thread(target=self._read_loop, name="ca-codec-worker-reader", daemon=True)
        self._reader.start()
        return self

    def close(self, timeout: float = 5.0) -> None:
        process = self._process
        if process is None:
            return
        try:
            if process.stdin is not None:
                process.stdin.close()
        except OSError:
            pass
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            process.terminate()
            try:
                process.wait(timeout=2.0)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=2.0)
        self._stop_process()

    def cancel(self) -> None:
        """Cancel an in-flight wait by terminating the worker.

        The protocol is serial, so a late response could not be matched safely.
        The next request starts a fresh worker process.
        """

        self._stop_process()

    def ping(self, timeout: float = 5.0) -> WorkerResponse:
        return self.request("ping", timeout=timeout)

    def capabilities(self, timeout: float = 10.0) -> WorkerResponse:
        return self.request("capabilities", timeout=timeout)

    def decode(
        self,
        kind: str,
        payload: bytes,
        *,
        root: str | None = None,
        options: Mapping[str, Any] | None = None,
        timeout: float = 10.0,
        request_id: str | None = None,
    ) -> WorkerResponse:
        if not isinstance(payload, (bytes, bytearray, memoryview)):
            raise TypeError("decode payload must be bytes")
        header: dict[str, Any] = {
            "operation": "decode",
            "kind": kind,
            "options": dict(options or {}),
        }
        if root is not None:
            header["root"] = root
        return self.request(
            "decode",
            kind=kind,
            payload=bytes(payload),
            root=root,
            options=options,
            timeout=timeout,
            request_id=request_id,
            _header=header,
        )

    def encode(
        self,
        kind: str,
        message: Any,
        *,
        type_name: str | None = None,
        root: str | None = None,
        options: Mapping[str, Any] | None = None,
        timeout: float = 10.0,
        request_id: str | None = None,
    ) -> WorkerResponse:
        header: dict[str, Any] = {
            "operation": "encode",
            "kind": kind,
            "message": message,
            "options": dict(options or {}),
        }
        if type_name is not None:
            header["type"] = type_name
        if root is not None:
            header["root"] = root
        return self.request(
            "encode",
            kind=kind,
            root=root,
            type_name=type_name,
            message=message,
            options=options,
            timeout=timeout,
            request_id=request_id,
            _header=header,
        )

    def request(
        self,
        operation: str,
        *,
        kind: str | None = None,
        payload: bytes = b"",
        root: str | None = None,
        type_name: str | None = None,
        message: Any = None,
        options: Mapping[str, Any] | None = None,
        timeout: float = 10.0,
        request_id: str | None = None,
        _header: Mapping[str, Any] | None = None,
    ) -> WorkerResponse:
        if not self.running:
            self.start()
        if request_id is None:
            self._request_counter += 1
            request_id = f"py-{os.getpid()}-{self._request_counter}-{uuid.uuid4().hex[:8]}"
        if _header is None:
            header: dict[str, Any] = {"operation": operation}
            if kind is not None:
                header["kind"] = kind
            if root is not None:
                header["root"] = root
            if type_name is not None:
                header["type"] = type_name
            if message is not None:
                header["message"] = message
            if options is not None:
                header["options"] = dict(options)
        else:
            header = dict(_header)
        header["requestId"] = request_id
        frame = self._encode_frame(header, payload)
        with self._write_lock:
            assert self._process is not None and self._process.stdin is not None
            try:
                self._process.stdin.write(frame)
                self._process.stdin.flush()
            except (BrokenPipeError, OSError) as exc:
                self._stop_process()
                raise CodecWorkerExited("worker stdin closed") from exc
        try:
            item = self._frames.get(timeout=timeout)
        except queue.Empty as exc:
            self.cancel()
            raise CodecWorkerTimeout(f"codec worker did not answer within {timeout}s") from exc
        if isinstance(item, BaseException):
            raise CodecWorkerExited(str(item)) from item
        response_header, binary = self._decode_frame(item)
        try:
            response = json.loads(response_header)
        except json.JSONDecodeError as exc:
            raise CodecWorkerProtocolError("worker response JSON is invalid") from exc
        got_id = response.get("requestId")
        if got_id != request_id:
            raise CodecWorkerProtocolError(f"response requestId {got_id!r} does not match {request_id!r}")
        return WorkerResponse(
            request_id=request_id,
            status=str(response.get("status", "")),
            operation=str(response.get("operation", "")),
            raw=response,
            binary=binary,
        )

    def _encode_frame(self, header: Mapping[str, Any], payload: bytes) -> bytes:
        header_bytes = json.dumps(header, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        if len(header_bytes) > MAX_JSON:
            raise CodecWorkerProtocolError("request JSON exceeds the worker bound")
        total = 1 + 4 + len(header_bytes) + len(payload)
        if total > self._max_frame:
            raise CodecWorkerProtocolError("request frame exceeds the configured bound")
        return struct.pack(">I", total) + bytes([PROTOCOL_VERSION]) + struct.pack(">I", len(header_bytes)) + header_bytes + payload

    def _decode_frame(self, frame: bytes) -> tuple[bytes, bytes]:
        if len(frame) < 5 or frame[0] != PROTOCOL_VERSION:
            raise CodecWorkerProtocolError("invalid worker frame version")
        header_len = struct.unpack(">I", frame[1:5])[0]
        if header_len > len(frame) - 5 or header_len > MAX_JSON:
            raise CodecWorkerProtocolError("invalid worker JSON length")
        return frame[5 : 5 + header_len], frame[5 + header_len :]

    def _read_loop(self) -> None:
        process = self._process
        assert process is not None and process.stdout is not None
        stream = process.stdout
        try:
            while True:
                prefix = _read_exact(stream, 4)
                if prefix is None:
                    break
                length = struct.unpack(">I", prefix)[0]
                if length < 5 or length > self._max_frame:
                    raise CodecWorkerProtocolError("invalid worker frame length")
                body = _read_exact(stream, length)
                if body is None:
                    break
                self._frames.put(body)
        except BaseException as exc:  # noqa: BLE001 - forwarded to the requester
            self._frames.put(exc)
        finally:
            try:
                process.wait(timeout=1.0)
            except Exception:  # noqa: BLE001
                pass
            self._frames.put(CodecWorkerExited("worker process exited"))

    def _stop_process(self) -> None:
        process = self._process
        self._process = None
        if process is None:
            return
        try:
            if process.stdin is not None:
                process.stdin.close()
        except OSError:
            pass
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=2.0)
            except subprocess.TimeoutExpired:
                process.kill()
                try:
                    process.wait(timeout=2.0)
                except subprocess.TimeoutExpired:
                    pass
        for stream in (process.stdout, process.stdin):
            try:
                if stream is not None:
                    stream.close()
            except OSError:
                pass


def _read_exact(stream: Any, length: int) -> bytes | None:
    chunks = bytearray()
    while len(chunks) < length:
        chunk = stream.read(length - len(chunks))
        if not chunk:
            if not chunks:
                return None
            raise CodecWorkerProtocolError("worker closed mid-frame")
        chunks.extend(chunk)
    return bytes(chunks)
