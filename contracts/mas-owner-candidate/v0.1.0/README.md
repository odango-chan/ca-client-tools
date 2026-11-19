# OpenAMSM MAS Owner source-contract candidate v0.1.0

**Status: NOT_QUALIFIED · NOT_DEPLOYABLE · NOT_LINKABLE.** This is a source-derived, versioned **candidate**, not a published trusted ToolCatalog, CapabilityContracts, Evidence Guarantee or authorized deployment.

This repository currently implements only the 11 read-only Provider v2 bindings. The 13 write-capable MAS operations have no shipped typed Provider implementation, no trusted Owner commit receipt and no deployed Reconcile endpoint. These files must not be embedded into the production Provider or accepted as a real Owner snapshot.

The [private Invoke prototype](../../../docs/mas-owner-invoke-prototype.md) exercises these mappings without registering them in Provider v2.
The [static contract closure](static-contract-closure.md) records the proposed Owner semantics and the compiler-only fixture.

## Scope and provenance

- OpenAMSM source: `odango-chan/env-openamsm@594dec78b95bff4fdc34deac8a3a9681fb41228f`.
- Policy-first plan: `upstream/openamsm/tools/nw-agent/plans/establish/pdu-internet-policy-first-v1.yaml`.
- API: `POST /nsmf-mas-orchestration/v1/invoke`, fixed MAS/op/stepId, secret header `X-OpenAMSM-Orch-Token`.
- Candidate public Capability IDs match the 14 grammar-corrected candidates in `ca-skills` PR #72 (13 MAS plus the separate SMF context read).
- Exact 13 steps match the first IPv4/SSC1/LOCAL_POOL/no-secondary-auth profile. This candidate intentionally excludes conditional s9 and compensation s14.

`source-map.json` defines all 13 operations and their **separate** CA-facing input names, pinned OES plan field names, expected output field names, candidate evidence guarantees and not-yet-verified mutation effect. For each entry:

- `schemas/<step>.input.schema.json` describes the current Skill-facing arguments (NOT the raw OpenAMSM wire).
- `schemas/<name>.output.schema.json` describes the **normalized public result projection expected by the Skill**. PA CREATE and UPDATE share one result schema ID. The source map records the raw output fields separately, including the private s3 reference.
- Every `$id` is an `urn:example:...` placeholder and therefore **cannot** be treated as a real published Schema ID.
- `argumentMapping` is a review map, not executable transformation. `scopeOnly` means a Skill argument is not passed in the pinned OES step's inputs; `sourceOnlyInputs` is a raw plan field not directly provided by the current Skill (for example optional GPSI). A trustworthy adapter must implement, validate and test every required mapping. Do not silently discard authorization/identity fields.

## Why a formal MAS ToolCatalog cannot be published yet

OpenAMSM's MAS Invoke API response contains `status: OK`, `outputs`, `mas`, `op`, `stepId`. It does **not** provide the Compiler-required `receipt.commitProof` binding `originalInvocationId` to `COMMITTED` / `NOT_COMMITTED`, an independently verified no-side-effect failure class, nor a durable exact-occurrence Reconcile callable. The server's session and idempotency tables are process-local. Therefore a ToolCatalog declaring mutation commitProof would be materially false until the Owner/Provider implements it.

Before issuing a signed/authorized Owner snapshot, implement and test a real Owner receipt and reconciliation protocol, verify the per-operation `READ` / `MUTATION` classification (SC.VALIDATE_SUBSCRIPTION can allocate an SMF session shell), and establish Source/Owner evidence authority, scope/freshness and deployment CapabilityContracts. Never turn `status: OK` into a fabricated receipt.

## Risk register and acceptance

1. **s1 pre-create side effect** — the MAS endpoint can create a session shell on admission. ca-skills PR #74 removed the ABSENT Observation. Host handles authentication and exact ingress correlation without an extra AMF or SMF query.
2. **s10 pending HTTP stream** — SSM.SIGNAL_UE_AN can wait for a later AMF UpdateSMContext; the current Provider 30-second deadline is not durable Owner completion.
3. **s7 charging** — current `NOT_IMPL_skip` returns a marker, not a genuine charging reservation.
4. **s13 event** — source-local event only, not 3GPP Step 18.
5. **Security** — Host-private `sessionRef`, `smContextRef`, `idempotencyKey` and orchestration token never become Skill inputs or output schemas.

Release checklist: (a) real typed Provider handler shipped with negative wire tests; (b) Owner receipt + recovery demonstrated against OpenAMSM; (c) **actual** Owner identity and stable package authoritatively published; (d) real ToolCatalog/Schema/Evidence exports and separate deployment CapabilityContracts issued; (e) Compiler v2.0.1 full Check/Build from immutable snapshots; (f) Host admission and real E2E independently verified. Only after these gates may the status change to `OWNER_QUALIFIED`.

Related: [ca-client-tools #10](https://github.com/odango-chan/ca-client-tools/issues/10), [ca-skills #71](https://github.com/odango-chan/ca-skills/issues/71), [Demo #215](https://github.com/odango-chan/ca-agent-demo/issues/215).
