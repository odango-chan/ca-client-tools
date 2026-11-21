# Typed MAS Provider v2 path

Status: **implemented in the Provider process; live Owner execution verified
against `env-openamsm` on 2026-11-21.** The typed path executes all 13
Establishment callables of the fixed table when the Owner can serve them. It
does not publish a mutation receipt, so the Provider never reports `COMMITTED`.

## What changed

- `catalog.HandlerMASInvoke` (`openamsmMASInvoke`) is a third typed handler next
  to the SMF context query and the metrics list. Its endpoint is fixed in source
  (`POST /nsmf-mas-orchestration/v1/invoke`); the catalog cannot express an
  arbitrary URL for it.
- `catalog.InstallTypedMASBindings` appends the fixed callable table derived from
  the same `mas.invokeBindings` projection that performs the wire mapping. A
  catalog binding therefore cannot drift from the step/mas/op/input/output
  contract: startup rejects an unknown callable. The reported catalog digest is
  extended with the typed closure.
- The Provider backend operation id is derived from the callable only:
  `openamsm.smf.mas.<lower-case callable>`.
- `provider.HostInvocation` is a new optional `invoke.params.hostInvocation`
  object. It carries the Host-private occurrence identity (`deploymentId`,
  `ownerInstanceId`, `originalInvocationId`, `sessionRef`, `smContextRef`,
  `supi`, `psi`). The Provider validates it strictly and accepts it only for
  the typed MAS handler; every other binding rejects it before any I/O. A typed
  MAS invocation without it is rejected.
- `provider.ContextualRuntime` lets the Provider server hand that scope to the
  runtime. Plain read-only bindings keep the unchanged `Runtime.Invoke` path.
- The idempotency key is derived from `(deploymentId, ownerInstanceId,
  originalInvocationId)`, so one Host occurrence always maps to one OpenAMSM
  idempotency key. The Provider owns no journal: the Host's durable dispatch
  claim remains the single dispatch authority.
- Owner results are returned as an observation envelope
  (`{"observation": {code, httpStatus, outputs}, "smContextRef": ...}`). HTTP
  200 and `status=OK` are never converted to `COMMITTED` here. Non-200 Owner
  responses keep their HTTP status and a stable observation code
  (`ACL_DENIED`, `OWNER_TIMEOUT`, `OWNER_ERROR`, `INVALID_RESPONSE`).

## Deployment configuration

| Environment | Meaning |
|---|---|
| `CA_OPENAMSM_MAS_INVOKE_ORIGIN` | Private OpenAMSM SMF SBI origin (h2c) |
| `CA_OPENAMSM_ORCH_TOKEN` | `X-OpenAMSM-Orch-Token` value for that endpoint |
| `CA_OPENAMSM_MAS_INVOKE_ORIGIN` + `CA_OPENAMSM_ORCH_TOKEN` | Must be set together |

Availability is reported per binding: without the pair all 21 typed callables
are `NOT_CONFIGURED`, and the read-only catalog is unchanged.

## Safety boundary

- No generic URL, method, header, token or stepId is accepted from a caller.
- The Provider stores no authoritative dispatch state and never retries.
- An Owner result is an observation. Only the Host may decide an Outcome, and
  the pinned Owner publishes no mutation receipt or exact-original Reconcile
  producer, so a Host that requires a receipt keeps the operation
  `OUTCOME_UNKNOWN`.
- The `openamsmMASInvoke` handler is the only mutation path that is not blocked
  by `MUTATION_GATED`; it exists only when the deployment configured the private
  endpoint and the Host supplied the exact occurrence scope.

## Verified live

`env-openamsm` (macOS/arm64, OpenAMSM 2.8.0, `plan_provider: agent_step`,
`orchestration.enabled: true`) served ten of the thirteen Establishment
callables through this path: s1, s2, s3, s3r, s3p, s4, s5, s6u, s7 and s8 all
returned HTTP 200 with schema-shaped outputs, including a real N4 session
(`n4_seid`). s10 `SSM.SIGNAL_UE_AN` reached the Owner but returned HTTP 502
`E_SOUTHBOUND` because no Agent-side Namf relay/N2 delivery was configured;
s11 and s13 were therefore not reached in that run. Host-side evidence lives in
`ca-agent-demo` `tests/integration/test_m1_real_mas_provider.py`.
