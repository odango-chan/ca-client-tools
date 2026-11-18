# Shared SBI transport and Payload Engine

See [architecture](architecture.md) for the Provider target and [WORK](WORK.md) for status and ordering. This document owns shared transport and payload mechanics. The [Provider protocol](provider-protocol-v2.md) owns process framing, the [Catalog](provider-backend-binding-catalog-v2.md) owns bindings, and [Host integration](host-integration.md) owns the external dependency.

Status: **IMPLEMENTED** for the P0 slice and the P1 artifact slice. One
`internal/sbi` client instance, built from the single trusted service registry,
serves the typed MAS client, the genericSBI executor and the typed metrics
handlers. The metrics listeners keep their source-verified HTTP/1.1 behavior
through an explicit trusted transport profile, while lifecycle, cancellation,
timeout, redirect, proxy and size bounds are shared. Binary/multipart Provider
projection and Host artifact streaming are implemented against the pinned Host
Artifact Store contract; the Payload Engine also gained an additive optional
streaming-sink seam while its in-memory sink remains the default.

## Shared transport

Use `internal/sbi` for trusted service resolution, connection lifecycle, cancellation, size/time limits and low-level response handling. Each trusted service declares one reviewed `TransportProfile`:

- `sbi-h2c` (the default) — cleartext HTTP/2 prior knowledge without HTTP/1.1 fallback; an HTTP/1.1 negotiation is a transport error;
- `metrics-http1` — the OpenAMSM metrics listener profile: HTTP/1.1 only (cleartext or TLS), with an HTTP/2 response treated as a transport error.

Both profiles are built from the same trusted `Limits`: dial/request timeouts,
idle/connection caps, idle-connection lifetime, request/response size bounds,
disabled ambient proxies and no redirect following. The typed metrics client
keeps its paging validation, response-shape checks and error taxonomy; it no
longer owns an HTTP client. Preserve native Windows and Linux support. WSL
forwarding is acceptance tooling only.

Do not follow redirects, inherit ambient proxies, accept arbitrary URLs, or let caller arguments override trusted headers or credentials. Use [service configuration](service-configuration.md) for trusted service keys and deployment-injected secrets. TLS enrollment, OAuth, NRF discovery and direct PFCP transport are outside this target.

## Shared Payload Engine

Reuse `internal/sbi/payload` for bounded JSON, binary and multipart/related processing. The generic executor owns Catalog-driven validation and composition. Typed handlers may add source-specific checks but must reuse shared payload and transport plumbing where applicable.

- JSON preserves nested values, including arrays, null and number precision. Reject duplicate keys and enforce schemas and bounds.
- Binary and multipart stream through scoped readers and sinks. Multipart uses a JSON root, generated unique Content-IDs and catalog-declared JSON Pointers. Validate part set, media type, root references, count, size and digest; apply context cancellation throughout transfer.
- Existing local file resolver/sink helpers are reusable primitives subject to the future Host contract. The artifact stream channel and reference shape require that published contract before Provider integration. See [payload requirements](provider-payload-extension-v1.md).

## Async and failure boundary

Preserve declared upstream status, including 202/204, and distinguish remote semantic errors from invalid responses, transport errors and cancellation. Host/Runtime owns pending correlation and continuation. No provider polling, retries or workflow lifecycle. See [WORK](WORK.md) for acceptance gates.
