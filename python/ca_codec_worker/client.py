"""Long-lived CA codec worker client.

The worker is the existing ``ca-client-tools codec worker`` process. This module
owns the process lifecycle and the framed stdin/stdout transport. It performs no
protocol decoding itself: JSON and raw binary are passed to the same Go facade
used by native Go consumers.

Concurrency model
-----------------
The worker protocol is strictly serial: one request frame produces exactly one
response frame, in order. A single :class:`threading.Lock` therefore covers the
whole request-response cycle (availability check, request ID, frame write,
response wait, request ID validation). This is deliberately simpler and safer
than multiplexing: without the lock two caller threads could each read the other
thread's response from the shared queue.

``cancel()`` and ``close()`` do not take that request lock, so they can stop an
in-flight wait immediately instead of waiting for the full request timeout. Each
worker process generation owns its own response queue and reader thread, so a
late frame from a stopped worker can never be delivered to a restarted worker.
"""

from __future__ import annotations

import json
import os
import queue
import struct
import subprocess
import threading
import uuid
from collections import deque
from dataclasses import dataclass
from typing import Any, Iterable, Mapping

PROTOCOL_VERSION = 1
DEFAULT_MAX_FRAME = 64 << 20
MAX_JSON = 4 << 20
# Bound the duplicate-request-ID memory without allowing unbounded growth.
MAX_TRACKED_REQUEST_IDS = 4096


class CodecWorkerError(Exception):
    """Base class for worker transport and process errors."""


class CodecWorkerExited(CodecWorkerError):
    """The worker process exited or closed its output."""


class CodecWorkerCancelled(CodecWorkerError):
    """An in-flight request was cancelled; the worker was stopped."""


class CodecWorkerTimeout(CodecWorkerError):
    """A request did not complete before its deadline; the worker was stopped."""


class CodecWorkerProtocolError(CodecWorkerError):
    """The worker emitted an invalid frame or mismatched request ID."""


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
    def reencode_verified(self) -> bool:
        return bool(self.raw.get("reencodeVerified"))

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

        # Lifecycle state. ``_lifecycle`` guards process/queue/generation
        # replacement; it is never held while waiting for a response.
        self._lifecycle = threading.RLock()
        self._process: subprocess.Popen[bytes] | None = None
        self._frames: queue.Queue[bytes | BaseException] | None = None
        self._reader: threading.Thread | None = None
        self._generation = 0
        self._closed = False
        self._cancelled_generations: set[int] = set()

        # One lock for the complete serial request-response cycle.
        self._request_lock = threading.Lock()
        self._request_counter = 0
        self._used_request_ids: set[str] = set()
        self._used_request_order: deque[str] = deque()
        self._in_flight = threading.Event()

    def __enter__(self) -> "CodecWorker":
        self.start()
        return self

    def __exit__(self, exc_type: object, exc: object, tb: object) -> None:
        self.close()

    @property
    def pid(self) -> int | None:
        with self._lifecycle:
            return self._process.pid if self._process is not None else None

    @property
    def running(self) -> bool:
        with self._lifecycle:
            return self._process is not None and self._process.poll() is None

    @property
    def in_flight(self) -> bool:
        """True while a request has been written and its response is awaited."""
        return self._in_flight.is_set()

    def start(self) -> "CodecWorker":
        """Start the worker if it is not already running.

        Thread-safe and idempotent. A stopped worker is replaced with a fresh
        process, queue and reader thread.
        """
        with self._lifecycle:
            if self._closed:
                raise CodecWorkerError("codec worker is closed")
            if self._process is not None and self._process.poll() is None:
                return self
            self._stop_process_locked()
            frames: queue.Queue[bytes | BaseException] = queue.Queue()
            try:
                process = subprocess.Popen(
                    self._command,
                    stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.DEVNULL,
                    cwd=self._cwd,
                    env=self._env,
                )
            except OSError as exc:
                self._process = None
                self._frames = None
                raise CodecWorkerError(f"could not start codec worker: {exc}") from exc
            self._process = process
            self._frames = frames
            self._generation += 1
            reader = threading.Thread(
                target=self._read_loop,
                args=(process, frames),
                name=f"ca-codec-worker-reader-{self._generation}",
                daemon=True,
            )
            self._reader = reader
            reader.start()
            return self

    def close(self, timeout: float = 5.0) -> None:
        """Close the worker permanently.

        Safe to call while another thread is inside :meth:`request`: the waiting
        request is woken with :class:`CodecWorkerExited` instead of blocking
        until its own timeout.
        """
        with self._lifecycle:
            self._closed = True
        self._stop_process(timeout)

    def cancel(self) -> None:
        """Cancel an in-flight request by stopping the worker.

        The protocol is serial, so a late response could not be matched safely.
        The next request starts a fresh worker process and a fresh queue. This
        call does not wait for the request lock, so it interrupts a wait
        immediately from another thread.
        """
        with self._lifecycle:
            if self._process is None:
                return
            self._cancelled_generations.add(self._generation)
        self._stop_process()

    def ping(self, timeout: float = 5.0, *, request_id: str | None = None) -> WorkerResponse:
        return self.request("ping", timeout=timeout, request_id=request_id)

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
        with self._request_lock:
            if self._closed:
                raise CodecWorkerError("codec worker is closed")
            if request_id is None:
                self._request_counter += 1
                request_id = f"py-{os.getpid()}-{self._request_counter}-{uuid.uuid4().hex[:8]}"
            self._reserve_request_id(request_id)
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
            self.start()
            with self._lifecycle:
                process = self._process
                frames = self._frames
                generation = self._generation
            if process is None or frames is None:
                raise CodecWorkerError("codec worker is not running")
            try:
                with self._lifecycle:
                    if self._process is not process:
                        raise CodecWorkerExited("codec worker was replaced before the request was written")
                    assert process.stdin is not None
                    process.stdin.write(frame)
                    process.stdin.flush()
            except (BrokenPipeError, OSError, ValueError) as exc:
                self._stop_process()
                raise CodecWorkerExited("worker stdin closed") from exc
            self._in_flight.set()
            try:
                item = frames.get(timeout=timeout)
            except queue.Empty as exc:
                self._stop_process()
                raise CodecWorkerTimeout(f"codec worker did not answer within {timeout}s") from exc
            finally:
                self._in_flight.clear()
            if isinstance(item, BaseException):
                # The transport failed for this generation. Drop the process
                # reference so the next request starts cleanly.
                self._stop_process()
                if self._was_cancelled(generation):
                    raise CodecWorkerCancelled("codec worker request was cancelled") from item
                if isinstance(item, CodecWorkerError):
                    raise item
                raise CodecWorkerExited(str(item)) from item
            response_header, binary = self._decode_frame(item)
            try:
                response = json.loads(response_header)
            except json.JSONDecodeError as exc:
                self._stop_process()
                raise CodecWorkerProtocolError("worker response JSON is invalid") from exc
            if not isinstance(response, Mapping):
                self._stop_process()
                raise CodecWorkerProtocolError("worker response JSON is not an object")
            got_id = response.get("requestId")
            if got_id != request_id:
                # The serial stream is out of sync; do not reuse this process.
                self._stop_process()
                raise CodecWorkerProtocolError(
                    f"response requestId {got_id!r} does not match {request_id!r}"
                )
            return WorkerResponse(
                request_id=request_id,
                status=str(response.get("status", "")),
                operation=str(response.get("operation", "")),
                raw=response,
                binary=binary,
            )

    def _reserve_request_id(self, request_id: str) -> None:
        if request_id in self._used_request_ids:
            raise CodecWorkerProtocolError(f"requestId {request_id!r} was already used")
        self._used_request_ids.add(request_id)
        self._used_request_order.append(request_id)
        while len(self._used_request_order) > MAX_TRACKED_REQUEST_IDS:
            self._used_request_ids.discard(self._used_request_order.popleft())

    def _was_cancelled(self, generation: int) -> bool:
        with self._lifecycle:
            return generation in self._cancelled_generations

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

    def _read_loop(self, process: subprocess.Popen[bytes], frames: queue.Queue[bytes | BaseException]) -> None:
        """Read frames for one process generation into that generation's queue.

        The process and queue are captured as arguments, never read from
        ``self``: a restarted worker must not receive frames read by the old
        process, and a stopping worker must still be able to report its exit to
        the request that is waiting on its own queue.
        """
        stream = process.stdout
        if stream is None:  # pragma: no cover - Popen always provides stdout here
            frames.put(CodecWorkerExited("worker has no stdout"))
            return
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
                frames.put(body)
        except BaseException as exc:  # noqa: BLE001 - forwarded to the requester
            frames.put(exc)
        finally:
            try:
                process.wait(timeout=1.0)
            except Exception:  # noqa: BLE001
                pass
            frames.put(CodecWorkerExited("worker process exited"))

    def _stop_process(self, timeout: float = 2.0) -> None:
        with self._lifecycle:
            process = self._process
            frames = self._frames
            self._process = None
            self._frames = None
            self._reader = None
        self._terminate(process, frames, timeout)

    def _stop_process_locked(self) -> None:
        """Stop the current process while ``_lifecycle`` is already held."""
        process = self._process
        frames = self._frames
        self._process = None
        self._frames = None
        self._reader = None
        self._terminate(process, frames, 2.0)

    @staticmethod
    def _terminate(
        process: subprocess.Popen[bytes] | None,
        frames: queue.Queue[bytes | BaseException] | None,
        timeout: float,
    ) -> None:
        if process is None:
            return
        try:
            if process.stdin is not None:
                process.stdin.close()
        except (OSError, ValueError):
            pass
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=timeout)
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
            except (OSError, ValueError):
                pass
        # Guarantee that a waiting request wakes up even if the reader thread is
        # delayed or has already exited silently.
        if frames is not None:
            frames.put(CodecWorkerExited("worker process stopped"))


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
