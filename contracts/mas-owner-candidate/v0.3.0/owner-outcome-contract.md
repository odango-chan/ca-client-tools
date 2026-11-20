# MAS Owner Outcome and Receipt Contract Candidate

**Status: REQUIRED OWNER CONTRACT / NOT IMPLEMENTED / NOT_PUBLISHED.**

This document defines the minimum Owner behavior required before any MAS lifecycle callable can become a trusted ToolCatalog operation. It is not evidence that ca-client-tools Provider or OpenAMSM currently implements it.

## Identity and immutable invocation binding

For every Mutation occurrence, Owner must bind the Host-owned original Invocation identity to:

- stable package ID and exact public callable ID;
- private OES plan and step occurrence;
- exact SUPI, PSI and private SMF session generation;
- authenticated Owner identity and deployment context;
- canonical input digest and Schema version;
- commit state, exact public output and Evidence;
- source observation time and receipt identity.

Private session references, OES step IDs, generation IDs, idempotency keys, invocation IDs, stream handles and transport correlation IDs never become Skill-authored arguments or public outputs.

## Outcome semantics

| State | Meaning | Required handling |
| --- | --- | --- |
| SUCCEEDED | Exact original Mutation is committed; declared output is complete and valid; required Evidence is available for the same occurrence and Scope. | Return exact output and a verifiable commit receipt. |
| FAILED | Owner can prove the exact original Mutation did not commit and left no residual side effect. | Return only a published failure class and proof. Never infer from HTTP error or timeout. |
| DENIED | Host/Runtime rejects authorization or Scope before Owner dispatch. | No Owner Mutation is dispatched. |
| OUTCOME_UNKNOWN | Commit state cannot be established, including timeout after dispatch, process restart, lost receipt or mismatched callback. | Preserve the original responsibility; do not retry automatically. |
| COMMITTED without recoverable output | Commit is proven but exact public output/Evidence cannot be recovered. | Not SUCCEEDED. Keep unresolved until exact-original recovery exists. |

`COMMITTED` and `NOT_COMMITTED` are commit states inside the private receipt, not standalone Skill Outcomes. There is no generic Owner NOT_COMMITTED outcome. FAILED is allowed only when NOT_COMMITTED and absence of residual side effects are independently proven. Raw HTTP 2xx, MAS `status=OK`, local markers and a missing response are not commit proof.

The private receipt must include at least `originalInvocationId`, `commitState`, callable identity, private occurrence identity, Scope, canonical input digest, output digest, observedAt and Evidence references. It must be immutable and verifiable by the Host/Runtime. Receipt metadata is not added to public Operation output Schemas.

## Exact-original Reconcile

A future Reconcile callable, if published, must be a separate trusted READ operation accepting only the original Invocation identity plus the minimum private binding needed to locate its receipt. It may inspect or recover the exact original operation; it must never dispatch the mutation again, compensate it, or create a new Run.

Reconcile must distinguish:
- original operation committed with recoverable exact output and Evidence;
- original operation proven not committed with no residual side effect;
- original operation still pending;
- original operation unknown or unrecoverable.

The current OpenAMSM in-memory idempotency/session tables are not durable reconciliation. A local retry or re-invocation is not Reconcile.

## Pending operations and callbacks

m4/m5 Modify and r2/r3/r4/r5 Release may wait for external network or NF completion. Pending state must be durable and correlated to the original Host Invocation, callable, OES occurrence, SUPI, PSI and generation. A late callback may complete only that original occurrence. It cannot satisfy a newer Run or a different generation.

A duplicate delivery of the same original Invocation returns the original settled result and receipt without a second dispatch. Reusing the same idempotency key with different input is rejected. OUTCOME_UNKNOWN never triggers a blind retry, compensation or replan.

## Evidence requirements

Evidence must be produced by an identified authoritative source, bound to exact callable and original Invocation, exact SUPI/PSI/generation Scope, and include a real observedAt and freshness policy. Evidence for a prior session state cannot prove a new Mutation occurrence.

For r3/r4/r6 the public output is a closed empty object. Their Evidence must be recorded in the private receipt / Owner evidence channel, not smuggled into output fields. r6's source-local in-memory marker is not durable Evidence by itself.

## Activation gates

Before a trusted Owner snapshot may be published:

1. Implement durable Owner receipts and exact-original Reconcile.
2. Implement fixed typed Provider bindings and validate every request and response against its closed Schema.
3. Prove the Release trigger start input is declared by the pinned upstream OES plan.
4. Implement durable callback continuation for pending SSM/PFCP/PCF/SC work.
5. Verify exact Owner identity and publisher authorization.
6. Have Deployment publish per-callable CapabilityContracts and EvidenceTrustBindings.
7. Run duplicate, unknown-outcome, wrong-generation, late-callback and isolated live 5GC acceptance.

Until all gates close, every candidate callable remains MUTATION_REVIEW_REQUIRED and NOT_LINKABLE / NOT_QUALIFIED.
