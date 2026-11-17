# SBI Capability and Provider Interoperability Contract

**Status:** DESIGN DRAFT — for cross-repository review; no runtime implementation is claimed.  
**Applies to:** ca-client-tools, ca-skills and ca-agent-demo.  
**Normative boundary:** This document does not modify or supersede the frozen CA-SL connectionagent.io/v1alpha3 language contract. Where this document and CA-SL differ, CA-SL and its versioned ToolCatalog / CapabilityContracts contracts prevail.

## 1. Purpose

Define how a Skill that grants one or more SBI-related Capabilities can use a trusted SBI client Provider without giving the Skill or Agent arbitrary HTTP access.

The intended result is:

- the Skill declares business-level Capability Grants and references exact callable exports;
- the deployment supplies trusted CapabilityContracts;
- the Tool Owner publishes callable contracts in ToolCatalog;
- ca-client-tools implements a bounded subset of those callables through its SBI Operation Binding Catalog, HTTP/2 transport and Payload Engine;
- the Host exposes only the exact callables used by the admitted Skill and permitted by deployment policy;
- Runtime remains responsible for per-call admission, authorization, Effect, Outcome, Evidence and Reconcile.

This is an interoperability design, not a claim that all 3GPP SBI operations are implemented by the current OpenAMSM source.

## 2. Critical identity distinction

The words Capability, callable, Tool and Provider implementation identify different things. They MUST NOT be collapsed.

| Identity | Authority | Meaning |
|---|---|---|
| Dependency Alias | Skill Contract / resolved dependency snapshot | Local name selecting one exact immutable dependency snapshot |
| Callable identity | ToolCatalog Owner | Exact exported callable: (owner, kind, id) |
| Capability ID | Deployment CapabilityContracts | Authorization group containing an explicit set of exact callables and Scope policies |
| Provider identity | Installed deployment package | Executable implementation package, such as ca-client-tools.sbi |
| Implementation binding | Trusted deployment registration | Selects which installed Provider implementation serves one exact callable |

### 2.1 Reading ca::sbi.udm.sm_data.get

Under the frozen CA-SL qualified-reference syntax:

- ca is the Dependency Alias;
- sbi.udm.sm_data.get is the exported callable ID;
- the full ca::sbi.udm.sm_data.get is a qualified callable reference;
- it is **not, by itself, a Capability ID**.

The Skill's capabilities list contains Capability IDs. Those IDs are resolved through the deployment's trusted CapabilityContracts. A Capability ID may use an sbi.* naming convention if the deployment explicitly publishes it, but the prefix has no automatic authorization meaning.

Do not infer Capability membership from string prefixes, names, Tool display names or package paths.

### 2.2 Capability cardinality

CA-SL defines Capability as an authorization grouping boundary, not as an Operation alias.

- One Capability MAY contain multiple Operation and Observation callable identities.
- One exact callable MAY be a member of multiple Capabilities when the deployment contract explicitly declares those memberships.
- A Capability MUST enumerate exact members. Wildcards, prefix expansion and implicit inheritance are forbidden.
- A Skill Grant authorizes only the explicitly declared membership, subject to Scope and Runtime admission.
- The Host SHOULD expose only the exact callable identities referenced by the admitted Skill, not every callable in every granted Capability.

Therefore, the Provider does not implement a Capability as a single indivisible function. It implements exact callable interfaces that may be members of one or more Capabilities.

## 3. Authority and ownership

| Component | Owns | Does not own |
|---|---|---|
| ca-skills | Skill Contract, Capability Grants, Procedure and exact callable references | Provider selection, URLs, HTTP method/path, credentials, deployment authorization |
| ToolCatalog Owner | Exact callable identity, kind, Input/Output or QueryContext schemas, Effect, Owner outcomes, Mutation/Reconcile and Evidence contract | Deployment grants, client transport implementation |
| Deployment CapabilityContracts | Capability IDs, exact callable membership, expected Effect and Scope policy references | Callable schemas, network paths, client implementation |
| ca-client-tools | SBI client implementation, pinned SBI Operation Binding Catalog, HTTP/2 transport, Payload Engine | Skill authorization, Capability membership, NF state authority, Runtime Outcome/Evidence/Reconcile |
| ca-agent-demo Host | Trusted artifact loading, Provider installation/configuration, active-Skill Tool exposure, trusted configuration and BinaryRef lifecycle | Re-defining Owner contracts or granting a Capability because a Provider reports support |
| CA Runtime | Callable admission, Capability and Scope authorization, Effect enforcement, Invocation identity, Outcome, Evidence and Reconcile | SBI path construction or arbitrary Provider discovery |
| Network Function / authoritative Owner | Actual network operation and authoritative state/result | Skill flow selection |

The ToolCatalog metadata.owner identifies the Tool contract Owner. It MUST NOT be changed to ca-client-tools merely because that repository supplies a client implementation. The Provider is an implementation source, not automatically the authoritative NF, resource or fact Owner.

For standardized operations, the pinned 3GPP OpenAPI source is the authority for the standardized wire definition. The ToolCatalog Owner remains responsible for the published CA callable contract and any normalized output/Evidence promises. The serving NF remains the authority for actual network state.

## 4. Three separate trusted inputs

The implementation MUST preserve three independent contract authorities and one separate deployment implementation binding. No one file or repository may silently replace the authority of another.

### 4.1 ToolCatalog — callable contract

The Owner-published ToolCatalog defines the exact callable:

- kind: operation | observation;
- stable exported id;
- Owner identity;
- Effect and Owner outcome contract;
- Operation Input/Output Schema, or Observation QueryContext/Output Schema;
- applicable Mutation receipt, failure class, Reconcile and Evidence guarantees.

This follows CA-SL v1alpha3 §§9 and Appendix F. The ToolCatalog is the source for the Agent-facing input/output schema and Effect contract.

A normal HTTP GET is not automatically an Observation Query. It is an Observation only when the Owner publishes it as kind: observation with the required query context, authority, Scope, temporal, correlation and Evidence guarantees. Otherwise it remains an Operation with Effect READ.

### 4.2 CapabilityContracts — deployment authorization

The deployment-owned CapabilityContracts maps each Capability ID to exact callable members:

- owner;
- id;
- kind;
- effect;
- trusted scopePolicyRef.

The Skill cannot create or modify this mapping. The Provider cannot grant a Capability. The Host and Runtime MUST validate the trusted deployment artifact and apply the same authorization semantics required by CA-SL.

The Provider MUST NOT return a Capability list and ask the Host to trust it as authorization.

### 4.3 SBI Operation Binding Catalog — transport execution and Provider support

ca-client-tools owns one separate, versioned SBI Operation Binding Catalog. Each entry maps an exact ToolCatalog callable identity to a pinned wire operation. **The set of valid entries in this catalog is the Provider's support declaration; there is no second Provider-support manifest that repeats the same callable list.**

For a standardized SBI operation, the binding references:

- the exact 3GPP release and API document revision;
- the pinned OpenAPI source revision and digest;
- the exact OpenAPI operationId;
- a trusted deployment service key;
- any CA-specific multipart/BinaryRef binding not already derivable from the OpenAPI definition.

The HTTP method, path template, parameters, media types, status codes and wire schemas MUST be derived from the pinned OpenAPI operation, not re-authored as an independent handwritten copy.

For a private OpenAMSM interface, the binding MUST identify a separately versioned private contract and MUST NOT label it as a standardized 3GPP operation.

The SBI Operation Binding Catalog MUST NOT redefine:

- Capability membership or grants;
- ToolCatalog Owner identity;
- Tool Input/Output schemas or Effect;
- deployment Scope policy;
- secret values or service origins.

Service origins, token profiles and resource limits come from trusted deployment configuration. The binding contains a service key, not an Agent-selectable URL.

## 5. Provider implementation selection

### 5.1 Provider support is the binding catalog

The installed Provider package ships the SBI Operation Binding Catalog as trusted, versioned package content. A callable is supported only when exactly one valid binding exists for its exact (owner, kind, id) identity and the pinned package digest is verified. A Provider cannot make an unknown callable valid by returning a self-asserted list.

The Provider catalog command described in §7 returns a filtered support projection from this same catalog. It does not introduce a second manifest or duplicate schemas, Effect, Capability membership or Scope.

### 5.2 Static implementation selection; Skill-scoped exposure

Two different forms of selection must remain distinct:

1. **Deployment implementation selection:** At deployment startup, trusted configuration selects an installed Provider implementation for each exact callable. The selected binding is validated and frozen. No runtime auto-discovery, environment-name guessing or silent fallback is introduced.
2. **Active-Skill Tool exposure:** When a Skill is admitted, the Host exposes only the exact callable subset used by that Skill, after resolving its Capability Grants, ToolCatalog dependencies, SBI Operation Binding Catalog and deployment policy. This is dynamic Tool visibility, not dynamic loading or selection of executable code.

If more than one Provider can implement the same callable, deployment configuration MUST select exactly one before the Resident becomes READY. Missing, duplicate, ambiguous or unsupported bindings fail closed. A running Invocation MUST NOT switch implementations. An uncertain Mutation and its Reconcile MUST remain bound to the original implementation identity.

The existing ca-agent-demo R5.8 manual registration is a Demo-specific implementation-selection mechanism. It is not a new CA-SL language construct or the normative SBI Operation Binding Catalog format. Its current operation keys and historical use of the word “Capability” MUST NOT be copied as the cross-repository identity model without reconciliation against CA-SL's separate CapabilityContracts and callable identities.

## 6. Host resolution and Tool registration

The Host performs the following deterministic resolution before exposing Agent tools:

1. Admit and freeze the Skill package and its dependency snapshots.
2. Verify the Skill's declared Capability Grants.
3. Load and verify the deployment CapabilityContracts artifact and its trusted deployment/publisher binding.
4. Resolve every exact callable referenced by the Skill's Procedure to one ToolCatalog export, including Owner, kind, Effect and schemas.
5. Prove that each callable is a member of at least one Capability granted by the Skill, and apply the intersection of applicable trusted Scope policies.
6. Resolve the exact callable against the installed Provider's SBI Operation Binding Catalog.
7. Verify deployment service configuration and source/deployment availability. A generic transport or a 3GPP OpenAPI definition alone does not prove that the selected OpenAMSM deployment implements the operation.
8. Build the Agent-facing Tool descriptor from the Owner-published ToolCatalog contract. The Provider supplies implementation support; it does not author or override the Tool schema.
9. Freeze the Tool set and implementation bindings for the active Skill/Invocation.
10. At every dispatch, Runtime rechecks callable admission, Capability Grant, Scope and Effect. Tool visibility is not authorization.

If a required callable is absent from ToolCatalog, not a member of a granted Capability, unsupported by the installed Provider, unavailable in deployment configuration or not source-verified for the selected NF deployment, the Skill MUST NOT be reported READY for that callable.

A Capability may contain more callables than a Skill uses. The Host MUST NOT expose unused members merely because the Skill granted the Capability.

## 7. Provider process boundary

The Go Provider remains a preinstalled executable, not an Agent-facing HTTP server, MCP gateway or arbitrary command runner.

The Host uses a fixed executable and fixed argument vector, never a shell. The conceptual process interface is:

~~~text
ca-client-tools provider catalog
ca-client-tools provider invoke
~~~

The catalog request contains exact callable identities already resolved by the Host, not Capability IDs:

~~~json
{
  "callables": [
    {
      "owner": "smf-owner",
      "kind": "operation",
      "id": "sbi.smf.example-operation"
    }
  ]
}
~~~

The response is a support projection only:

~~~json
{
  "data": {
    "supported": [
      {
        "owner": "smf-owner",
        "kind": "operation",
        "id": "sbi.smf.example-operation",
        "bindingRef": "sbi.smf.example-operation"
      }
    ],
    "unsupported": []
  }
}
~~~

The Host MUST verify every returned identity is an exact member of the request and matches the trusted ToolCatalog and the pinned Provider SBI Operation Binding Catalog. Duplicate, unknown or altered identities fail closed.

The Host builds the Agent-facing Tool descriptor from ToolCatalog plus trusted deployment authorization. The Provider response MUST NOT supply or override capability, effect, inputSchema, outputSchema, URL, HTTP method, path, headers or credentials.

The invoke request carries:

- exact callable identity (owner, kind, id);
- bounded JSON arguments validated against the ToolCatalog Input or QueryContext Schema;
- Host-generated invocation metadata required by the trusted binding;
- Host-staged relative binary file references, if the operation contract requires them.

It MUST NOT accept Agent-supplied URLs, methods, paths, arbitrary headers, token values, service keys, absolute filesystem paths or executable code.

The Provider validates the exact callable and its binding, constructs the request from trusted metadata, calls the shared SBI Transport, and returns a bounded structured result. The Host/Runtime independently validates output, Owner Outcome, Evidence and Mutation proof. Provider process success or HTTP 2xx alone MUST NOT be treated as Runtime SUCCEEDED or Mutation COMMITTED.

## 8. Direct SBI calls versus existing Owner workflows

The existence of an SBI operation name does not mean that a direct client call may replace the existing Owner workflow.

Before an operation is exposed through the generic SBI Provider, verify:

- its exact 3GPP or private API contract;
- the selected OpenAMSM source actually implements the endpoint;
- the Tool Owner and authoritative state Owner;
- identity, Scope, correlation and lifecycle bindings;
- mutation ownership, single-sender, idempotency and Reconcile requirements;
- the live acceptance plan.

In particular, do not replace existing ca-agent-demo CreatedSession implementations merely because their IDs begin with sbi. Existing UDM subscription, PCF policy creation and AMF N1/N2 transfer paths include Provider/Fabric ownership, hold/claim, journal, single-sender, callback readiness or exact-original Reconcile semantics. A direct HTTP call that bypasses those owners is not an equivalent implementation.

The existing sbi.udm.sm_data.get path also has a Provider-side implementation and provenance/result handling; do not silently redirect it to a direct NF call without an explicit owner and migration review.

Current ca-client-tools evidence must be kept distinct:

- the eight SMF MAS Context queries are private OpenAMSM interfaces, not standardized 3GPP SBI operations;
- AMF/SMF metrics endpoints are private HTTP projections, not SBI ToolCatalog operations by default;
- the shared h2c Transport and Payload Engine do not prove that any additional NF API is implemented;
- UDM/PCF/AUSF/NRF and other operations remain deferred until their pinned source and deployment support are verified.

A generic Provider is a reusable execution mechanism, not a claim of universal NF API coverage.

## 9. Effect, asynchronous responses and binary data

- Effect comes from ToolCatalog and MUST agree with the independently trusted CapabilityContracts declaration. Provider support metadata does not define Effect.
- READ operations still require Capability authorization, Scope and audit.
- Mutation requests are not automatically retried. A retry requires the exact Owner contract and Runtime policy to permit it.
- HTTP 202 and correlation headers are returned to Host/Runtime. The Provider does not create pending-operation state, Wait, Ingress listeners, continuation or a journal.
- A successful HTTP response does not itself prove business Postcondition or external Mutation commitment.
- Host owns BinaryRef storage, access control, expiry, cleanup and per-invocation staging. The Provider streams only Host-authorized staged files and writes response attachments into the Host-created output root.
- The Provider MUST NOT create a second persistent artifact store or expose base64 payloads to the model.
- Transport and Payload Engine preserve protocol and payload semantics; Runtime/Owner retain Outcome, Evidence and Reconcile authority.

## 10. Configuration and security

The first deployment profile uses static, trusted service configuration. NRF discovery, OAuth2, TLS/mTLS management and token lifecycle are deferred as previously decided.

The following values are never Skill/Agent-controlled:

- service origin and API root;
- HTTP method and path;
- trusted header/token profile and token value;
- redirect/proxy policy;
- transport timeout and resource limits;
- binary staging roots;
- Provider executable path and argv.

The Provider uses only configured service keys and trusted binding metadata. It does not accept absolute URLs or caller-selected network destinations. The Host terminates the process on cancellation and discards partial outputs; the Provider cancels the associated request.

## 11. Migration boundary with current repositories

### ca-skills

- Keep CA-SL v1alpha3 FROZEN.
- Do not add Provider, implementation ID, URL, HTTP method/path or deployment service configuration to Skill source.
- Keep contract.yaml Capability Grants separate from Procedure callable references.
- Do not rename current Skill callable references as part of this Provider design.

### ca-agent-demo

- Reuse existing Skill admission, ToolCatalog validation, CapabilityContracts, Runtime backend and READY coverage.
- Do not create a second Runtime, Capability registry, generic Tool registry, discovery service or MCP gateway.
- The current manual registration remains deployment-selected and frozen. A future SBI Provider adapter must bind exact callable identities and preserve existing owner-backed workflows.
- Do not migrate current B-class Provider/Fabric/journal implementations into the direct SBI client.

### ca-client-tools

- Keep the current CLI v1 contract unchanged.
- Keep AMF/SMF metrics clients separate from standardized SBI operations.
- Keep the typed MAS context-query client explicitly private.
- Add the SBI Operation Binding Catalog and Provider process contract as a separate workstream after the Transport/Payload PRs are merged.
- Do not implement Operation Catalog or Tool Provider inside the CLI command path.

## 12. Implementation sequence

| Step | Work | Exit condition |
|---|---|---|
| 0 | Review and freeze the identity/authority split in this document | CA-SL, ToolCatalog, CapabilityContracts and Provider roles have no ambiguous identity mapping |
| 1 | Define the versioned SBI Operation Binding Catalog schema | No duplicate Capability, Effect or Tool schema authority |
| 2 | Load and validate pinned OpenAPI plus CA execution bindings | Unknown operation, unpinned source, unsupported media type or service key fails closed |
| 3 | Implement Provider catalog/invoke subprocess contract | Fixed executable/argv, exact callable binding, strict JSON, cancellation and no arbitrary HTTP |
| 4 | Integrate a Host adapter for one source-verified read callable | Active-Skill registration, Runtime authorization and output validation pass |
| 5 | Add a mutation callable only after Owner proof/idempotency/Reconcile contract is available | No bypass of existing Owner, hold/claim, journal or single-sender semantics |
| 6 | Add multipart/binary callable after a real supported API is identified | Host BinaryRef lifecycle and real payload E2E pass |

The first real callable MUST be selected from the pinned OpenAMSM source inventory. Do not select a UDM/PCF API merely because the 3GPP OpenAPI repository defines it. If no suitable standardized SBI operation is currently source-verified, complete Steps 0–3 with fixtures and keep live operation onboarding NOT_AVAILABLE / NOT_RUN until an Owner publishes and verifies one.

## 13. Required acceptance gates

1. A Skill with a valid Capability Grant and exact callable reference resolves successfully.
2. Missing Grant, wrong Owner, wrong kind, wrong callable ID, duplicate membership, Effect mismatch or missing Scope fails closed.
3. A Provider cannot create a Capability, widen membership or alter Scope.
4. A Provider cannot execute a callable absent from its pinned SBI Operation Binding Catalog.
5. A Provider cannot alter ToolCatalog schemas, Effect or Owner outcomes.
6. A Skill cannot override service key, origin, method, path, headers, token profile or binary paths.
7. Only callables actually referenced by the admitted Skill are exposed to the Agent.
8. Runtime repeats Capability, Scope and Effect checks on every dispatch.
9. Two Capabilities containing the same callable apply the required trusted Scope intersection; no prefix or wildcard matching occurs.
10. A Provider switch during an Invocation is rejected; uncertain Mutation Reconcile remains pinned to the original Provider.
11. A 202 response does not create Provider-side Wait/Ingress state.
12. Binary input/output remains Host-owned and invocation-scoped.
13. Source support, client implementation, fixture tests and live NF acceptance are reported as separate evidence states.

## 14. Decisions intentionally left open

The following must be decided before implementation, not guessed in code:

- exact SBI Operation Binding Catalog apiVersion and canonical digest framing;
- exact SBI Operation Binding Catalog schema and how generated OpenAPI files are packaged;
- exact subprocess envelope fields for invocation identity, cancellation and error output;
- first source-verified standardized SBI callable to onboard;
- exact ToolCatalog Owner identity for each callable and the authorized relationship between that Owner and the Provider implementation.

These are contract-version decisions. They do not justify adding Capability fields to Skill source or changing frozen CA-SL semantics.

## 15. Non-goals

This design does not implement:

- arbitrary Agent-facing curl or raw HTTP;
- automatic Provider/plugin discovery or runtime implementation switching;
- OAuth2, TLS/mTLS or NRF discovery;
- a second Artifact Store;
- Host/Runtime Wait, Ingress, Journal, Evidence or Reconcile;
- direct PFCP/N4 transport;
- migration of existing Provider/Fabric Owner workflows;
- universal support for all 3GPP SBI APIs.
