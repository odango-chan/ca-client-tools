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
