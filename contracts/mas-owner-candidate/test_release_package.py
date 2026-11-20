"""Regression tests for immutable MAS candidate archives and versioned reuse."""

import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

from release_package import (
    ARCHIVE, CANDIDATE, RECORD, ROOT, build, encode, existing_archive, sha256, stage, validate_archive,
)


class ReleasePackageTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.manifest, cls.archive = build(CANDIDATE)

    def schema_source(self, filename):
        current = CANDIDATE / "schemas" / filename
        if current.exists():
            return current
        return CANDIDATE.parent / "v0.2.0" / "schemas" / filename

    def test_v0_3_candidate_is_reproducible_and_contains_exact_schema_bytes(self):
        self.assertEqual(build(CANDIDATE), (self.manifest, self.archive))
        with zipfile.ZipFile(io.BytesIO(self.archive)) as archive:
            catalog = json.loads(archive.read("tool-catalog-candidate.json"))
            schemas = json.loads(archive.read("schema-manifest.json"))["schemas"]
            self.assertEqual(catalog["kind"], "ToolCatalogCandidate")
            self.assertEqual(catalog["metadata"]["status"], "NOT_LINKABLE")
            self.assertEqual(len(catalog["spec"]["contracts"]), 21)
            self.assertEqual(len(schemas), 40)
            for entry in schemas.values():
                path = entry["path"]
                filename = Path(path).name
                data = archive.read(path)
                self.assertEqual(data, self.schema_source(filename).read_bytes())
                self.assertEqual(entry["sha256"], sha256(data))
            self.assertFalse(self.manifest["deployable"])
            self.assertFalse(self.manifest["compilerLinkable"])
            self.assertEqual(self.manifest["publisherAuthorization"], "DEPLOYMENT_REQUIRED")
            self.assertEqual(self.manifest["packageVersion"], "0.3.0")
            self.assertIn("owner-outcome-contract.md", archive.namelist())
            files = {name: archive.read(name) for name in archive.namelist()}

        corrupt = io.BytesIO()
        with zipfile.ZipFile(corrupt, "w") as archive:
            for name, data in files.items():
                archive.writestr(name, b"tampered" if name == "source-map.json" else data)
        with self.assertRaisesRegex(ValueError, "Owner Package digests"):
            validate_archive(corrupt.getvalue(), files)

    def test_v0_2_candidate_remains_byte_reproducible(self):
        legacy = ROOT / "v0.2.0"
        manifest, archive = build(legacy)
        self.assertEqual(manifest["packageVersion"], "0.2.0")
        with zipfile.ZipFile(io.BytesIO(archive)) as package:
            catalog = json.loads(package.read("tool-catalog-candidate.json"))
            schemas = json.loads(package.read("schema-manifest.json"))["schemas"]
            self.assertEqual(len(catalog["spec"]["contracts"]), 13)
            self.assertEqual(len(schemas), 25)
            self.assertNotIn("owner-outcome-contract.md", package.namelist())

    def test_archive_discovery_across_release_pages(self):
        asset = {"name": ARCHIVE, "state": "uploaded"}
        release = {"tag_name": "v1.1.0", "assets": [asset]}

        def github(command, *args):
            if command == "api":
                return encode([[{"tag_name": "v1.2.0", "assets": []}], [release]])
            self.assertEqual((command, *args[:2]), ("release", "download", "v1.1.0"))
            self.assertIn(ARCHIVE, args)
            Path(args[-1]).write_bytes(self.archive)

        with patch("release_package.gh", side_effect=github):
            self.assertEqual(existing_archive("example/repository"), ("v1.1.0", self.archive))
            asset["state"] = "starter"
            with self.assertRaisesRegex(ValueError, "incomplete upload"):
                existing_archive("example/repository")
        with patch("release_package.gh", return_value=encode([[release], [release]])):
            with self.assertRaisesRegex(ValueError, "Multiple release assets"):
                existing_archive("example/repository")

    def test_first_publication_reuse_and_collision(self):
        with tempfile.TemporaryDirectory() as directory, patch(
            "release_package.build", return_value=(self.manifest, self.archive)
        ):
            root = Path(directory)
            args = ("a" * 40, "example/repository")
            first, later, conflict = (root / name for name in ("first", "later", "conflict"))
            stage(first, "v1.1.0", *args)
            stage(later, "v1.2.0", *args, ("v1.1.0", self.archive))
            retry = root / "retry"
            stage(retry, "v1.1.0", *args, ("v1.1.0", self.archive))
            self.assertFalse((retry / ARCHIVE).exists())
            self.assertEqual((retry / RECORD).read_bytes(), (first / RECORD).read_bytes())
            self.assertEqual((first / ARCHIVE).read_bytes(), self.archive)
            self.assertFalse((later / ARCHIVE).exists())
            record = json.loads((later / RECORD).read_bytes())
            self.assertEqual(record["runtimeVersion"], "1.2.0")
            package = record["contractPackage"]
            self.assertEqual(package["packageVersion"], "0.3.0")
            self.assertEqual(package["archive"]["releaseTag"], "v1.1.0")
            self.assertEqual(package["archive"]["sha256"], sha256(self.archive))
            with self.assertRaisesRegex(ValueError, "Immutable package"):
                stage(conflict, "v1.3.0", *args, ("v1.1.0", b"changed"))
            self.assertFalse(conflict.exists())


if __name__ == "__main__":
    unittest.main()
