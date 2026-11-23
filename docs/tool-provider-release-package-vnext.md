# CA Client Tools Provider Release Package — vNext design

**Status:** TARGET DESIGN ONLY (2026-11-23). This format is **not** shipped by the current [ca-client-tools v1.0.0 release](https://github.com/odango-chan/ca-client-tools/releases/tag/v1.0.0).

**Consumers:** [ca-skill-compiler v2.1.0](https://github.com/odango-chan/ca-skill-compiler/releases/tag/v2.1.0) at Skill build time; `ca-agent-demo` v0.5.0 Generic Tool Registry at Host startup and execution. **Implementation tracker:** [ca-agent-demo#241](https://github.com/odango-chan/ca-agent-demo/issues/241).

## 1. Architecture decision

**Publish once, consume twice.** The Go Provider owns exactly **one canonical Tool definition source**. Release tooling deterministically emits:

1. **The Compiler interface** in its **existing, frozen** CA-SL Interface Package format: `connectionagent.io/interface-package/v1alpha1`, with `ToolCatalog` (the exact `connectionagent.io/toolcatalog/v1alpha4` payload), `CapabilityCatalog`, standalone JSON Schemas, and any required EvidenceDescriptor/other linked artifacts.
2. **The Host runtime binding**: a small, generated `provider.json` containing the launch protocol/argv, the immutable Interface Package reference/digest, and exact public Callable ID → private `backendOperationId` mappings. It **does not duplicate** the Tool Schema, Effect, Capability definitions or a second ToolCatalog.
3. **The Go executable** containing the implementation and the embedded private backend catalog. Its `initialize` identity/digest and `catalog` response must agree with the packaged runtime binding.

The two generated **views** are not two separately authored catalogs. A single generation/validation pipeline from one source is mandatory. The Go source can use its existing embedded `provider-bindings/v2` Catalog as the starting point, but it currently lacks complete public Tool contracts and must be extended. Hand-maintained copies in Python Host or a parallel release Schema directory are forbidden.

**OpenAPI is not a mandatory Tool interface format.** Existing 3GPP/OpenAMSM OpenAPI (prefer 3.1 for new protocol material) may be a *source* for HTTP request/response schemas and private Provider execution details. The generic compiler-visible interface is `ToolCatalog + JSON Schema` in the already released compiler dialect. A non-HTTP Tool must not be forced to invent HTTP paths or fake OpenAPI operations.

## 2. On-disk release contents

Per-platform archive (e.g. future Linux amd64; version/name are examples, not a release claim):

```text
ca-client-tools_<version>_linux_amd64/
  ca-client-tools
  provider.json
  interface/
    package.json
    tool-catalog.yaml
    capabilities.json
    schemas/
      <public-input>.schema.json
      <public-output>.schema.json
    evidence-descriptors.yaml    # include only if the interface exports one
  README.md
  CHANGELOG.md

GitHub Release:
  ca-client-tools_<version>_linux_amd64.tar.gz
  ca-client-tools_<version>_linux_arm64.tar.gz
  ca-client-tools_<version>_windows_amd64.zip
  SHA256SUMS
```

All compiler-visible files, including any optional EvidenceDescriptor and transitive Schema dependencies, must be included in an immutable Interface Package snapshot and exported with the expected manifest file digests. **Do not require Compiler to execute the Go binary, contact a Provider process or fetch schemas over the network.**

For cross-package Schema dependencies, provide the exact external Interface Package snapshots and Dependency Lock to Compiler as separate inputs; do not copy dependent schemas into this package as unrecorded duplicates.

### Compiler interface (`interface/package.json`)

Use the **already implemented** `connectionagent.io/interface-package/v1alpha1`. Its manifest has `apiVersion`, `packageId`, `version`, `languageTarget=connectionagent.io/v1alpha5`, `publisher`, `dependencies[]`, `exports[]`, `files[]`. Export kinds are `CallableCatalog`, `Schema`, `CapabilityCatalog` and, when needed, `EvidenceDescriptor` / `IngressCatalog`. Actual file hashes and export IDs are generated and verified; the following is only an abbreviated structural sketch:

```json
{
  "apiVersion": "connectionagent.io/interface-package/v1alpha1",
  "packageId": "<stable-provider-interface-package-id>",
  "version": "<package-version>",
  "languageTarget": "connectionagent.io/v1alpha5",
  "publisher": "<published-identity>",
  "dependencies": [],
  "exports": [
    {"kind": "CallableCatalog", "id": "example.session.read", "path": "tool-catalog.yaml"},
    {"kind": "CapabilityCatalog", "id": "example.session-read", "path": "capabilities.json"},
    {"kind": "Schema", "id": "urn:example:session-read-input:v1", "path": "schemas/session-read-input.schema.json"},
    {"kind": "Schema", "id": "urn:example:session-read-output:v1", "path": "schemas/session-read-output.schema.json"}
  ],
  "files": ["<exact sha256 manifest entries for all non-package.json files>"]
}
```

The `files` placeholder above is not valid conformance data. Real generation must emit the exact `{path,sha256}` records, and export every member of any multi-operation ToolCatalog by precise ID. Publisher identity and exported owner must meet existing Compiler static linking rules.

### Tool definitions (`interface/tool-catalog.yaml`)

Reuse the frozen `connectionagent.io/toolcatalog/v1alpha4` DTO; do not introduce a custom parallel `tools[]` contract for the Compiler. An illustrative READ operation:

```yaml
apiVersion: connectionagent.io/toolcatalog/v1alpha4
kind: ToolCatalog
metadata:
  name: example-provider-tools
  owner: example-provider-owner
spec:
  contracts:
    - kind: operation
      id: example.session.read
      effect: READ
      ownerOutcomes: [SUCCEEDED, FAILED]
      inputSchema:
        $ref: urn:example:session-read-input:v1
      outputSchema:
        $ref: urn:example:session-read-output:v1
```

The `capabilities.json` file exports exact Capability-to-Callable membership. Its contents are separately listed in the **same** Interface Package manifest, but all are generated from the **same Go canonical Tool source**, not manually authored as another authority. Each referenced `$ref` must resolve within the compiler's declared direct-dependency and snapshot/lock rules.

The standalone schemas are JSON Schema using the **Compiler-supported dialect/subset**. OpenAPI `components.schemas` from 3GPP source documents may be converted into these standalone schemas, but the converter must preserve required properties, type/format constraints, references and identity, or fail rather than silently narrow/weaken. A valid OpenAPI 3.1 document alone does not establish valid CA-SL interface package or full dialect compatibility.

For MUTATION, the compiler's frozen ToolCatalog requires more than Effect: Outcome, receipt Schema, mutation/reconcile and any Evidence Descriptor fields must be present and statically valid. The release builder must not emit a fake success receipt or silently omit those contracts just to make a mutation compile. Current production Provider v1.0.0 still gates Mutation.

### Host runtime binding (`provider.json`)

**Proposed small runtime binding format, not an implemented schema.** Contains **no duplicate** input/output schemas:

```json
{
  "apiVersion": "ca.tool-provider-runtime-binding/v1",
  "providerId": "ca-client-tools.sbi",
  "protocol": "ca-client-tools.provider/v2",
  "transport": {
    "type": "stdio",
    "argv": ["./ca-client-tools", "provider", "serve"]
  },
  "interfacePackagePath": "interface",
  "interfaceSnapshotDigest": "<sha256 of exact interface package snapshot>",
  "bindings": [
    {
      "publicCallableId": "example.session.read",
      "backendOperationId": "openamsm.smf.mas.sc.query"
    }
  ]
}
```

Public and private IDs in the runtime example are illustrative; actual bindings must be generated with exact reviewed IDs and valid callable identities, not inferred by string concatenation or lowercasing.

`transport.argv` is **process startup**, not one CLI command per Tool invocation. No shell expansion or arbitrary path escape. Connection uses long-lived NDJSON over stdin/stdout. Host owns the selected local package path and process configuration; sensitive credentials and private Host invocation references never appear in the public Schema or ToolCatalog.

At startup Host verifies the manifest, interface snapshot digest, Provider handshake identity and embedded catalog digest, and calls existing Provider v2 `catalog` with the explicit private IDs already enumerated in `bindings[]`. **No new “list all” wire method is necessary** for discovery when the release package lists exact public/private IDs. Unsupported or not-configured Tool bindings stay unavailable; a Skill requiring them is NOT_READY.

## 3. Compiler and Host consume the same publication

```text
Go canonical Tool definitions + actual handlers
             |
        release generator
             |
     +-------+-----------------+
     |                         |
Interface Package           Runtime Binding + Binary
(tool-catalog.yaml,         (provider.json + Go executable)
 capability catalog,
 JSON Schemas, evidence)
     |                         |
Compiler + Interface Lock   CA Agent Host deployment
 (static Skill linking)      (Tool registration, READY)
     |                         |
     +----------+--------------+
                |
          Runtime execution
                |
       Go Provider v2 invoke
```

- **Compiler:** accepts CA-SL source, exact Interface Package snapshots and Dependency Lock (Development v1alpha1) or frozen v1alpha5 resolved dependencies. It does not load the executable, evaluate runtime availability or claim Production Qualification. `build-development` output is `NOT_ADMITTED` until separate Host admission.
- **Host:** reads only an operator-approved local extracted Provider package, uses its Interface Package's **same** ToolCatalog and Schema files for registration/runtime validation, and uses the runtime-binding sidecar to start and dispatch to the Provider. The Host does not re-author ToolCatalog/Capability data.
- **Skill selection and authorization:** a registered Tool does not grant an arbitrary Skill access. Exact compiled Skill requirements and the Host's existing Root-scoped authorization still gate the call. Schema mismatch, unavailable binding and unsupported Effect fail clearly and are repaired in the next Provider release.
- **Version pinning:** the Compiler artifact records the Interface Package identity/digest; the Host must load a matching installed package or refuse admission. No version solver, automatic download, Plugin manager, governance platform or duplicate catalog needed.

## 4. Acceptance gates before calling format implemented

1. Go generator emits `interface/package.json`, ToolCatalog, CapabilityCatalog, standalone schemas and runtime binding deterministically **from the same source**. Hashes and cross-file IDs are consistent; no human-maintained duplicate artifacts.
2. Existing `ca-skill-compiler v2.1.0` **actually** links a synthetic Skill against the generated Interface Package and its exact Dependency Lock; verify expected callable/Schema/Capability export resolution and a negative broken `$ref` case. Do not call an OpenAPI parse smoke test equivalent.
3. The release archive includes the Go binary, both generated interface/runtime views, valid checksums; provider `initialize/catalog` agrees with `provider.json`.
4. The built Host wheel registers all selected available Tools **using the published package alone** and validates calls/outputs without Python MAS maps or manual `ToolImplementationCatalog` objects.
5. New Tool added to the Go catalog and released + Skill recompiled → unchanged Host wheel loads it; missing Schema, duplicate ID, Effect mismatch or ungranted operation fail, not silently repaired.
6. Existing READ regression passes; Scheme C Mutation Create/Modify/Release requires an independently verified new Go Provider release with true Owner outcomes, private Run scopes and admission. Do not turn the current v1.0.0 READ-only binary into an implied mutation-capable release.
7. Cleanly remove old Host-authored Tool catalogs/manual registration/MAS-specific Python adapters **at final Tool cutover** (ca-agent-demo#241); no lasting compatibility path.

## 5. Non-goals

Not an OpenAPI-only interface, general-purpose schema registry, duplicate ToolCatalog, new Compiler API, runtime-capability trust authority, Plugin system, MCP transport, automatic installation platform or production-qualification proof.

**Release boundary:** merge this design independently of implementation. No tag, binary publish, SDK behavior or compiler compatibility claim until tests with the exact generated assets pass.
