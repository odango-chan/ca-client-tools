"""Executable examples of the MAS contract rules, with synthetic records."""

import ipaddress
import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


def nonzero_decimal(value, bits):
    width = len(str((1 << bits) - 1))
    return (
        isinstance(value, str)
        and len(value) <= width
        and re.fullmatch("[1-9][0-9]*", value) is not None
        and int(value) < 1 << bits
    )


def literal_ip(value):
    if not isinstance(value, str) or "%" in value:
        return False
    try:
        ipaddress.ip_address(value)
    except ValueError:
        return False
    return True


class ContractExamples(unittest.TestCase):
    def test_tunnel_identity_value_boundaries(self):
        for bits in (32, 64):
            with self.subTest(bits=bits):
                for valid in ("1", str((1 << bits) - 1)):
                    self.assertTrue(nonzero_decimal(valid, bits))
                for invalid in ("", "0", "00", "01", "+1", " 1", "1\n", "x", str(1 << bits)):
                    self.assertFalse(nonzero_decimal(invalid, bits), invalid)
        for valid in ("192.0.2.1", "2001:db8::1", "::ffff:192.0.2.1"):
            self.assertTrue(literal_ip(valid))
        for invalid in ("invalid", "256.0.0.1", "192.0.2.1:2152", "fe80::1%eth0", "192.0.2.1/24"):
            self.assertFalse(literal_ip(invalid), invalid)

    def test_compiler_fixture_links_scope_value_and_failure_proof(self):
        with tempfile.TemporaryDirectory() as temp:
            project = Path(temp) / "fixture"
            generator = Path(__file__).with_name("build_fixture.py")
            subprocess.run([sys.executable, str(generator), str(project)], check=True)
            snapshot = project / "snapshots/mas"
            catalog = json.loads((snapshot / "tool-catalog.yaml").read_text(encoding="utf-8"))
            guarantees = json.loads((snapshot / "evidence-guarantees.yaml").read_text(encoding="utf-8"))
            self.assertEqual(len(guarantees), 13)
            for guarantee in guarantees.values():
                self.assertEqual(guarantee["scope"]["originalProducer"], {
                    "record": "scope.originalProducer", "equals": "execution.producerInvocationId"})
                self.assertEqual(guarantee["factValue"], "value")
            for operation in catalog["spec"]["contracts"]:
                [failure] = operation["mutation"]["failureClasses"]
                self.assertEqual(failure["outcome"], "FAILED")
                self.assertEqual(failure["proofPath"], "receipt.commit")
                self.assertTrue(failure["noSideEffect"])
                self.assertFalse(failure["retryable"])


if __name__ == "__main__":
    unittest.main()
