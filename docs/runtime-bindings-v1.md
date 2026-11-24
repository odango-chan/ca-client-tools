# Runtime-only Provider binding sidecar (v1)

Status: **implemented release contract**. First published by `ca-client-tools`
v1.2.0. Tracked by [ca-client-tools#37](https://github.com/odango-chan/ca-client-tools/issues/37).

The sidecar is the thin, private execution mapping between the Host Runtime and
the Provider process. It is **not** a public ToolCatalog:

- it does not appear in the Compiler Interface Package or in
  `interface/capabilities.json`;
- installing the Provider still grants no Skill any Capability;
- a Skill can only use a binding after Root admission and the per-Root
  Capability/Operation allowlist authorize it.

The public READ contract published by v1.1.0 is unchanged.

## 1. Why it exists

The Owner (`env-openamsm`) publishes the authoritative ToolCatalog
(`urn:openamsm:mas:tool-contracts`, `v1alpha4`): Callable identity, Effect, Owner
Outcomes, input/output Schema, capability membership, receipt, commit proof and
exact-original reconcile. The Provider (`ca-client-tools`) implements those
Callables through one private Go dispatch table.

The Host cannot derive the private side:

- `catalog` needs explicit `backendOperationId`s; it has no enumeration;
- private backend IDs must never be reconstructed from names;
- the Provider observation envelope and the location of the Owner receipt are
  Provider protocol details;
- the Host invocation scope and private argument slots are Host/Provider private.

The sidecar publishes exactly those facts. It never redefines Owner business
semantics: every business field is either a reference to the pinned Owner
publication or a private execution fact read from the reviewed Go table.

## 2. Location and identity

`provider.yaml` references the sidecar as a pair:

```yaml
runtimeBindingsPath: runtime-bindings.yaml
runtimeBindingsDigest: sha256:<exact file bytes>
```

Both are required together. The Host resolves the path inside the selected
package, rejects symlinks/escapes, and verifies the digest before parsing. The
sidecar is a top-level package file; it is included in every platform archive
and is never included in the standalone Interface Package ZIP.

Top level:

```yaml
apiVersion: ca.tool-provider-runtime-bindings/v1
providerId: ca-client-tools.sbi
providerVersion: 1.2.0
ownerContract:
  stableId: urn:openamsm:mas:tool-contracts
  version: v1alpha4
  publicationDigest: sha256:<Owner manifest publicationDigest>
  manifestDigest: sha256:<exact manifest.json bytes>
  operations: 25
files:
  - path: runtime/schemas/<name>.json
    sha256: sha256:<exact file bytes>
bindings: [...]
```

`files` covers every package-relative Schema the bindings reference. A file that
is not referenced, or a reference not covered by a digest, fails closed.

## 3. Binding

```yaml
- runtimeBindingId: sc-create-sm-context
  runtimeOperationId: SC.CREATE_SM_CONTEXT      # Runtime identity
  backendOperationId: openamsm.smf.mas.sc.create_sm_context
  ownerCallableId: SC.CREATE_SM_CONTEXT         # must equal runtimeOperationId
  effect: MUTATION                              # Owner Effect
  capabilityId: mas.sc.context.create           # Owner capability reference
  ownerOutcomes: [SUCCEEDED, FAILED, NOT_COMMITTED]
  inputSchema:  {$ref: runtime/schemas/sc-create-sm-context.input.schema.json}
  outputSchema: {$ref: runtime/schemas/sc-create-sm-context.output.schema.json}
  hostInvocation: {...}
  privateArguments: []
  observation: {...}                            # MUTATION and typed READ
  commitProof: {...}                            # MUTATION only
```

Rules:

- `runtimeBindingId`, `runtimeOperationId` and `backendOperationId` are unique.
- `effect` is exactly `READ` or `MUTATION`; `RECONCILE`/`UNKNOWN` are Host
  outcomes, not Provider Effects.
- `ownerOutcomes` may not include `COMMITTED`.
- The Schema exports are deterministic inline expansions of the pinned Owner
  Schema; URN `$ref`s are resolved at release time, and no external `$ref`
  remains. Their file digests are published in `files`.
- A READ binding declares no commit proof. A MUTATION binding requires an
  observation envelope, a Host invocation scope and a commit-proof contract.

## 4. Host invocation scope and private arguments

```yaml
hostInvocation:
  version: openamsm.mas-host-invocation/v1
  arguments:
    - {argument: ue_identity, scopeField: supi}
    - {argument: psi, scopeField: psi}
  fields:
    - {name: deploymentId, source: HOST_CONTEXT, required: true}
    - {name: ownerInstanceId, source: HOST_CONTEXT, required: true}
    - {name: originalInvocationId, source: DERIVED_ORIGINAL_INVOCATION_ID, required: true}
    - {name: sessionRef, source: HOST_CONTEXT, required: true}
    - {name: supi, source: HOST_CONTEXT, required: true}
    - {name: psi, source: HOST_CONTEXT, required: true, type: integer, minimum: 1, maximum: 15}
    - {name: smContextRef, source: HOST_CONTEXT, required: false}
privateArguments:
  - {name: n1_sm, source: PRIVATE_MATERIAL}
```

`HOST_CONTEXT` values come from the admitted invocation context (or static
deployment identity); `DERIVED_ORIGINAL_INVOCATION_ID` is the exact Runtime
invocation identity; `PRIVATE_MATERIAL` is Host-retained material that no Skill
may author. `arguments` pins a Skill-authored argument to a Host scope value: the
Host rejects the call before dispatch when they differ.

`privateArguments` may not intersect the binding's input Schema properties. The
Host injects them **after** Runtime input validation, so a Skill can never
supply them.

## 5. Observation envelope

```yaml
observation:
  envelopePath: response.observation
  codeField: code
  outputsField: outputs
  outcomes:
    WIRE_OK: RECEIPT        # MUTATION: verify the Owner receipt
    ACL_DENIED: DENIED
    OWNER_TIMEOUT: UNKNOWN
    OWNER_ERROR: UNKNOWN
    TRANSPORT_ERROR: UNKNOWN
    TIMEOUT: UNKNOWN
    CANCELED: UNKNOWN
    INVALID_RESPONSE: UNKNOWN
```

For a typed READ binding `WIRE_OK` maps to `OUTPUTS` instead. Any code not
declared is `OUTCOME_UNKNOWN`; the Host never infers an outcome from an HTTP
status. `DENIED`/`FAILED` are terminal negative dispositions; `UNKNOWN` routes
the call to exact-original reconcile material.

## 6. Owner receipt, commit proof and reconcile

```yaml
commitProof:
  mode: OWNER_RECEIPT
  receiptSchema: {$ref: runtime/schemas/mutation-receipt.schema.json}
  receipt:
    path: receipt                      # relative to the observation envelope
    schemaVersionField: schemaVersion
    schemaVersion: openamsm.mas-mutation-receipt/v1
    invocationIdField: originalInvocationId
    stateField: state
    states: {COMMITTED: COMMITTED, NOT_COMMITTED: NOT_COMMITTED, UNKNOWN: UNKNOWN}
    bindings:
      - {field: operation.callable, source: RUNTIME_OPERATION_ID}
      - {field: operation.mas, source: LITERAL, value: SC}
      - {field: operation.op, source: LITERAL, value: CREATE_SM_CONTEXT}
      - {field: operation.stepId, source: LITERAL, value: s3}
      - {field: owner.deploymentId, source: HOST_SCOPE, hostScopeField: deploymentId}
      - {field: owner.instanceId, source: HOST_SCOPE, hostScopeField: ownerInstanceId}
      - {field: scope.supi, source: HOST_SCOPE, hostScopeField: supi}
      - {field: scope.psi, source: HOST_SCOPE, hostScopeField: psi}
    commit:
      path: commit
      stateField: commitState
      committedValue: COMMITTED
      invocationIdField: originalInvocationId
  reconcile:
    bindingId: mas-reconcile
    arguments:
      - {field: originalInvocationId, source: ORIGINAL_INVOCATION_ID}
      - {field: deploymentId, source: HOST_SCOPE, hostScopeField: deploymentId}
      - {field: ownerInstanceId, source: HOST_SCOPE, hostScopeField: ownerInstanceId}
      - {field: operator, source: LITERAL, value: ca-agent-host}
      - {field: reason, source: LITERAL, value: exact-original reconcile}
    decision:
      path: response.decision
      resolvedField: resolved
      stateField: state
      receiptPath: receipt
```

The Host reports `COMMITTED` only when every receipt binding matches the exact
invocation and Host scope, the declared receipt state maps to `COMMITTED`, and the
Owner commit proof binds the same invocation. A well-formed receipt whose state
is `UNKNOWN` stays `OUTCOME_UNKNOWN`. Reconcile reuses the original invocation
identity, never dispatches the mutation again, and accepts only the declared
decision with a receipt that binds that same original identity.

`DEVELOPMENT_NO_RECEIPT` remains a valid mode for a Provider that cannot publish
an Owner receipt. A binding in that mode can never produce `COMMITTED`.

## 7. Release generation and gates

The release generator (`cmd/ca-client-tools-release`) reads the pinned Owner
publication from `contracts/owner/openamsm-mas/v1alpha4/` and the reviewed Go
tables, then:

1. verifies every Owner artifact against `manifest.json`;
2. requires every Owner-published business Callable to exist in Go with the same
   Effect, and every Go Callable not published to be on the audited legacy list
   (`SC.UPDATE_SM_CONTEXT`, `UPN.MODIFY_UP_SESSION_INTENT`,
   `SSM.SIGNAL_UE_AN_MODIFY`, `SSM.SIGNAL_UE_AN_RELEASE`, `PA.DELETE_SM_POLICY`,
   `SC.RELEASE_SM_CONTEXT`, `SE.EMIT_RELEASE_EVENT`, plus `UPN.MODIFY_QOS_FLOWS`);
3. requires the Provider/Owner input field closure to match (private argument
   slots excepted);
4. emits 25 runtime bindings (21 MUTATION + 4 READ: two Owner business READs and
   `MAS.RECEIPT_QUERY`/`MAS.RECONCILE`), their Schema exports and the sidecar;
5. proves each `backendOperationId` resolves in the Provider catalog with the
   same Effect;
6. writes `provider.yaml` with the sidecar path + digest.

The public Interface Package is unchanged: 11 READ Tools, 2 Capabilities,
4 Schemas. A binding that would add a public Tool fails the shipped-catalog
validation.
