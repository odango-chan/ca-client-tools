#!/usr/bin/env python3
"""Declare the Scheme C typed MAS operations as development Compiler inputs.

This is fixture material only: the operations below are the merged OpenAMSM
Owner occurrences that a CA-SL Procedure can call, but their Owner
ToolCatalog / schema / Evidence publication does not exist yet. The fixture
builder turns this file into a synthetic trusted closure so the real Compiler
can build and the Host can admit a *development* artifact.

Run this script to (re)write `scheme-c-operations.json` and the schema files.
"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
VERSION = "v0.4.0"
SCHEMA_DIR = ROOT / "scheme-c-schemas"
DRAFT = "https://json-schema.org/draft/2020-12/schema"


def schema(identity: str, properties: dict, required: list[str]) -> dict:
    return {
        "$schema": DRAFT,
        "$id": identity,
        "type": "object",
        "additionalProperties": False,
        "required": required,
        "properties": properties,
    }


def text(**extra) -> dict:
    value = {"type": "string", "minLength": 1, "maxLength": 255}
    value.update(extra)
    return value


def const(value: str) -> dict:
    return {"type": "string", "const": value}


def integer(**extra) -> dict:
    value = {"type": "integer"}
    value.update(extra)
    return value


OPS = [
    {
        "name": "ssm-parse-n1-modify",
        "stepId": "c-parse",
        "mas": "SSM",
        "op": "SIGNAL_UE_AN",
        "publicCallable": "SSM.PARSE_N1_MODIFY",
        "capabilityCandidate": "mas.ssm.n1.modify.parse",
        "evidenceGuaranteeCandidate": "ssm.n1-modify-parsed",
        "effect": "READ",
        "sourcePlan": "modify",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "parse_n1": const("1"),
            "n1_sm": {"type": "string", "minLength": 8, "maxLength": 1024},
        },
        "outputs": {
            "qfi": text(maxLength=2),
            "five_qi": text(maxLength=3),
            "gfbr_ul": text(maxLength=13),
            "gfbr_dl": text(maxLength=13),
            "mfbr_ul": text(maxLength=13),
            "mfbr_dl": text(maxLength=13),
            "requested_kind": const("qos_flow_descriptions"),
        },
        "optional": [],
    },
    {
        "name": "upn-modify-qos-delta",
        "stepId": "c-apply",
        "mas": "UPN",
        "op": "MODIFY_UP_SESSION",
        "publicCallable": "UPN.MODIFY_QOS_DELTA",
        "capabilityCandidate": "mas.upn.up.qos.delta",
        "evidenceGuaranteeCandidate": "upn.qos-delta-applied",
        "effect": "MUTATION",
        "sourcePlan": "modify",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "qos_flows_delta": const("1"),
            "qfi": text(maxLength=2),
            "five_qi": text(maxLength=3),
            "gfbr_ul": text(maxLength=13),
            "gfbr_dl": text(maxLength=13),
            "mfbr_ul": text(maxLength=13),
            "mfbr_dl": text(maxLength=13),
        },
        "outputs": {
            "qos_delta_state": {
                "type": "string",
                "enum": ["APPLIED", "ALREADY", "PFCP_APPLIED_N1N2_FAILED"],
            },
            "n1n2_dispatched": const("1"),
            "qfi": text(maxLength=2),
            "five_qi": text(maxLength=3),
            "gfbr_ul": text(maxLength=13),
            "gfbr_dl": text(maxLength=13),
            "mfbr_ul": text(maxLength=13),
            "mfbr_dl": text(maxLength=13),
            "n4_seid": text(maxLength=20),
        },
        "optional": ["n1n2_dispatched", "n4_seid"],
    },
    {
        "name": "ssm-parse-n1-release",
        "stepId": "r-parse",
        "mas": "SSM",
        "op": "SIGNAL_UE_AN",
        "publicCallable": "SSM.PARSE_N1_RELEASE",
        "capabilityCandidate": "mas.ssm.n1.release.parse",
        "evidenceGuaranteeCandidate": "ssm.n1-release-parsed",
        "effect": "READ",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "parse_n1": const("1"),
            "n1_sm": {"type": "string", "minLength": 8, "maxLength": 1024},
        },
        "outputs": {
            "psi": text(maxLength=2),
            "pti": text(maxLength=3),
            "cause": text(maxLength=3),
            "release_intent": const("ue_requested"),
        },
        "optional": ["cause"],
    },
    {
        "name": "ssm-build-release-command",
        "stepId": "r-build",
        "mas": "SSM",
        "op": "SIGNAL_UE_AN",
        "publicCallable": "SSM.BUILD_RELEASE_COMMAND",
        "capabilityCandidate": "mas.ssm.n1.release.build",
        "evidenceGuaranteeCandidate": "ssm.release-command-built",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "release": const("1"),
            "mode": const("build"),
            "trigger": {"type": "integer", "enum": [1, 2, 3, 4, 5, 6, 7]},
            "release_txn": text(maxLength=127),
        },
        "outputs": {
            "n1_sm_hex": {"type": "string", "minLength": 2, "maxLength": 1024},
            "n2_sm_hex": {"type": "string", "minLength": 2, "maxLength": 8192},
            "n2_sm_info_type": const("PDU_RES_REL_CMD"),
            "release_generation": text(maxLength=10),
            "release_txn": text(maxLength=127),
            "release_phase": const("command_built"),
        },
        "optional": [],
    },
    {
        "name": "ssm-release-confirm-n1",
        "stepId": "r-confirm-n1",
        "mas": "SSM",
        "op": "SIGNAL_UE_AN",
        "publicCallable": "SSM.RELEASE_CONFIRM_N1",
        "capabilityCandidate": "mas.ssm.n1.release.confirm.n1",
        "evidenceGuaranteeCandidate": "ssm.release-n1-confirmed",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "release_confirm": const("n1"),
            "release_txn": text(maxLength=127),
            "release_generation": text(maxLength=10),
        },
        "outputs": {
            "release_ready": text(maxLength=1),
            "n1_released": text(maxLength=1),
            "n2_released": text(maxLength=1),
        },
        "optional": [],
    },
    {
        "name": "ssm-release-confirm-n2",
        "stepId": "r-confirm-n2",
        "mas": "SSM",
        "op": "SIGNAL_UE_AN",
        "publicCallable": "SSM.RELEASE_CONFIRM_N2",
        "capabilityCandidate": "mas.ssm.n1.release.confirm.n2",
        "evidenceGuaranteeCandidate": "ssm.release-n2-confirmed",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "release_confirm": const("n2"),
            "release_txn": text(maxLength=127),
            "release_generation": text(maxLength=10),
        },
        "outputs": {
            "release_ready": text(maxLength=1),
            "n1_released": text(maxLength=1),
            "n2_released": text(maxLength=1),
        },
        "optional": [],
    },
    {
        "name": "pa-delete-sm-policy",
        "stepId": "r-pa",
        "mas": "PA",
        "op": "DELETE_SM_POLICY",
        "publicCallable": "PA.DELETE_SM_POLICY_C",
        "capabilityCandidate": "mas.pa.sm.policy.delete",
        "evidenceGuaranteeCandidate": "pa.policy-association-released",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "release": const("1"),
            "trigger": {"type": "integer", "enum": [1, 2, 3, 4, 5, 6, 7]},
        },
        "outputs": {"terminated": const("1")},
        "optional": [],
    },
    {
        "name": "se-emit-release-event",
        "stepId": "r-se",
        "mas": "SE",
        "op": "EMIT_EVENTS",
        "publicCallable": "SE.EMIT_RELEASE_EVENT_C",
        "capabilityCandidate": "mas.se.events.emit",
        "evidenceGuaranteeCandidate": "se.release-event-marker-written",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "ev": const("RELEASED"),
        },
        "outputs": {"last_event": const("1")},
        "optional": [],
    },
    {
        "name": "sc-release-sm-context",
        "stepId": "r-sc",
        "mas": "SC",
        "op": "RELEASE_SM_CONTEXT",
        "publicCallable": "SC.RELEASE_SM_CONTEXT_C",
        "capabilityCandidate": "mas.sc.context.release",
        "evidenceGuaranteeCandidate": "sc.context-release-committed",
        "effect": "MUTATION",
        "sourcePlan": "release",
        "inputs": {
            "ue_identity": {"type": "string", "pattern": "^imsi-[0-9]{1,15}$"},
            "psi": integer(minimum=1, maximum=15),
            "release": const("1"),
            "finalize": const("1"),
            "trigger": {"type": "integer", "enum": [1, 2, 3, 4, 5, 6, 7]},
        },
        "outputs": {
            "lifecycle": const("RELEASING"),
            "user_plane_ready": const("0"),
            "sm_context_ref": text(maxLength=127),
        },
        "optional": ["sm_context_ref"],
    },
]


def main() -> None:
    SCHEMA_DIR.mkdir(parents=True, exist_ok=True)
    operations = []
    for op in OPS:
        input_id = f"wtlab:openamsm:mas:{VERSION}:{op['name']}-input"
        output_id = f"wtlab:openamsm:mas:{VERSION}:{op['name']}-result"
        input_file = f"scheme-c-{op['name']}.input.schema.json"
        output_file = f"scheme-c-{op['name']}.output.schema.json"
        (SCHEMA_DIR / input_file).write_text(
            json.dumps(schema(input_id, op["inputs"], list(op["inputs"])), indent=2) + "\n"
        )
        output_required = [
            name for name in op["outputs"] if name not in op["optional"]
        ]
        (SCHEMA_DIR / output_file).write_text(
            json.dumps(schema(output_id, op["outputs"], output_required), indent=2) + "\n"
        )
        author_inputs = list(op["inputs"])
        operations.append({
            "stepId": op["stepId"],
            "mas": op["mas"],
            "op": op["op"],
            "publicCallable": op["publicCallable"],
            "capabilityCandidate": op["capabilityCandidate"],
            "evidenceGuaranteeCandidate": op["evidenceGuaranteeCandidate"],
            "sourceOperation": "masOrchestrationInvoke",
            "effectCandidate": op["effect"],
            "receiptPublication": "ABSENT",
            "ownerQualification": "NOT_QUALIFIED",
            "inputSchema": input_id,
            "outputSchema": output_id,
            "authorInputs": author_inputs,
            "oesInputs": author_inputs,
            "argumentMapping": [{"from": name, "to": name} for name in author_inputs],
            "sourceOnlyInputs": [],
            "sourceOutputFields": [name for name in op["outputs"] if name not in op["optional"]],
            "inputSchemaFile": input_file,
            "outputSchemaFile": output_file,
            "sourcePlan": op["sourcePlan"],
        })
    (ROOT / "scheme-c-operations.json").write_text(
        json.dumps({"operations": operations}, indent=2) + "\n"
    )
    print(f"[PASS] declared {len(operations)} Scheme C operations ({VERSION})")


if __name__ == "__main__":
    main()
