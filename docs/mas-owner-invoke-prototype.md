# OpenAMSM MAS Owner Invoke prototype

The private Go adapter implements the 21 fixed callable mappings from the v0.3.0
source candidate, with closed input and output projections. Production
Provider v2 registration and activation require separate qualification.

The authorized Host supplies the versioned private context with deployment,
SMF Owner instance, original invocation ID, stable session reference, exact
SUPI and PSI, and optional verified SM context reference. The adapter checks
scope against the Skill arguments and sends the known flat fields. S-NSSAI is
flattened to SST and optional SD. The orchestration token comes from deployment
configuration.

The Host journal must atomically bind the session scope and reserve the exact
operation before sending, persisting the original ID, canonical request
digest and idempotency key. The key is SHA-256 of a JSON array containing
deployment ID, Owner instance ID and original invocation ID. A changed
request under the same original ID conflicts. The claim survives crashes,
timeouts and cancellation. Duplicate dispatch is prohibited even after the
OpenAMSM replay cache or session binding table disappears. Tests exercise the
journal interface using an in-memory fixture.

Reconcile reads durable proof for the exact original occurrence from the Host
journal without making an SBI call. Committed and not-committed decisions
require verified Owner proof. Deferred requires an authenticated durable
pending handle. A parked SSM HTTP request supplies no such handle. An explicit
source ACL rejection is denied. Other HTTP failures, timeouts, cancellation,
invalid outputs and raw HTTP 200/OK leave the commit state unknown. If the
observation cannot be recorded, the claim still prevents replay.

The SM context reference stays private. Address and readiness markers retain
their source meaning. Charging produces a skip marker and SE emits a local
event. The deadline is at most 30 seconds, including journal interactions
when the implementation honors context cancellation.

Following ca-skills PR #74, Host authenticates and correlates the initial
CreateSMContext ingress and arbitrates duplicates and competing writers.
There is no extra AMF registration, CM CONNECTED or generic SMF Session ABSENT
gate. s1 can materialize a session shell.


## v0.3.0 lifecycle extension

The fixed callable table now covers all 21 unique callable IDs in the combined
v0.3.0 candidate. Eight lifecycle-specific bindings are added:

- Modify: m0 `SC.UPDATE_SM_CONTEXT`, m3
  `UPN.MODIFY_UP_SESSION_INTENT`, and m4
  `SSM.SIGNAL_UE_AN_MODIFY`.
- Release: r2 `UPN.DELETE_UP_SESSION`, r3
  `SSM.SIGNAL_UE_AN_RELEASE`, r4 `PA.DELETE_SM_POLICY`, r5
  `SC.RELEASE_SM_CONTEXT`, and r6 `SE.EMIT_RELEASE_EVENT`.

Modify m5 deliberately reuses the existing s11 `UPN.MODIFY_UP_SESSION`
callable because the source input/output profile is identical. The tests read
the v0.3.0 source map and its exact input Schema filenames, falling back only
to the immutable v0.2.0 Schema directory for inherited profiles.

Release trigger is accepted only as an explicit Host-supplied JSON integer in
the closed range 1-7. No default is synthesized. The pinned Release OES plan
still declares no `start_inputs.trigger` while r2/r3 consume it, so the
Release Procedure remains blocked from linking and qualification until the
upstream plan is corrected.

Current state (2026-11-21): the fixed 21-callable table is registered in the
real Provider v2 process as the typed `openamsmMASInvoke` handler and passed
development interop against real OpenAMSM 2.8.0. This is **not** a production
qualification. The candidate Owner still publishes no authenticated mutation
receipt or exact-original Reconcile decision producer, so the Provider continues
to return observations only: it never maps `HTTP 200`/`WIRE_OK` to `COMMITTED`,
never retries, and keeps no authoritative journal. A Host that requires a
receipt keeps the operation `OUTCOME_UNKNOWN`. Production publication remains
blocked until the Owner receipt producer, exact-original recovery and live
acceptance gates are independently met.
