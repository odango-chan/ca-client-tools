# Host integration

The demo Host launches the fixed `ca-client-tools provider serve` executable directly, verifies the package and embedded Catalog digest against trusted deployment metadata, initializes one resident process, and selects available bindings for the active Skill under deployment policy. Correlate by request ID, bound pending requests, discard late results after cancellation, and treat process failure as incomplete work. Restart without replay.

| Owner | Responsibility |
|---|---|
| `ca-client-tools` | Single protocol and Catalog, private invocation validation, typed handlers, generic execution, artifact channel client, shared transport and Payload Engine |
| `ca-agent` Host | Process integration, configuration and secrets injection, tool activation, private projection/redaction, Artifact Store and stream adapter |
| `ca-runtime` | Callable admission, Capability/Scope authorization, execution identity, Procedure state, Outcomes, Evidence and Reconcile |
| `ca-skills` | Business procedures and declared callable dependencies |
| `env-openamsm` | Source, deployment and NF lifecycle |

Backend IDs are private. Enforce admission and bind trusted UE/session identity before dispatch. Validate echoed identity and project sensitive metrics/subscriber data before any Agent-visible output.

## Identity separation

The following identities are distinct and MUST NOT be substituted for one
another:

| Identity | Authority | Meaning |
|---|---|---|
| Capability ID | Deployment CapabilityContracts | Authorization grant named by the Skill |
| Public callable identity | Host-owned ToolCatalog | Exact `(owner, kind, id)` exported to the Skill surface; owner is the Host |
| Qualified callable reference | Skill Procedure | Dependency alias plus the exact Host export ID |
| Provider ID | Provider package/deployment | Installed backend executable, e.g. `ca-client-tools.sbi` |
| Provider backend operation ID | Provider Catalog | Private fixed typed/generic operation |
| Host adapter binding | Trusted Host package/deployment | Maps one admitted public callable to one exact backend operation ID |
| Invocation/producer ID | Host/Runtime | Execution and Evidence correlation identity |

A catalog entry, CLI command or public callable never grants permission and a
backend ID never becomes an Agent-visible name. The generic
`smf.session.context.query` callable and the MAS projection callables (for
example `smf.mas.sc.context.query`) are Host-owned names; the Provider's
`openamsm.*` identifiers remain private. Any future public naming, Capability
mapping or Skill dependency remains a Host/owner contract and is not
introduced by this repository.

## P1 Host Artifact Store dependency: published and pinned

The Host Artifact Store contract is now **published and pinned**. P1 is
**enabled in this repository's code** against exactly that contract.

| Item | Pinned value |
|---|---|
| Contract identity | `ca-host.artifact-store/v1` |
| Owning repository | `odango-chan/ca-agent` (Host integration / Artifact Store) |
| Contract commit | `2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae` (documentation-only update of accepted `3a0d140401109d19152e253cb0b89b59feef7bfb`; contract content and fixture bytes unchanged) |
| Contract document | <https://github.com/odango-chan/ca-agent/blob/2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae/docs/design/host-artifact-store-v1.md> |
| Fixture manifest SHA-256 | `59238a4363c598a56e9d091eb9b5d96a3c492583435021fb9a7b2170ea66ddb4` |
| Pinned fixture copy | [`contracts/host-artifact-store/v1/`](../contracts/host-artifact-store/v1) (byte-identical copy of `conformance/artifact-store/v1/*`; `contracts/host-artifact-store/v1/MANIFEST.sha256` is verified by `internal/artifactchannel`) |
| JSON schemas | `ca-agent/schemas/artifact-store/v1/` (reference, channel config, channel error, artifactSlots) |

The contract fixes the reference JSON shape, invocation/slot/direction binding,
loopback channel wire protocol, bootstrap configuration, size/digest/quota
metadata, cancellation/timeout/expiry/cleanup semantics, result publication and
stable error mapping. The Provider side is implemented by
`internal/artifactchannel` (channel client), the `providerapp` artifact bridge
and the executor's optional streaming sink; `internal/provider` validates the
`artifactChannel` bootstrap and publishes the closed `artifactSlots` descriptor
extension. See [the payload contract](provider-payload-extension-v1.md) and
[Provider protocol §2.1/§4](provider-protocol-v2.md).

Cross-repository end-to-end acceptance against the real `ca-agent` Host adapter
is run by the Team Lead and recorded by the Lead; this repository records only
its own local unit/process-level gates in
[the P1 evidence](evidence/provider-p1-20261118.md). A real-NF multipart
operation is not onboarded by this contract and remains `NOT_VERIFIED`.

No wire field names, staging roots, environment variables, reference grammar or
Host API beyond the pinned contract is approved here. Existing local file
adapters and executor fixtures remain implementation or test scaffolding only.
