# OpenAMSM MAS Owner Contract Candidate v0.3.0

**Status: SOURCE-ALIGNED CANDIDATE ONLY · NOT_QUALIFIED · NOT_DEPLOYABLE · NOT_LINKABLE.**

This version extends the Establishment candidate v0.2.0 with the source-aligned Modify and Release callable profiles required by ca-skills v1alpha5. It is not a trusted Owner ToolCatalog, signed Owner publication, Deployment CapabilityContract, EvidenceTrustBinding, or production Provider catalog.

## Package contents

- **22 unique operation callables**: the original 13 Establishment callables plus 9 new lifecycle callables (including the dedicated `UPN.MODIFY_QOS_FLOWS` QoS-delta profile).
- **42 distinct JSON Schemas**: 25 inherited byte-identically from v0.2.0 and 17 new v0.3.0 projections.
- **9 Procedure-to-OES occurrence bindings**: Modify m0/m3/m4/m5 and Release r2–r6.
- Pinned source map, plan provenance, Schema inheritance record, static contract closure and Owner outcome requirements.

Modify m5 does **not** reuse the Establishment s11 `UPN.MODIFY_UP_SESSION` (`n3_synced`) profile. It has the dedicated `UPN.MODIFY_QOS_FLOWS` callable on the same Owner step s11, with a closed QFI/5QI/GBR/MBR input and a canonical decimal target output (`APPLIED` / `ALREADY` / `PFCP_APPLIED_N1N2_FAILED`). Modify m3 has a separate `UPN.MODIFY_UP_SESSION_INTENT` callable; it is not a widened version of m5. Modify m4 and Release r3 use distinct SSM callable profiles. The Establishment SSM, SE and s11 schemas are not widened.

## Source baseline

All OES plans and implementation references are pinned to `odango-chan/env-openamsm@594dec78b95bff4fdc34deac8a3a9681fb41228f`.

- Establishment: `pdu-internet-policy-first-v1.yaml`
- Modify: `pdu-mod-v3.yaml`
- Release: `pdu-rel-v4.yaml`

Exact blob SHAs and C implementation provenance are recorded in `source-map.json` and `schema-provenance.json`.

## Lifecycle profile boundaries

| Procedure node | Callable | Public result |
| --- | --- | --- |
| Modify m0 | `SC.UPDATE_SM_CONTEXT` | `lifecycle=OPERATIONAL` |
| Modify m3 | `UPN.MODIFY_UP_SESSION_INTENT` | `n4_seid` |
| Modify m4 | `SSM.SIGNAL_UE_AN_MODIFY` | Existing v0.2.0 AN tunnel tuple |
| Modify m5 | Existing `UPN.MODIFY_UP_SESSION` | Existing `n3_synced` |
| Release r2 | `UPN.DELETE_UP_SESSION` | `released=1` only after PFCP deletion completion |
| Release r3 | `SSM.SIGNAL_UE_AN_RELEASE` | Closed empty object |
| Release r4 | `PA.DELETE_SM_POLICY` | Closed empty object |
| Release r5 | `SC.RELEASE_SM_CONTEXT` | `lifecycle=RELEASING` |
| Release r6 | `SE.EMIT_RELEASE_EVENT` | Closed empty object |

The OES source plan's step IDs remain private Owner binding data. They are not additional Skill arguments. The CA-facing `ue_identity` and `psi` values are Scope bindings; where an OES step declares no inputs, the Owner validates Scope without forwarding invented OES inputs.

## Known blocker

The pinned Release plan declares `start_inputs: {}` but r2 and r3 reference `from_start: trigger`. The upstream OES plan must declare the typed trigger input before these callables can be linked or qualified. The candidate does not synthesize a default or patch the source plan.

## Publication and implementation boundary

The declared stable package identity is `wtlab:packages:openamsm:mas-compiler-contract`; the declared Owner ID is `wtlab`. This records naming only. Publisher authorization, signed package publication, trusted ToolCatalog exports, Evidence Guarantees, CapabilityContracts and EvidenceTrustBindings are not present.

The production Provider still exposes only the existing read-only catalog. This candidate does not register new write bindings or open the mutation gate. No Owner commit receipt, durable exact-original Reconcile, Host-private Update/Release admission, Runtime acceptance, or live 5GC E2E is claimed.

See [static contract closure](static-contract-closure.md), [Owner outcome contract](owner-outcome-contract.md), [publication naming](publication-naming.md), and the repository [release procedure](../../../docs/RELEASE.md#mas-contract-archives).

## QoS outcome representation

`UPN.MODIFY_QOS_FLOWS` publishes three candidate states. The typed Provider only
projects a state together with its required evidence:

- `APPLIED` must carry `n1n2_dispatched=1` (PFCP QER accepted and the native
  N1/N2 Modify handed to the relay).
- `ALREADY` and `PFCP_APPLIED_N1N2_FAILED` must not carry
  `n1n2_dispatched`. Numeric target fields are canonical decimal strings on
  both the input and output side; the Provider rejects a sign, a leading zero,
  an out-of-range QFI/5QI/bitrate, or `GBR > MBR`.

The current OpenAMSM Owner reports an N1/N2 dispatch failure as an error
(`SMF_MAS_E_MAS` / HTTP 502 problem) without outputs, so in this version the
Provider maps it to `OWNER_ERROR` and `PFCP_APPLIED_N1N2_FAILED` is not
delivered as a successful output. It is never a success path: an Owner error
cannot produce `WIRE_OK`, and the Skill Postcondition additionally rejects the
state. A future Owner that returns it with a complete target output is accepted
as a non-success observation only.
