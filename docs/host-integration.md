# Host integration

The demo Host launches the fixed `ca-client-tools provider serve` executable directly, verifies the package and embedded Catalog digest against trusted deployment metadata, initializes one resident process, and selects available bindings for the active Skill under deployment policy. Correlate by request ID, bound pending requests, discard late results after cancellation, and treat process failure as incomplete work. Restart without replay.

| Owner | Responsibility |
|---|---|
| `ca-client-tools` | Single protocol and Catalog, private invocation validation, typed handlers, generic execution, shared transport and Payload Engine |
| `ca-agent` Host | Process integration, configuration and secrets injection, tool activation, private projection/redaction, Artifact Store and stream adapter |
| `ca-runtime` | Callable admission, Capability/Scope authorization, execution identity, Procedure state, Outcomes, Evidence and Reconcile |
| `ca-skills` | Business procedures and declared callable dependencies |
| `env-openamsm` | Source, deployment and NF lifecycle |

Backend IDs are private. Enforce admission and bind trusted UE/session identity before dispatch. Validate echoed identity and project sensitive metrics/subscriber data before any Agent-visible output.

## Required cross-repository dependency for P1

The Host Artifact Store owns storage, authorization, reference issuance and resolution, isolation, expiration, quota, cleanup and publication. The Provider consumes invocation-scoped input streams and streams validated attachments to a Host-provided sink. Only Host-issued opaque references appear at declared locations in JSON. The Host owns publication and disposal of partial transfers.

Before P1 integration, get the Host owner to publish and pin the accepted contract covering:

1. Reference JSON shape, scope and authentication, binding to invocation and direction, and the reference-to-stream handoff.
2. Cross-process transfer mechanism, adapter bootstrap/configuration, stream and sink interfaces, size/digest metadata and quotas.
3. Result attachment mapping and reference publication/finalization, including cancellation, timeout, partial writes, expiry and crash cleanup.
4. Stable error mapping and shared conformance fixtures for both repositories.

No wire field names, staging roots, environment variables, reference grammar or Host API are approved here. Existing local file adapters and executor fixtures are implementation evidence only. Record the accepted contract URL and commit in WORK before enabling P1. Continue P0 while this dependency is pending. See [payload requirements](provider-payload-extension-v1.md).
