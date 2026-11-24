from __future__ import annotations

import os
import struct
import sys
import threading
import time
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "python"))

from ca_codec_worker import (  # noqa: E402
    CodecWorker,
    CodecWorkerCancelled,
    CodecWorkerError,
    CodecWorkerExited,
    CodecWorkerProtocolError,
    CodecWorkerTimeout,
)

VERSION = 1

FAKE_WORKER = r"""
import json
import os
import struct
import sys
import time

VERSION = 1


def read_exact(n):
    buf = b""
    while len(buf) < n:
        chunk = sys.stdin.buffer.read(n - len(buf))
        if not chunk:
            return None
        buf += chunk
    return buf


def read_frame():
    prefix = read_exact(4)
    if prefix is None:
        return None, None
    length = struct.unpack(">I", prefix)[0]
    body = read_exact(length)
    if body is None:
        return None, None
    json_len = struct.unpack(">I", body[1:5])[0]
    return body[5:5 + json_len], body[5 + json_len:]


def write_frame(header, payload=b""):
    encoded = json.dumps(header).encode("utf-8")
    total = 1 + 4 + len(encoded) + len(payload)
    sys.stdout.buffer.write(struct.pack(">I", total) + bytes([VERSION]) +
                            struct.pack(">I", len(encoded)) + encoded + payload)
    sys.stdout.buffer.flush()


while True:
    header, payload = read_frame()
    if header is None:
        break
    request = json.loads(header)
    options = request.get("options") or {}
    mode = options.get("mode")
    if mode == "exit":
        os._exit(3)
    if mode == "sleep":
        time.sleep(float(options.get("sleep", 30)))
    response = {
        "requestId": request.get("requestId"),
        "operation": request.get("operation", ""),
        "status": "SUCCESS",
        "options": options,
    }
    if mode == "wrong_id":
        response["requestId"] = "someone-else"
    if mode == "error":
        response["status"] = "INVALID"
        response["error"] = {"code": "FAKE_ERROR", "message": "fake failure"}
    if mode == "bad_frame":
        sys.stdout.buffer.write(struct.pack(">I", 5) + bytes([9]) + struct.pack(">I", 0))
        sys.stdout.buffer.flush()
        continue
    write_frame(response, payload)
"""


def worker_binary() -> str:
    configured = os.environ.get("CA_CLIENT_TOOLS_BIN")
    if configured:
        return configured
    candidate = ROOT / ("ca-client-tools.exe" if os.name == "nt" else "ca-client-tools")
    return str(candidate)


NAS_ACCEPT = "2e0101c211000901000631310101ff01060261a80261a82905010a0a0008220401000001"
NAS_EPCO = "2e0203c1ffff93a37b000e00010401020304000d0303080808"
N2_SETUP = "000003008b000a01f00a0a0001000007d100860001000088000700010000091c00"


@unittest.skipUnless(os.path.exists(worker_binary()), "ca-client-tools binary is not built")
class CodecWorkerTest(unittest.TestCase):
    def test_ping_capabilities_and_reuse(self) -> None:
        with CodecWorker(worker_binary()) as worker:
            pid = worker.pid
            self.assertEqual(worker.ping().status, "SUCCESS")
            caps = worker.capabilities()
            self.assertEqual(caps.status, "SUCCESS")
            self.assertEqual(caps.raw["capabilities"]["schemaVersion"], "ca-codec-capabilities/v1")
            self.assertEqual(worker.pid, pid)
            self.assertEqual(caps.raw["capabilities"]["modules"][1]["version"], "v1.2.0")

    def test_nas_decode_encode_and_raw_binary(self) -> None:
        raw = bytes.fromhex(NAS_ACCEPT)
        with CodecWorker(worker_binary()) as worker:
            decoded = worker.decode("nas", raw)
            self.assertEqual(decoded.status, "SUCCESS")
            self.assertTrue(decoded.raw["rawPreserved"])
            self.assertEqual(decoded.raw["rawLength"], len(raw))
            self.assertTrue(decoded.reencode_eligible)
            self.assertTrue(decoded.qualified)
            self.assertEqual(decoded.message["PDUSessId"], 1)
            self.assertEqual(decoded.message["PTI"], 1)
            encoded = worker.encode("nas", decoded.message, type_name="nas.message.PDUSessEstAccept")
            self.assertEqual(encoded.status, "SUCCESS")
            self.assertEqual(encoded.binary, raw)

    def test_nas_unqualified_message_keeps_roundtrip_evidence(self) -> None:
        with CodecWorker(worker_binary()) as worker:
            decoded = worker.decode("nas", bytes.fromhex("2e0101cc"))
            self.assertEqual(decoded.status, "SUCCESS")
            self.assertFalse(decoded.qualified)
            self.assertTrue(decoded.reencode_verified)
            self.assertTrue(decoded.reencode_eligible)
            self.assertTrue(decoded.raw["complete"])
            codes = {item["code"] for item in decoded.raw.get("diagnostics", [])}
            self.assertIn("NAS_NOT_QUALIFIED", codes)

    def test_nas_partial_and_lossy_epco(self) -> None:
        with CodecWorker(worker_binary()) as worker:
            partial = worker.decode("nas", bytes.fromhex("2e0101c1ffff91a16603112233"))
            self.assertEqual(partial.status, "PARTIAL")
            self.assertEqual(partial.raw["ieToDo"], [102])
            self.assertFalse(partial.raw["complete"])
            lossy = worker.decode("nas", bytes.fromhex(NAS_EPCO))
            self.assertEqual(lossy.status, "SUCCESS")
            self.assertFalse(lossy.reencode_eligible)
            codes = {item["code"] for item in lossy.raw.get("diagnostics", [])}
            self.assertIn("NAS_EPCO_REENCODE_LOSSY", codes)
            encode = worker.encode("nas", lossy.message, type_name="nas.message.PDUSessEstReq")
            self.assertEqual(encode.status, "NOT_QUALIFIED")

    def test_n2_root_and_round_trip(self) -> None:
        raw = bytes.fromhex(N2_SETUP)
        with CodecWorker(worker_binary()) as worker:
            decoded = worker.decode("n2", raw, root="PDUSessionResourceSetupRequestTransfer")
            self.assertEqual(decoded.status, "SUCCESS")
            self.assertTrue(decoded.qualified)
            self.assertEqual(decoded.raw["root"], "PDUSessionResourceSetupRequestTransfer")
            encoded = worker.encode(
                "n2",
                decoded.message,
                root="PDUSessionResourceSetupRequestTransfer",
            )
            self.assertEqual(encoded.status, "SUCCESS")
            self.assertEqual(encoded.binary, raw)
            rejected = worker.decode("n2", b"\x00", root="NoSuchTransfer")
            self.assertEqual(rejected.status, "UNSUPPORTED_MESSAGE")

    def test_ngap_pdu_is_not_qualified(self) -> None:
        # NGReset self-encoded vector: byte-identical re-encode is not qualification.
        raw = bytes.fromhex("0014000d000002000f4001400058000100")
        with CodecWorker(worker_binary()) as worker:
            decoded = worker.decode("ngap", raw)
            self.assertEqual(decoded.status, "SUCCESS")
            self.assertFalse(decoded.qualified)
            self.assertTrue(decoded.reencode_verified)
            codes = {item["code"] for item in decoded.raw.get("diagnostics", [])}
            self.assertIn("NGAP_NOT_QUALIFIED", codes)
            encoded = worker.encode("ngap", decoded.message, type_name="ngap.message.NGReset")
            self.assertEqual(encoded.status, "SUCCESS")
            self.assertEqual(encoded.binary, raw)

    def test_unsupported_json_encode_is_gated(self) -> None:
        with CodecWorker(worker_binary()) as worker:
            decoded = worker.decode("nas", bytes.fromhex(NAS_ACCEPT))
            response = worker.encode("nas", decoded.message, type_name="nas.message.PDUSessModComplete")
            self.assertEqual(response.status, "NOT_QUALIFIED")
            self.assertEqual(response.raw["error"]["code"], "JSON_ENCODE_NOT_QUALIFIED")

    def test_timeout_stops_worker(self) -> None:
        command = [sys.executable, "-c", "import time; time.sleep(30)"]
        worker = CodecWorker(command=command)
        self.addCleanup(worker.close)
        worker.start()
        with self.assertRaises(CodecWorkerTimeout):
            worker.ping(timeout=0.2)
        self.assertFalse(worker.running)

    def test_abnormal_exit_is_reported(self) -> None:
        worker = CodecWorker(command=[sys.executable, "-c", "raise SystemExit(3)"])
        self.addCleanup(worker.close)
        worker.start()
        with self.assertRaises(CodecWorkerExited):
            worker.ping(timeout=2.0)


class FakeWorkerTestCase(unittest.TestCase):
    def fake_worker(self, **kwargs: object) -> CodecWorker:
        worker = CodecWorker(command=[sys.executable, "-c", FAKE_WORKER], **kwargs)  # type: ignore[arg-type]
        self.addCleanup(worker.close)
        return worker

    @staticmethod
    def assert_stopped(test: unittest.TestCase, worker: CodecWorker) -> None:
        test.assertFalse(worker.running)
        test.assertIsNone(worker._process)


class CodecWorkerConcurrencyTest(FakeWorkerTestCase):
    def test_two_threads_do_not_mismatch(self) -> None:
        worker = self.fake_worker()
        barrier = threading.Barrier(2)
        results: dict[int, tuple[object, bytes]] = {}
        errors: list[BaseException] = []

        def call(index: int) -> None:
            payload = bytes([index]) * 4
            barrier.wait(timeout=5)
            try:
                response = worker.decode("nas", payload, options={"tag": index})
                results[index] = (response, payload)
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        threads = [threading.Thread(target=call, args=(i,)) for i in (1, 2)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(timeout=15)
        self.assertFalse(any(thread.is_alive() for thread in threads), "request deadlocked")
        self.assertEqual(errors, [])
        self.assertEqual(set(results), {1, 2})
        for index, (response, payload) in results.items():
            self.assertEqual(response.request_id, response.raw["requestId"])  # type: ignore[attr-defined]
            self.assertEqual(response.raw["options"]["tag"], index)  # type: ignore[attr-defined]
            self.assertEqual(response.binary, payload)  # type: ignore[attr-defined]

    def test_many_threads_sequential_requests(self) -> None:
        worker = self.fake_worker()
        thread_count, per_thread = 8, 5
        barrier = threading.Barrier(thread_count)
        errors: list[BaseException] = []
        request_ids: list[str] = []
        lock = threading.Lock()

        def call(index: int) -> None:
            barrier.wait(timeout=5)
            try:
                for step in range(per_thread):
                    payload = bytes([index, step]) * 3
                    response = worker.decode("nas", payload, options={"thread": index, "step": step})
                    if response.binary != payload:
                        raise AssertionError("binary payload mismatch")
                    if response.raw["options"] != {"thread": index, "step": step}:
                        raise AssertionError("response was associated with another request")
                    with lock:
                        request_ids.append(response.request_id)
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        threads = [threading.Thread(target=call, args=(i,)) for i in range(thread_count)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(timeout=30)
        self.assertFalse(any(thread.is_alive() for thread in threads), "request deadlocked")
        self.assertEqual(errors, [])
        self.assertEqual(len(request_ids), thread_count * per_thread)
        self.assertEqual(len(set(request_ids)), len(request_ids), "request IDs collided")

    def test_slow_request_does_not_receive_fast_response(self) -> None:
        worker = self.fake_worker()
        barrier = threading.Barrier(2)
        results: dict[str, object] = {}
        errors: list[BaseException] = []

        def slow() -> None:
            barrier.wait(timeout=5)
            try:
                results["slow"] = worker.decode(
                    "nas", b"slow", options={"tag": "slow", "mode": "sleep", "sleep": 0.2}
                )
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        def fast() -> None:
            barrier.wait(timeout=5)
            try:
                results["fast"] = worker.decode("nas", b"fast", options={"tag": "fast"})
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        threads = [threading.Thread(target=slow), threading.Thread(target=fast)]
        started = time.monotonic()
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(timeout=20)
        elapsed = time.monotonic() - started
        self.assertFalse(any(thread.is_alive() for thread in threads))
        self.assertEqual(errors, [])
        for key in ("slow", "fast"):
            self.assertEqual(results[key].raw["options"]["tag"], key)  # type: ignore[attr-defined]
            self.assertEqual(results[key].binary, key.encode())  # type: ignore[attr-defined]
        # The serial lock must order the two requests; a fast request cannot
        # overtake the slow one and steal its response.
        self.assertGreaterEqual(elapsed, 0.15)

    def test_worker_exit_during_request_then_restart(self) -> None:
        worker = self.fake_worker()
        worker.start()
        with self.assertRaises(CodecWorkerExited):
            worker.decode("nas", b"x", options={"mode": "exit"}, timeout=5.0)
        self.assert_stopped(self, worker)
        response = worker.ping()
        self.assertEqual(response.status, "SUCCESS")

    def test_cancel_interrupts_in_flight_request(self) -> None:
        worker = self.fake_worker()
        worker.start()
        errors: list[BaseException] = []

        def call() -> None:
            try:
                worker.decode("nas", b"x", options={"mode": "sleep", "sleep": 30}, timeout=60.0)
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        thread = threading.Thread(target=call)
        thread.start()
        self.assertTrue(worker.in_flight or self._wait_for_in_flight(worker), "request never reached the worker")
        worker.cancel()
        thread.join(timeout=10)
        self.assertFalse(thread.is_alive(), "cancel did not terminate the wait")
        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], CodecWorkerCancelled)
        self.assert_stopped(self, worker)

    def test_timeout_then_restart(self) -> None:
        worker = self.fake_worker()
        old_pid = worker.start().pid
        with self.assertRaises(CodecWorkerTimeout):
            worker.decode("nas", b"x", options={"mode": "sleep", "sleep": 30}, timeout=0.2)
        self.assert_stopped(self, worker)
        response = worker.ping()
        self.assertEqual(response.status, "SUCCESS")
        self.assertNotEqual(worker.pid, old_pid)
        self.assertTrue(worker.running)

    def test_restart_isolates_old_response_queue(self) -> None:
        worker = self.fake_worker()
        worker.start()
        old_frames = worker._frames
        with self.assertRaises(CodecWorkerTimeout):
            worker.decode("nas", b"x", options={"mode": "sleep", "sleep": 30}, timeout=0.2)
        response = worker.ping()
        self.assertEqual(response.status, "SUCCESS")
        self.assertIsNot(worker._frames, old_frames)
        self.assertIsNotNone(worker._frames)
        # The old generation must only contain failure markers, never a response
        # that could be mistaken for the new generation's answer.
        while not old_frames.empty():  # type: ignore[union-attr]
            item = old_frames.get_nowait()  # type: ignore[union-attr]
            self.assertIsInstance(item, BaseException)

    def test_close_interrupts_in_flight_request(self) -> None:
        worker = self.fake_worker()
        worker.start()
        errors: list[BaseException] = []

        def call() -> None:
            try:
                worker.decode("nas", b"x", options={"mode": "sleep", "sleep": 30}, timeout=60.0)
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        thread = threading.Thread(target=call)
        thread.start()
        self.assertTrue(self._wait_for_in_flight(worker), "request never reached the worker")
        worker.close()
        thread.join(timeout=10)
        self.assertFalse(thread.is_alive(), "close did not terminate the wait")
        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], CodecWorkerExited)
        self.assert_stopped(self, worker)
        with self.assertRaises(CodecWorkerError):
            worker.ping()

    def test_duplicate_request_id_is_rejected(self) -> None:
        worker = self.fake_worker()
        first = worker.request("ping", request_id="dup-id")
        self.assertEqual(first.request_id, "dup-id")
        with self.assertRaises(CodecWorkerProtocolError):
            worker.request("ping", request_id="dup-id")
        self.assertEqual(worker.ping().status, "SUCCESS")

    def test_response_request_id_mismatch_is_rejected(self) -> None:
        worker = self.fake_worker()
        with self.assertRaises(CodecWorkerProtocolError):
            worker.decode("nas", b"x", options={"mode": "wrong_id"})
        self.assert_stopped(self, worker)

    def test_malformed_response_frame_stops_worker(self) -> None:
        worker = self.fake_worker()
        with self.assertRaises(CodecWorkerProtocolError):
            worker.decode("nas", b"x", options={"mode": "bad_frame"})
        self.assert_stopped(self, worker)
        self.assertEqual(worker.ping().status, "SUCCESS")

    def test_request_order_and_association(self) -> None:
        worker = self.fake_worker()
        for index in range(25):
            payload = f"request-{index}".encode()
            response = worker.decode("nas", payload, options={"sequence": index})
            self.assertEqual(response.binary, payload)
            self.assertEqual(response.raw["options"]["sequence"], index)
            self.assertEqual(response.raw["requestId"], response.request_id)
        self.assertTrue(worker.running)

    def test_frame_bounds_are_rejected_before_allocation(self) -> None:
        worker = self.fake_worker(max_frame=32)
        with self.assertRaises(CodecWorkerProtocolError):
            worker.decode("nas", b"x" * 64)
        with self.assertRaises(CodecWorkerProtocolError):
            worker._decode_frame(b"\x02" + b"\x00\x00\x00\x00")
        with self.assertRaises(CodecWorkerProtocolError):
            worker._decode_frame(b"\x01" + struct.pack(">I", 100) + b"short")

    @staticmethod
    def _wait_for_in_flight(worker: CodecWorker, timeout: float = 5.0) -> bool:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if worker.in_flight:
                return True
            time.sleep(0.005)
        return False


if __name__ == "__main__":
    unittest.main()
