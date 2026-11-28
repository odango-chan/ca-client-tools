"""Materialize synthetic Compiler inputs. Never use the output in release mode."""

import argparse
import json
import shutil
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("output", type=Path, help="new fixture project directory")
parser.add_argument("--version", choices=("v0.1.0", "v0.2.0", "v0.3.0"), default="v0.1.0")
parser.add_argument("--with-scheme-c", action="store_true",
                    help="also materialize the development Scheme C Owner callables")
args = parser.parse_args()
root = args.output
root.mkdir(parents=True, exist_ok=False)
candidate = Path(__file__).resolve().parents[1] / args.version
owner = "fixture-mas-owner"
fixture = "urn:example:openamsm:mas:fixture"
draft = "https://json-schema.org/draft/2020-12/schema"

def write(name, data):
    target = root / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")

def schema(id_, required, properties):
    return {"$schema": draft, "$id": id_, "type": "object",
            "additionalProperties": False, "required": required, "properties": properties}

source_map = json.loads((candidate / "source-map.json").read_text())
ops = source_map["operations"]
mas_package_id = source_map["owner"].get("stablePackageId") or \
    "urn:example:connectionagent:openamsm-mas-tool-contracts"
dst = root / "snapshots/mas/schemas"
dst.mkdir(parents=True, exist_ok=True)
for source_dir in ([candidate.parent / "v0.2.0" / "schemas", candidate / "schemas"]
                   if args.version == "v0.3.0" else [candidate / "schemas"]):
    for path in source_dir.glob("*.schema.json"):
        target = dst / path.name
        if target.exists():
            raise ValueError(f"duplicate fixture Schema filename: {path.name}")
        shutil.copyfile(path, target)

if args.with_scheme_c:
    fixtures = Path(__file__).resolve().parent
    extra_ops = json.loads((fixtures / "scheme-c-operations.json").read_text())["operations"]
    for path in (fixtures / "scheme-c-schemas").glob("*.json"):
        target = dst / path.name
        if target.exists():
            raise ValueError(f"duplicate fixture Schema filename: {path.name}")
        shutil.copyfile(path, target)
    ops = ops + extra_ops

proof = fixture + ":commit-proof:v1"
receipt = fixture + ":receipt:v1"
write("snapshots/mas/proof.schema.json", schema(proof, ["originalInvocationId", "commitState"], {
    "originalInvocationId": {"type": "string", "minLength": 1},
    "commitState": {"type": "string", "enum": ["COMMITTED", "NOT_COMMITTED"]}}))
write("snapshots/mas/receipt.schema.json", schema(receipt, ["commit"], {
    "commit": {"$ref": proof}, "failureClass": {"type": "string"}}))
write("snapshots/ca-context/subject.schema.json",
      schema("urn:connectionagent:ca:context:subject:v1", ["supi"],
             {"supi": {"type": "string", "minLength": 1}}))

catalog, guarantees, capability_map = [], {}, {}
for op in ops:
    callable_id = op["publicCallable"]
    gid = op["evidenceGuaranteeCandidate"]
    effect = "READ" if op.get("effectCandidate") == "READ" else "MUTATION"
    if effect == "READ":
        catalog.append({"id": callable_id, "kind": "operation", "effect": "READ",
                        "ownerOutcomes": ["SUCCEEDED", "FAILED"],
                        "inputSchema": {"$ref": op["inputSchema"]},
                        "outputSchema": {"$ref": op["outputSchema"]},
                        "guarantees": {"evidence": {"provides": [{"guarantee": gid, "path": "output"}]}}})
    else:
        catalog.append({"id": callable_id, "kind": "operation", "effect": "MUTATION",
                        "ownerOutcomes": ["SUCCEEDED", "FAILED"], "inputSchema": {"$ref": op["inputSchema"]},
                        "outputSchema": {"$ref": op["outputSchema"]}, "receiptSchema": {"$ref": receipt},
                        "mutation": {
                            "commitProof": {"schema": proof, "path": "receipt.commit",
                                            "bind": "originalInvocationId"},
                            "failureClassPath": "receipt.failureClass",
                            "failureClasses": [{
                                "id": "fixture.proven-no-effect", "outcome": "FAILED",
                                "proofSchema": proof, "proofPath": "receipt.commit",
                                "noSideEffect": True, "retryable": False}]},
                        "guarantees": {"evidence": {"provides": [{"guarantee": gid, "path": "output"}]}}})
    guarantees[gid] = {
        "authority": owner, "fact": gid,
        "producers": [{"owner": owner, "kind": "operation", "id": callable_id,
                       "outcome": "SUCCEEDED", "path": "output"}],
        "scope": {
            "subject": {"record": "scope.subject", "equals": "arguments.ue_identity"},
            "originalProducer": {"record": "scope.originalProducer",
                                 "equals": "execution.producerInvocationId"}},
        "correlation": {"invocation": {"record": "invocationId", "equals": "execution.invocationId"},
                        "producerInvocation": {"record": "producerInvocationId",
                                               "equals": "execution.producerInvocationId"}},
        "factValue": "value", "selection": {"mode": "exact", "orderBy": "observedAt"},
        "freshness": {"timestamp": "observedAt", "maxAgeMs": 10000}}
    if "psi" in op["authorInputs"]:
        guarantees[gid]["scope"]["pduSession"] = {
            "record": "scope.pduSession", "equals": "arguments.psi"}
    capability_map.setdefault(op["capabilityCandidate"], []).append(
        {"owner": owner, "id": callable_id, "kind": "operation",
         "effect": effect, "scopePolicyRef": "fixture.mas-scope"})
write("snapshots/mas/tool-catalog.yaml", {"apiVersion": "connectionagent.io/toolcatalog/v1alpha4",
      "kind": "ToolCatalog", "metadata": {"owner": owner}, "spec": {"contracts": catalog}})
write("snapshots/mas/evidence-guarantees.yaml", guarantees)
capabilities = [{"id": capability_id, "owner": "fixture-deployment", "callables": callables}
                for capability_id, callables in sorted(capability_map.items())]
write("capability/capability-contracts.yaml", {"apiVersion": "connectionagent.io/capability-contracts/v1alpha4",
      "kind": "CapabilityContracts", "spec": {"capabilities": capabilities}})
write("trusted.json", {"dependencyBindings": [
      {"dependencyId": "mas", "stablePackageId": mas_package_id,
       "authorizedOwners": [owner]},
      {"dependencyId": "ca-context", "stablePackageId": "urn:connectionagent:ca:host-context",
       "authorizedOwners": ["fixture-context-owner"]}],
      "deployment": {"deploymentId": fixture + ":deployment", "publisherId": fixture + ":publisher",
                     "authorizationRef": fixture + ":authorization", "capabilityDirectory": "capability"}})
