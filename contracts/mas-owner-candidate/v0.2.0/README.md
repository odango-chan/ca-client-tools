# OpenAMSM MAS Schema Candidate v0.2.0

**Status: NOT_QUALIFIED · NOT_DEPLOYABLE · NOT_LINKABLE.** This is a source-derived, versioned **candidate**, not a published trusted ToolCatalog, CapabilityContracts, Evidence Guarantee or authorized deployment.

This repository currently implements only the 11 read-only Provider v2 bindings. The 13 write-capable MAS operations have no shipped typed Provider implementation, no trusted Owner commit receipt and no deployed Reconcile endpoint. These files must not be embedded into the production Provider or accepted as a real Owner snapshot.

The [static contract review](static-contract-closure.md) records the proposed Owner semantics and the compiler-only fixture. The separate [private Invoke prototype](https://github.com/odango-chan/ca-client-tools/pull/14) consumes this candidate.

The [publication naming proposal](publication-naming.md) and [source registry](schema-provenance.json) distinguish upstream definitions from Skill-facing projections. The [ID migration map](schema-id-migration.json) maps all 25 v0.1.0 IDs to the v0.2.0 IDs and files. Schema constraints and operation identities are unchanged.

## Downloads

- Review all 25 projections in one OpenAPI 3.1 file, `openamsm-mas-skill-projections-v0.2.0.openapi.yaml`. The components preserve WTLab Schema IDs. The pinned wire API has a separate OpenAPI document.
- Get 3GPP definitions upstream from [3GPP Forge 5G APIs](https://forge.3gpp.org/rep/all/5G_APIs). Match the source version recorded in `schema-provenance.json`; the candidate catalog does not duplicate 3GPP Common Data.
- Use the separately packaged JSON Schema files for Compiler v2.0.2. An OpenAPI-only snapshot was tested and reports `SCHEMA_MISSING`, as v2.0.2 links standalone Schema roots by `$id`.

Maintainers can regenerate the catalog from the JSON Schema sources with
`python contracts/mas-owner-candidate/fixtures/export_openapi.py <output.yaml>`
using PyYAML.

## Scope and provenance

- OpenAMSM source: `odango-chan/env-openamsm@594dec78b95bff4fdc34deac8a3a9681fb41228f`.
- Policy-first plan: `upstream/openamsm/tools/nw-agent/plans/establish/pdu-internet-policy-first-v1.yaml`.
- API: `POST /nsmf-mas-orchestration/v1/invoke`, fixed MAS/op/stepId, secret header `X-OpenAMSM-Orch-Token`.
- The 13 candidate Capability IDs match the corrected MAS Skill in `ca-skills` PR #74.
- Exact 13 steps match the first IPv4/SSC1/LOCAL_POOL/no-secondary-auth profile. This candidate intentionally excludes conditional s9 and compensation s14.

`source-map.json` defines all 13 operations and their **separate** CA-facing input names, pinned OES plan field names, expected output field names, candidate evidence guarantees and not-yet-verified mutation effect. For each entry:

- Input and output files use operation names, for example `sc-create-sm-context.input.schema.json`. The source map resolves each operation and OES step to its exact files and IDs.
- PA CREATE and UPDATE share `pa-sm-policy.output.schema.json`. The source map records raw output fields separately, including the private s3 reference.
- OpenAMSM projection IDs use the project URI namespace `wtlab:openamsm:mas:skill:v0.2.0:`. The repository archives and packages their definitions. Authorized Compiler snapshots and grants are separate from the Schema IDs.
- `argumentMapping` is a review map, not executable transformation. `scopeOnly` means a Skill argument is not passed in the pinned OES step's inputs; `sourceOnlyInputs` is a raw plan field not directly provided by the current Skill (for example optional GPSI). A trustworthy adapter must implement, validate and test every required mapping. Do not silently discard authorization/identity fields.

## Why a formal MAS ToolCatalog cannot be published yet

OpenAMSM's MAS Invoke API response contains `status: OK`, `outputs`, `mas`, `op`, `stepId`. It supplies no `receipt.commit` binding `originalInvocationId` to `COMMITTED` / `NOT_COMMITTED`, independently verified no-side-effect failure class, or durable exact-occurrence Reconcile callable. The ToolCatalog refers to that receipt via `mutation.commitProof`. The server's session and idempotency tables are process-local.

Before issuing a signed/authorized Owner snapshot, implement and test a real Owner receipt and reconciliation protocol, verify the per-operation `READ` / `MUTATION` classification (SC.VALIDATE_SUBSCRIPTION can allocate an SMF session shell), and establish Source/Owner evidence authority, scope/freshness and deployment CapabilityContracts. Never turn `status: OK` into a fabricated receipt.

## Risk register and acceptance

1. **s1 pre-create side effect** — the MAS endpoint can create a session shell on admission. ca-skills PR #74 removed the ABSENT Observation. Host handles authentication and exact ingress correlation without an extra AMF or SMF query.
2. **s10 pending HTTP stream** — SSM.SIGNAL_UE_AN can wait for a later AMF UpdateSMContext; the current Provider 30-second deadline is not durable Owner completion.
3. **s7 charging** — current `NOT_IMPL_skip` returns a marker, not a genuine charging reservation.
4. **s13 event** — source-local event only, not 3GPP Step 18.
5. **Security** — Host-private `sessionRef`, `smContextRef`, `idempotencyKey` and orchestration token never become Skill inputs or output schemas.

Release checklist: (a) real typed Provider handler shipped with negative wire tests; (b) Owner receipt and any declared recovery demonstrated against OpenAMSM; (c) actual Owner identity and stable package authoritatively published; (d) real ToolCatalog/Schema/Evidence exports and separate deployment CapabilityContracts issued; (e) replace placeholder package and Schema references in the canonical Skill with the published identities, then run full Check/Inspect/Build with a released Compiler carrying the fix from PR #47 and immutable snapshots; (f) independently verify Host admission and live E2E for deployment qualification.

Related: [ca-client-tools #10](https://github.com/odango-chan/ca-client-tools/issues/10), [ca-skills #71](https://github.com/odango-chan/ca-skills/issues/71), [Demo #215](https://github.com/odango-chan/ca-agent-demo/issues/215).
