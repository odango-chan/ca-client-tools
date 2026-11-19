#!/usr/bin/env python3
"""Read-only fail-closed validation for the unpublished MAS source-contract candidate."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
EXPECTED = {
    "s1": "SC.VALIDATE_SUBSCRIPTION",
    "s2": "SAA.SECONDARY_AUTH",
    "s3": "SC.CREATE_SM_CONTEXT",
    "s3r": "SC.REGISTER_UECM",
    "s3p": "PA.CREATE_SM_POLICY",
    "s4": "UPS.DECIDE_ANCHOR",
    "s5": "IAM.ALLOCATE_ADDRESS",
    "s6u": "PA.UPDATE_SM_POLICY",
    "s7": "CA.CREATE_CHARGING",
    "s8": "UPN.CREATE_UP_SESSION",
    "s10": "SSM.SIGNAL_UE_AN",
    "s11": "UPN.MODIFY_UP_SESSION",
    "s13": "SE.EMIT_EVENTS",
}
OUTPUT_PREFIX = "urn:example:openamsm:mas:"


def schema_at(path: Path, identity: str, keys: list[str]) -> None:
    data = json.loads(path.read_text(encoding="utf-8"))
    assert data.get("$id") == identity, (path, "schema identity mismatch")
    assert data.get("$schema") == "https://json-schema.org/draft/2020-12/schema"
    assert data.get("type") == "object" and data.get("additionalProperties") is False
    assert data.get("required") == keys, (path, "required names/order mismatch")
    assert set(data.get("properties", {})) == set(keys)
    for field in keys:
        assert isinstance(data["properties"][field], dict), (path, field)


def verify(ROOT: Path) -> None:
    doc = json.loads((ROOT / "source-map.json").read_text(encoding="utf-8"))
    provenance = json.loads((ROOT / "schema-provenance.json").read_text(encoding="utf-8"))
    assert provenance["status"] == "SOURCE_PROVENANCE_ONLY"
    assert provenance["repository"] == doc["source"]["repository"]
    assert provenance["commit"] == doc["source"]["commit"]
    sources = provenance["sources"]
    assert sources["openamsm"]["path"] == doc["source"]["openapi"]
    for source in sources.values():
        for name, component in source["components"].items():
            assert component["pointer"] == f"/components/schemas/{name}"
            assert component["standaloneId"] is None
    for kind, count in (("input", 13), ("output", 12)):
        projection = provenance["projections"][kind]
        assert projection["glob"] == f"schemas/*.{kind}.schema.json"
        assert len(list(ROOT.glob(projection["glob"]))) == projection["count"] == count
        assert projection["sourceComponent"] in sources["openamsm"]["components"]
        assert projection["sourceValuePointer"] == f"/properties/{kind}s"
    field_sources = provenance["projections"]["input"]["fieldSources"]
    assert set(field_sources) == {"ue_identity", "psi", "dnn", "sNssai"}
    assert set(field_sources.values()) == set(sources["threegpp"]["components"])
    assert doc["kind"] == "MASOwnerSourceContractCandidate"
    assert doc["status"] == "NOT_QUALIFIED" and doc["deployable"] is False
    owner = doc["owner"]
    assert owner["ownerId"] is None and owner["stablePackageId"] is None
    assert owner["declaredOwnerAuthority"] == "UNBOUND"
    assert all(owner[key] is False for key in (
        "publishedToolCatalog", "publishedCapabilityContracts",
        "publishedEvidenceGuarantees", "mutationReceiptProducer", "reconcileProducer"
    ))
    operations = doc["operations"]
    assert len(operations) == 13
    assert {o["stepId"] for o in operations} == set(EXPECTED)
    assert len({o["publicCallable"] for o in operations}) == 13
    for item in operations:
        step = item["stepId"]
        assert item["publicCallable"] == EXPECTED[step], step
        assert item["effectCandidate"] == "MUTATION_REVIEW_REQUIRED"
        assert item["receiptPublication"] == "ABSENT"
        assert item["ownerQualification"] == "NOT_QUALIFIED"
        assert item["capabilityCandidate"]
        assert item["inputSchema"].startswith(OUTPUT_PREFIX)
        assert item["outputSchema"].startswith(OUTPUT_PREFIX)
        authors = item["authorInputs"]
        mappings = item["argumentMapping"]
        assert len(mappings) == len(authors)
        assert [m["from"] for m in mappings] == authors
        mapped = {x for m in mappings for x in (
            m["to"] if isinstance(m["to"], list) else [m["to"]]
        )}
        assert set(item["oesInputs"]) - mapped == set(item["sourceOnlyInputs"])
        if item["stepId"] == "s7":
            assert "supi" in mapped and "supi" not in item["sourceOnlyInputs"]
        input_file = item.get("inputSchemaFile", f"{step}.input.schema.json")
        schema_at(ROOT / "schemas" / input_file, item["inputSchema"], authors)
        output_id = item["outputSchema"]
        output_file = item.get("outputSchemaFile")
        if output_file is None:
            assert output_id.endswith(":v1")
            output_file = output_id.removeprefix(OUTPUT_PREFIX).removesuffix(":v1") + ".output.schema.json"
        private_outputs = item.get("privateOutputFields", [])
        assert private_outputs == (["sm_context_ref"] if step == "s3" else [])
        assert set(private_outputs) <= set(item["sourceOutputFields"])
        public_outputs = [
            field for field in item["sourceOutputFields"] if field not in private_outputs
        ]
        schema_at(
            ROOT / "schemas" / output_file,
            output_id, public_outputs
        )
    if ROOT.name == "v0.2.0":
        migration = json.loads((ROOT / "schema-id-migration.json").read_text(encoding="utf-8"))
        previous = json.loads((ROOT.parent / "v0.1.0" / "source-map.json").read_text(encoding="utf-8"))
        assert len(migration) == 25
        assert {entry["file"] for entry in migration.values()} == {
            path.name for path in (ROOT / "schemas").glob("*.json")
        }
        for path in (ROOT.parent / "v0.1.0" / "schemas").glob("*.json"):
            schema = json.loads(path.read_text(encoding="utf-8"))
            target = migration[schema["$id"]]
            schema["$id"] = target["schemaId"]
            assert schema == json.loads((ROOT / "schemas" / target["file"]).read_text(encoding="utf-8"))
        for old, new in zip(previous["operations"], operations, strict=True):
            assert old["publicCallable"] == new["publicCallable"]
            for kind in ("input", "output"):
                migrated = migration[old[f"{kind}Schema"]]
                assert migrated["schemaId"] == new[f"{kind}Schema"]
                assert migrated["file"] == new[f"{kind}SchemaFile"]
                assert migrated["schemaId"].startswith(OUTPUT_PREFIX + "skill:v0.2.0:")
    print(f"{ROOT.name} MAS_OWNER_CANDIDATE_SOURCE_SHAPE_PASS / NOT_QUALIFIED / NOT_DEPLOYABLE")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", choices=("v0.1.0", "v0.2.0"))
    selected = parser.parse_args().version
    for version in (selected,) if selected else ("v0.1.0", "v0.2.0"):
        verify(ROOT / version)
