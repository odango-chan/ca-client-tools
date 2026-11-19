#!/usr/bin/env python3
"""Build a reproducible MAS candidate archive and record the tagged build's provenance."""

import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

from verify import verify

CANDIDATE = Path(__file__).resolve().parent / "v0.2.0"
VERSION = CANDIDATE.name.removeprefix("v")
ARCHIVE = f"openamsm-mas-compiler-contract-v{VERSION}.zip"
RECORD = "mas-runtime-compatibility.json"
STABLE_TAG = r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"


def encode(value):
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode("utf-8")


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def sha256_files(files):
    return {name: sha256(data) for name, data in sorted(files.items())}


def build(candidate):
    verify(candidate)
    source = json.loads((candidate / "source-map.json").read_bytes())
    if source["version"] != f"{VERSION}-candidate":
        raise ValueError("Candidate and release package versions differ")
    identity = {
        "ownerId": source["owner"]["ownerId"],
        "stablePackageId": source["owner"]["stablePackageId"],
        "packageVersion": VERSION,
    }
    schemas = {}
    for path in sorted((candidate / "schemas").glob("*.schema.json")):
        data = path.read_bytes()
        schema_id = json.loads(data)["$id"]
        schemas[schema_id] = (f"schemas/{path.name}", data)
    files = dict(schemas.values())
    if len(files) != 25:
        raise ValueError("Expected 25 distinct Schema IDs")
    contracts = []
    referenced = set()
    for operation in source["operations"]:
        entry = {
            "id": operation["publicCallable"],
            "kind": "operation",
            "effectCandidate": operation["effectCandidate"],
        }
        for direction in ("input", "output"):
            schema_id = operation[f"{direction}Schema"]
            path, _ = schemas[schema_id]
            if path != f"schemas/{operation[f'{direction}SchemaFile']}":
                raise ValueError(f"Schema path mismatch for {schema_id}")
            entry[f"{direction}Schema"] = {"$ref": schema_id}
            referenced.add(schema_id)
        contracts.append(entry)
    if referenced != schemas.keys():
        raise ValueError("Catalog must reference the complete Schema set")

    files["tool-catalog-candidate.json"] = encode({
        "kind": "ToolCatalogCandidate",
        "metadata": {**identity, "status": "NOT_LINKABLE"},
        "spec": {"contracts": contracts},
    })
    files["schema-manifest.json"] = encode({
        "kind": "SchemaManifest",
        **identity,
        "schemas": {
            schema_id: {"path": path, "sha256": sha256(data)}
            for schema_id, (path, data) in schemas.items()
        },
    })
    for name in ("source-map.json", "schema-provenance.json"):
        files[name] = (candidate / name).read_bytes()
    digests = sha256_files(files)
    manifest = {
        "kind": "OwnerPackageManifest",
        **identity,
        "declaredPublisherId": identity["ownerId"],
        "status": "NOT_QUALIFIED",
        "deployable": False,
        "compilerLinkable": False,
        "publisherAuthorization": "DEPLOYMENT_REQUIRED",
        "files": digests,
        "contentSha256": sha256(encode(digests)),
    }
    files["owner-package-manifest.json"] = encode(manifest)
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as archive:
        for name, data in sorted(files.items()):
            info = zipfile.ZipInfo(name)  # Fixed timestamp, mode and storage method.
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            archive.writestr(info, data)
    data = buffer.getvalue()
    validate_archive(data, files)
    return manifest, data


def validate_archive(data, expected):
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        if sorted(archive.namelist()) != sorted(expected):
            raise ValueError("Archive file set differs from generated package")
        actual = {name: archive.read(name) for name in archive.namelist()}
    manifest = json.loads(actual.pop("owner-package-manifest.json"))
    digests = sha256_files(actual)
    if manifest["files"] != digests or manifest["contentSha256"] != sha256(encode(digests)):
        raise ValueError("Archive contents do not match the Owner Package digests")
    actual["owner-package-manifest.json"] = encode(manifest)
    if actual != expected:
        raise ValueError("Archived bytes differ from the generated files")


def gh(*args):
    return subprocess.check_output(["gh", *args])


def existing_archive(repository):
    pages = json.loads(gh("api", f"repos/{repository}/releases?per_page=100",
                          "--paginate", "--slurp"))
    matches = [
        (release["tag_name"], asset)
        for page in pages
        for release in page
        for asset in release["assets"]
        if asset["name"] == ARCHIVE
    ]
    if not matches:
        return None
    if len(matches) != 1:
        raise ValueError(f"Multiple release assets named {ARCHIVE}")
    tag, asset = matches[0]
    if asset["state"] != "uploaded" or not re.fullmatch(STABLE_TAG, tag):
        raise ValueError(f"{ARCHIVE} has an incomplete upload or an unexpected tag: {tag}")
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory) / ARCHIVE
        gh("release", "download", tag, "--repo", repository,
           "--pattern", ARCHIVE, "--output", str(path))
        return tag, path.read_bytes()


def stage(output, tag, commit, repository, existing=None):
    if not re.fullmatch(STABLE_TAG, tag) or not re.fullmatch(r"[a-f0-9]{40}", commit):
        raise ValueError("Expected a stable runtime tag and its exact source commit")
    manifest, archive = build(CANDIDATE)
    archive_tag = tag
    if existing is not None:
        archive_tag, previous = existing
        if archive != previous:
            raise ValueError(f"Immutable package {ARCHIVE} differs from {archive_tag}; "
                             "advance the contract version for changed content")
    output.mkdir(parents=True, exist_ok=True)
    if (output / ARCHIVE).exists() or (output / RECORD).exists():
        raise ValueError("Use a fresh staging directory for the MAS assets")
    if existing is None:
        (output / ARCHIVE).write_bytes(archive)
    record = {
        "kind": "RuntimeContractCompatibility",
        "runtimeVersion": tag[1:],
        "runtimeSupport": "NOT_QUALIFIED",
        "buildSource": {"repository": repository, "tag": tag, "commit": commit},
        "contractPackage": {
            **{key: manifest[key] for key in ("ownerId", "stablePackageId",
                                              "packageVersion", "contentSha256")},
            "archive": {"repository": repository, "releaseTag": archive_tag,
                        "asset": ARCHIVE, "sha256": sha256(archive)},
        },
    }
    (output / RECORD).write_bytes(encode(record))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--reuse-github-archive", action="store_true")
    args = parser.parse_args()
    try:
        existing = existing_archive(args.repository) if args.reuse_github_archive else None
        stage(args.output, args.tag, args.commit, args.repository, existing)
    except (AssertionError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"MAS package validation failed: {error}\n")


if __name__ == "__main__":
    main()
