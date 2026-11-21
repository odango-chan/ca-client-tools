#!/usr/bin/env python3
"""Read-only fail-closed validation for unpublished MAS source-contract candidates."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
EXPECTED_V02 = {
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
EXPECTED_V03 = {
    **EXPECTED_V02,
    "m0": "SC.UPDATE_SM_CONTEXT",
    "m3": "UPN.MODIFY_UP_SESSION_INTENT",
    "m4": "SSM.SIGNAL_UE_AN_MODIFY",
    "r2": "UPN.DELETE_UP_SESSION",
    "r3": "SSM.SIGNAL_UE_AN_RELEASE",
    "r4": "PA.DELETE_SM_POLICY",
    "r5": "SC.RELEASE_SM_CONTEXT",
    "r6": "SE.EMIT_RELEASE_EVENT",
}
# The D5 QoS delta is a second typed profile on the same Owner step s11:
# the source plan exposes one MODIFY_UP_SESSION step with two callable shapes.
V03_ADDITIONAL_CALLABLES = {"s11": {"UPN.MODIFY_QOS_FLOWS"}}
OUTPUT_PREFIX = "urn:example:openamsm:mas:"
WTLAB_OWNER = "wtlab"
MAS_PACKAGE_ID = "wtlab:packages:openamsm:mas-compiler-contract"
WTLAB_V02_PREFIX = "wtlab:openamsm:mas:v0.2.0:"
WTLAB_V03_PREFIX = "wtlab:openamsm:mas:v0.3.0:"
SOURCE_COMMIT = "594dec78b95bff4fdc34deac8a3a9681fb41228f"


def load(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def schema_at(path: Path, identity: str, keys: list[str],
              required: list[str] | None = None) -> dict:
    data = load(path)
    assert data.get("$id") == identity, (path, "schema identity mismatch")
    assert data.get("$schema") == "https://json-schema.org/draft/2020-12/schema"
    assert data.get("type") == "object" and data.get("additionalProperties") is False
    assert data.get("required") == (keys if required is None else required), (
        path, "required names/order mismatch")
    assert set(data.get("properties", {})) == set(keys)
    for field in keys:
        assert isinstance(data["properties"][field], dict), (path, field)
    return data


def verify_provenance(root: Path, doc: dict, provenance: dict) -> None:
    assert provenance["status"] == "SOURCE_PROVENANCE_ONLY"
    assert provenance["repository"] == doc["source"]["repository"]
    assert provenance["commit"] == doc["source"]["commit"] == SOURCE_COMMIT
    sources = provenance["sources"]
    assert sources["openamsm"]["path"] == doc["source"]["openapi"]
    for source in sources.values():
        for name, component in source["components"].items():
            assert component["pointer"] == f"/components/schemas/{name}"
            assert component["standaloneId"] is None


def verify_legacy(root: Path) -> None:
    doc = load(root / "source-map.json")
    provenance = load(root / "schema-provenance.json")
    verify_provenance(root, doc, provenance)
    sources = provenance["sources"]
    for kind, count in (("input", 13), ("output", 12)):
        projection = provenance["projections"][kind]
        assert projection["glob"] == f"schemas/*.{kind}.schema.json"
        assert len(list(root.glob(projection["glob"]))) == projection["count"] == count
        assert projection["sourceComponent"] in sources["openamsm"]["components"]
        assert projection["sourceValuePointer"] == f"/properties/{kind}s"
    field_sources = provenance["projections"]["input"]["fieldSources"]
    assert set(field_sources) == {"ue_identity", "psi", "dnn", "sNssai"}
    assert set(field_sources.values()) == set(sources["threegpp"]["components"])
    assert doc["kind"] == "MASOwnerSourceContractCandidate"
    assert doc["status"] == "NOT_QUALIFIED" and doc["deployable"] is False
    owner = doc["owner"]
    if root.name == "v0.2.0":
        assert owner["ownerId"] == WTLAB_OWNER
        assert owner["stablePackageId"] == MAS_PACKAGE_ID
        assert owner["declaredOwnerAuthority"] == "DECLARED_IDENTITY_ONLY"
    else:
        assert owner["ownerId"] is None and owner["stablePackageId"] is None
        assert owner["declaredOwnerAuthority"] == "UNBOUND"
    assert all(owner[key] is False for key in (
        "publishedToolCatalog", "publishedCapabilityContracts",
        "publishedEvidenceGuarantees", "mutationReceiptProducer", "reconcileProducer"
    ))
    operations = doc["operations"]
    assert len(operations) == 13
    assert {o["stepId"] for o in operations} == set(EXPECTED_V02)
    assert len({o["publicCallable"] for o in operations}) == 13
    for item in operations:
        step = item["stepId"]
        assert item["publicCallable"] == EXPECTED_V02[step], step
        assert item["effectCandidate"] == "MUTATION_REVIEW_REQUIRED"
        assert item["receiptPublication"] == "ABSENT"
        assert item["ownerQualification"] == "NOT_QUALIFIED"
        assert item["capabilityCandidate"]
        prefix = WTLAB_V02_PREFIX if root.name == "v0.2.0" else OUTPUT_PREFIX
        assert item["inputSchema"].startswith(prefix)
        assert item["outputSchema"].startswith(prefix)
        authors = item["authorInputs"]
        mappings = item["argumentMapping"]
        assert len(mappings) == len(authors)
        assert [m["from"] for m in mappings] == authors
        mapped = {x for m in mappings for x in (
            m["to"] if isinstance(m["to"], list) else [m["to"]]
        )}
        assert set(item["oesInputs"]) - mapped == set(item["sourceOnlyInputs"])
        if step == "s7":
            assert "supi" in mapped and "supi" not in item["sourceOnlyInputs"]
        input_file = item.get("inputSchemaFile", f"{step}.input.schema.json")
        schema_at(root / "schemas" / input_file, item["inputSchema"], authors)
        output_id = item["outputSchema"]
        output_file = item.get("outputSchemaFile")
        if output_file is None:
            assert output_id.endswith(":v1")
            output_file = output_id.removeprefix(OUTPUT_PREFIX).removesuffix(":v1") + ".output.schema.json"
        private_outputs = item.get("privateOutputFields", [])
        assert private_outputs == (["sm_context_ref"] if step == "s3" else [])
        assert set(private_outputs) <= set(item["sourceOutputFields"])
        public_outputs = [f for f in item["sourceOutputFields"] if f not in private_outputs]
        schema_at(root / "schemas" / output_file, output_id, public_outputs)
    if root.name == "v0.2.0":
        migration = load(root / "schema-id-migration.json")
        previous = load(root.parent / "v0.1.0" / "source-map.json")
        assert len(migration) == 25
        assert {entry["file"] for entry in migration.values()} == {
            path.name for path in (root / "schemas").glob("*.json")
        }
        for path in (root.parent / "v0.1.0" / "schemas").glob("*.json"):
            schema = load(path)
            target = migration[schema["$id"]]
            schema["$id"] = target["schemaId"]
            assert schema == load(root / "schemas" / target["file"])
        for old, new in zip(previous["operations"], operations, strict=True):
            assert old["publicCallable"] == new["publicCallable"]
            for kind in ("input", "output"):
                migrated = migration[old[f"{kind}Schema"]]
                assert migrated["schemaId"] == new[f"{kind}Schema"]
                assert migrated["file"] == new[f"{kind}SchemaFile"]
                assert migrated["schemaId"].startswith(WTLAB_V02_PREFIX)
    print(f"{root.name} MAS_OWNER_CANDIDATE_SOURCE_SHAPE_PASS / NOT_QUALIFIED / NOT_DEPLOYABLE")


def verify_v03(root: Path) -> None:
    doc = load(root / "source-map.json")
    provenance = load(root / "schema-provenance.json")
    verify_provenance(root, doc, provenance)
    assert doc["kind"] == "MASOwnerSourceContractCandidate"
    assert doc["version"] == "0.3.0-candidate"
    assert doc["status"] == "NOT_QUALIFIED" and doc["deployable"] is False
    owner = doc["owner"]
    assert owner["ownerId"] == WTLAB_OWNER
    assert owner["stablePackageId"] == MAS_PACKAGE_ID
    assert owner["declaredOwnerAuthority"] == "DECLARED_IDENTITY_ONLY"
    assert all(owner[key] is False for key in (
        "publishedToolCatalog", "publishedCapabilityContracts",
        "publishedEvidenceGuarantees", "mutationReceiptProducer", "reconcileProducer"
    ))

    plans = doc["source"]["plans"]
    expected_plans = {
        "establish": ("upstream/openamsm/tools/nw-agent/plans/establish/pdu-internet-policy-first-v1.yaml",
                      "b28b20103258f2f106e66278a0bc9c78eef047a9"),
        "modify": ("upstream/openamsm/tools/nw-agent/plans/modify/pdu-mod-v3.yaml",
                   "8a2caf343c29bb64cbb09b29994c79ac8ad5da05"),
        "release": ("upstream/openamsm/tools/nw-agent/plans/release/pdu-rel-v4.yaml",
                    "849c3206569d452a663d1b8d42a63ffdc367be31"),
    }
    assert set(plans) == set(expected_plans)
    for name, (path, blob) in expected_plans.items():
        assert plans[name] == {"path": path, "blobSha": blob}
    assert doc["source"]["openapiBlobSha"] == "87083bb1ecd700240b97e6455ebde3e33c54d6e9"
    assert doc["source"]["implementationBlobs"] == {
        "upnode.c": "0993e8ed35c691dc7dc76a682b5f1b4d23fa38cc",
        "ssm_ue.c": "e59acacea03532a102d699edbbd7a0a07662f8f9",
        "session_context.c": "c7763919785870f4350b8779b59563dc64533a5e",
        "policy_adapter.c": "110924c66cfe3455407c3ad4cdffb3e3292338b1",
        "session_exposure.c": "bafdc069522eb1e2c8f0a3c08eed28745abbcd48",
        "xact.h": "bbdb879d217fe7ec360a661ac22a5b08865e115c",
    }

    operations = doc["operations"]

    def callables_for(step: str) -> set[str]:
        return {EXPECTED_V03[step]} | V03_ADDITIONAL_CALLABLES.get(step, set())

    expected_callables = {
        callable_id for step in EXPECTED_V03 for callable_id in callables_for(step)
    }
    assert len(operations) == 22
    assert {o["stepId"] for o in operations} == set(EXPECTED_V03)
    assert {o["publicCallable"] for o in operations} == expected_callables
    assert len({o["publicCallable"] for o in operations}) == 22
    old_ops = {o["publicCallable"]: o for o in load(root.parent / "v0.2.0" / "source-map.json")["operations"]}
    for item in operations:
        step = item["stepId"]
        assert item["publicCallable"] in callables_for(step), step
        assert item["effectCandidate"] == "MUTATION_REVIEW_REQUIRED"
        assert item["receiptPublication"] == "ABSENT"
        assert item["ownerQualification"] == "NOT_QUALIFIED"
        assert item["capabilityCandidate"] and item["evidenceGuaranteeCandidate"]
        assert item["sourcePlan"] in plans
        assert item["inputSchema"].startswith((WTLAB_V02_PREFIX, WTLAB_V03_PREFIX))
        assert item["outputSchema"].startswith((WTLAB_V02_PREFIX, WTLAB_V03_PREFIX))
        authors = item["authorInputs"]
        mappings = item["argumentMapping"]
        assert [m["from"] for m in mappings] == authors
        mapped = {x for m in mappings for x in (
            m["to"] if isinstance(m["to"], list) else [m["to"]]
        )}
        assert set(item["oesInputs"]) - mapped == set(item["sourceOnlyInputs"])
        input_file = item["inputSchemaFile"]
        input_path = root / "schemas" / input_file
        if not input_path.exists():
            input_path = root.parent / "v0.2.0" / "schemas" / input_file
        schema_at(input_path, item["inputSchema"], authors)
        output_file = item["outputSchemaFile"]
        output_path = root / "schemas" / output_file
        if not output_path.exists():
            output_path = root.parent / "v0.2.0" / "schemas" / output_file
        private_outputs = item.get("privateOutputFields", [])
        assert private_outputs == (["sm_context_ref"] if step == "s3" else [])
        assert set(private_outputs) <= set(item["sourceOutputFields"])
        public_outputs = [f for f in item["sourceOutputFields"] if f not in private_outputs]
        optional_outputs = item.get("optionalOutputFields", [])
        assert set(optional_outputs) <= set(public_outputs)
        required_outputs = [f for f in public_outputs if f not in optional_outputs]
        schema_at(output_path, item["outputSchema"], public_outputs,
                  required=required_outputs)
        if item["publicCallable"] in old_ops:
            previous = old_ops[item["publicCallable"]]
            for direction in ("input", "output"):
                assert item[f"{direction}Schema"] == previous[f"{direction}Schema"]
                assert item[f"{direction}SchemaFile"] == previous[f"{direction}SchemaFile"]
        else:
            assert item["inputSchema"].startswith(WTLAB_V03_PREFIX)

    schema_counts = doc["schemaCounts"]
    assert schema_counts == {
        "input": 22, "output": 20, "total": 42,
        "inheritedFromV0_2_0": 25, "newInV0_3_0": 17
    }
    new_schema_ids = {
        item[key]
        for item in operations if item["publicCallable"] not in old_ops
        for key in ("inputSchema", "outputSchema")
        if item[key].startswith(WTLAB_V03_PREFIX)
    }
    assert len(new_schema_ids) == 17
    assert len(list((root / "schemas").glob("*.input.schema.json"))) == 9
    assert len(list((root / "schemas").glob("*.output.schema.json"))) == 8
    pattern_schemas = {"upn-modify-qos-flows.input.schema.json"}
    for path in (root / "schemas").glob("*.schema.json"):
        data = load(path)
        if path.name in pattern_schemas:
            # The QoS delta is the one profile that validates decimal lexical
            # form (QFI 1-63, 1..13 digit bitrates); the Owner repeats the
            # check authoritatively.
            assert set(data["properties"]) == {
                "ue_identity", "psi", "anTunnelTeid", "anTunnelIp", "n4Seid",
                "qosFlowsDelta", "qfi", "fiveQi", "gfbrUl", "gfbrDl",
                "mfbrUl", "mfbrDl",
            }
            for field in ("qfi", "gfbrUl", "gfbrDl", "mfbrUl", "mfbrDl"):
                assert "pattern" in data["properties"][field]
            continue
        assert all("pattern" not in value for value in data.get("properties", {}).values())
    assert provenance["projections"]["input"]["count"] == 9
    assert provenance["projections"]["output"]["count"] == 8
    assert provenance["inheritedProjections"] == {
        "sourceVersion": "0.2.0", "inputCount": 13, "outputCount": 12,
        "totalCount": 25, "byteIdentical": True
    }
    assert provenance["totalProjections"] == {
        "inputCount": 22, "outputCount": 20, "totalCount": 42
    }
    assert provenance["newProjectionCounts"] == {"input": 9, "output": 8, "total": 17}

    inheritance = load(root / "schema-inheritance.json")
    assert inheritance["sourceVersion"] == "0.2.0" and inheritance["byteIdentical"] is True
    inherited = {x["schemaId"]: x["file"] for x in inheritance["schemas"]}
    assert len(inheritance["schemas"]) == len(inherited) == 25
    v02_doc = load(root.parent / "v0.2.0" / "source-map.json")
    v02_schema_ids = {
        item[key]
        for item in v02_doc["operations"]
        for key in ("inputSchema", "outputSchema")
    }
    assert set(inherited) == v02_schema_ids
    v02 = root.parent / "v0.2.0" / "schemas"
    for schema_id, filename in inherited.items():
        data = load(v02 / filename)
        assert data["$id"] == schema_id
        assert not (root / "schemas" / filename).exists()

    bindings = doc["procedureBindings"]
    expected_bindings = {
        ("modify-pdu-session-mas", "m0", "modify", "m0", "SC.UPDATE_SM_CONTEXT"),
        ("modify-pdu-session-mas", "m3", "modify", "m3", "UPN.MODIFY_UP_SESSION_INTENT"),
        ("modify-pdu-session-mas", "m4", "modify", "m4", "SSM.SIGNAL_UE_AN_MODIFY"),
        ("modify-pdu-session-mas", "m5", "modify", "m5", "UPN.MODIFY_UP_SESSION"),
        ("release-pdu-session-mas", "r2", "release", "r2", "UPN.DELETE_UP_SESSION"),
        ("release-pdu-session-mas", "r3", "release", "r3", "SSM.SIGNAL_UE_AN_RELEASE"),
        ("release-pdu-session-mas", "r4", "release", "r4", "PA.DELETE_SM_POLICY"),
        ("release-pdu-session-mas", "r5", "release", "r5", "SC.RELEASE_SM_CONTEXT"),
        ("release-pdu-session-mas", "r6", "release", "r6", "SE.EMIT_RELEASE_EVENT"),
    }
    assert len(bindings) == 9
    assert {(b["skill"], b["node"], b["sourcePlan"], b["sourceStepId"], b["publicCallable"])
            for b in bindings} == expected_bindings
    by_callable = {o["publicCallable"]: o for o in operations}
    for binding in bindings:
        op = by_callable[binding["publicCallable"]]
        assert binding["inputSchema"] == op["inputSchema"]
        assert binding["outputSchema"] == op["outputSchema"]
    s11 = by_callable["UPN.MODIFY_UP_SESSION"]
    m5 = next(b for b in bindings if b["node"] == "m5")
    assert m5["inputSchema"] == s11["inputSchema"]
    assert m5["outputSchema"] == s11["outputSchema"]
    m4 = by_callable["SSM.SIGNAL_UE_AN_MODIFY"]
    assert m4["outputSchema"] == by_callable["SSM.SIGNAL_UE_AN"]["outputSchema"]

    for step in ("r3", "r4", "r6"):
        item = next(o for o in operations if o["stepId"] == step)
        output_path = root / "schemas" / item["outputSchemaFile"]
        empty = schema_at(output_path, item["outputSchema"], [])
        assert empty["properties"] == {}
    m0 = next(o for o in operations if o["stepId"] == "m0")
    assert load(root / "schemas" / m0["outputSchemaFile"])["properties"]["lifecycle"]["const"] == "OPERATIONAL"
    m3 = next(o for o in operations if o["stepId"] == "m3")
    assert load(root / "schemas" / m3["inputSchemaFile"])["properties"]["upIntent"]["const"] == "1"
    r2 = next(o for o in operations if o["stepId"] == "r2")
    assert load(root / "schemas" / r2["outputSchemaFile"])["properties"]["released"]["const"] == "1"
    r5 = next(o for o in operations if o["stepId"] == "r5")
    assert load(root / "schemas" / r5["outputSchemaFile"])["properties"]["lifecycle"]["const"] == "RELEASING"

    assert doc["sourceDefects"] == [{
        "sourcePlan": "release", "field": "trigger", "declaredStartInputs": [],
        "referencedBy": ["r2", "r3"], "status": "BLOCKS_LINKING",
        "resolution": "Upstream OES plan must declare a typed start input; Owner must not synthesize a default."
    }]
    exclusions = {(x["sourcePlan"], x["stepId"], x["operation"]) for x in doc["exclusions"]}
    assert exclusions == {
        ("modify", "m2", "CA.UPDATE_CHARGING"),
        ("release", "r1", "CA.DELETE_CHARGING"),
        ("release", "r7", "COMPENSATE_RELEASE"),
    }
    print("v0.3.0 MAS_OWNER_LIFECYCLE_CANDIDATE_PASS / NOT_QUALIFIED / NOT_DEPLOYABLE / NOT_LINKABLE")


def verify(root: Path) -> None:
    if root.name == "v0.3.0":
        verify_v03(root)
    else:
        verify_legacy(root)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", choices=("v0.1.0", "v0.2.0", "v0.3.0"))
    selected = parser.parse_args().version
    for version in (selected,) if selected else ("v0.1.0", "v0.2.0", "v0.3.0"):
        verify(ROOT / version)
