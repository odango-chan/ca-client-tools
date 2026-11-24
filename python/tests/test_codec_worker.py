from __future__ import annotations

import os
import sys
import time
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "python"))

from ca_codec_worker import CodecWorker, CodecWorkerExited, CodecWorkerTimeout  # noqa: E402


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
            self.assertEqual(decoded.message["PDUSessId"], 1)
            self.assertEqual(decoded.message["PTI"], 1)
            encoded = worker.encode("nas", decoded.message, type_name="nas.message.PDUSessEstAccept")
            self.assertEqual(encoded.status, "SUCCESS")
            self.assertEqual(encoded.binary, raw)

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


if __name__ == "__main__":
    unittest.main()
