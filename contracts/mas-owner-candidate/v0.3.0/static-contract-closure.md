# MAS v0.3.0 lifecycle static contract closure

**Status: SOURCE-CONTRACT REVIEW BASELINE / NOT_QUALIFIED / NOT_LINKABLE.**

This document closes the source mapping needed by ca-skills v1alpha5 Modify and Release. It does not claim a working Provider binding or a published Owner.

## Exact callable and occurrence mapping

| Procedure | Node | OES step | Callable | OES inputs | Public output |
| --- | --- | --- | --- | --- | --- |
| Modify | m0 | SC.UPDATE_SM_CONTEXT | SC.UPDATE_SM_CONTEXT | none | lifecycle |
| Modify | m3 | UPN.MODIFY_UP_SESSION | UPN.MODIFY_UP_SESSION_INTENT | up_intent='1' | n4_seid |
| Modify | m4 | SSM.SIGNAL_UE_AN | SSM.SIGNAL_UE_AN_MODIFY | none | an_tunnel_teid, an_tunnel_ip, an_tunnel |
| Modify | m5 | UPN.MODIFY_UP_SESSION | Existing UPN.MODIFY_UP_SESSION | an_tunnel_teid, an_tunnel_ip, n4_seid | n3_synced |
| Release | r2 | UPN.DELETE_UP_SESSION | UPN.DELETE_UP_SESSION | trigger | released |
| Release | r3 | SSM.SIGNAL_UE_AN | SSM.SIGNAL_UE_AN_RELEASE | release='1', trigger | none |
| Release | r4 | PA.DELETE_SM_POLICY | PA.DELETE_SM_POLICY | release='1' | none |
| Release | r5 | SC.RELEASE_SM_CONTEXT | SC.RELEASE_SM_CONTEXT | release='1' | lifecycle |
| Release | r6 | SE.EMIT_EVENTS | SE.EMIT_RELEASE_EVENT | ev='RELEASED' | none |

The v0.3.0 catalog adds eight unique callable IDs. Modify m5 is a ninth Procedure occurrence but reuses the exact v0.2.0 UPN modify callable and Schemas already used by Establishment s11. The occurrence-to-step binding remains private to the Owner.

## Input and Scope projection

All lifecycle callable inputs are closed objects. `ue_identity` is the admitted SUPI and `psi` is the PDU Session ID (1–15). They bind the exact Owner Scope. For m0 and m4, the OES plan declares no inputs: these fields are checked by the Owner and are not forwarded as fabricated OES arguments.

- m3 forwards only `up_intent='1'` beyond Scope.
- m4 has a distinct no-business-input Modify profile and reuses only the exact AN tunnel output Schema.
- r2 forwards the numeric trigger.
- r3 forwards `release='1'` and numeric trigger.
- r4/r5 forward `release='1'`.
- r6 forwards `ev='RELEASED'`.

Release trigger values are the pinned OpenAMSM constants: 1 LOCAL_INITIATED, 2 UE_REQUESTED, 3 PCF_INITIATED, 4 RAN_INITIATED, 5 SMF_INITIATED, 6 AMF_RELEASE_SM_CONTEXT, 7 AMF_UPDATE_SM_CONTEXT. Host derives entryKind and trigger from authenticated SBI ingress or a trusted internal event. The caller cannot select them.

## Commit and evidence boundaries

Every callable is a candidate MUTATION. A successful raw MAS `status=OK` or HTTP 2xx is not a commit receipt. Owner SUCCEEDED must bind the original Invocation, exact callable and OES occurrence, SUPI, PSI, private session generation, input digest, commit state, observed time and Evidence.

- m0 Evidence must prove the exact existing SM Context became OPERATIONAL.
- m3 Evidence must prove the no-tunnel intent branch was applied and the returned N4 SEID belongs to this occurrence. It does not prove N3 synchronization.
- m4 Evidence must prove the matching AN result and the returned tunnel tuple for this Modify occurrence.
- m5 Evidence must prove PFCP modification completion for the exact N4 SEID and AN tunnel; `n3_synced` alone is not sufficient without exact-occurrence proof.
- r2 Evidence must prove PFCP Session Deletion completed. Do not return `released=1` when only the request was sent.
- r3 Evidence must distinguish the source-defined no-signal branches (triggers 1, 6, 7) from branches requiring matching AMF release confirmation (triggers 2, 3, 4, 5).
- r4 Evidence must prove the exact PCF association was deleted or its absence was verified for the same Scope. No public `terminated` field exists.
- r5 exposes `lifecycle=RELEASING`; it does not claim final RELEASED or a Host ResourceMap tombstone.
- r6 proves only the local in-memory MAS release marker. It is not durable persistence, an external event delivery, or a 3GPP SM Context Status Notify.

r3, r4 and r6 have closed empty output objects. Their Evidence must bind to the exact empty result root and the private Owner receipt; no synthetic public output fields may be introduced.

## Optional steps and exclusions

- Modify m2 CA.UPDATE_CHARGING is optional with no source-defined applicability predicate. It is omitted. The pinned adapter only writes a local marker and does not reserve CHF charging.
- Release r1 CA.DELETE_CHARGING is optional with no source-defined applicability predicate. It is omitted. The pinned adapter only clears a local marker and does not delete a CHF resource.
- Release r7 COMPENSATE_RELEASE is `execute: skip`. It is not a callable. No automatic compensation, retry or replan is imported.

## Known upstream blocker

The pinned `pdu-rel-v4.yaml` declares `start_inputs: {}` while r2 and r3 reference `from_start: trigger`. This is a source-plan defect. It blocks executable Owner binding and linking until the upstream plan declares a typed trigger input. Do not invent a default, patch the plan locally, or bypass its validation.

## Host and Provider boundary

The Provider currently has no production typed write binding for these callables. The Host owns authenticated ingress classification, exact SUPI/PSI/generation Scope, stable business-session reference, original Invocation identity and durable dispatch journal. Provider owns only fixed transport bindings and exact wire/output validation; it is not the authorization or commit-proof authority.

The MAS session and idempotency state in OpenAMSM are process-local. A Provider timeout after possible dispatch is OUTCOME_UNKNOWN. No blind retry is permitted. SSM operations that wait for AMF callbacks require durable original-occurrence continuation; the existing Provider request deadline is not such a continuation mechanism.

## Qualification

Static source and Schema shape can be checked in CI. Owner receipt, exact-original Reconcile, callback races, duplicate delivery, generation isolation, Host admission, Runtime authorization and live 5GC scenarios are NOT_RUN. This package remains NOT_LINKABLE / NOT_QUALIFIED.
