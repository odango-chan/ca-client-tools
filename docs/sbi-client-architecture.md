# Shared SBI transport and Payload Engine

See [architecture](architecture.md) for the Provider target and [WORK](WORK.md) for status and ordering. This document owns shared transport and payload mechanics. The [Provider protocol](provider-protocol-v2.md) owns process framing, the [Catalog](provider-backend-binding-catalog-v2.md) owns bindings, and [Host integration](host-integration.md) owns the external dependency.

Status: **IMPLEMENTED** for the P0 slice. One `internal/sbi` client instance,
built from the single trusted service registry, serves the typed MAS client and
the genericSBI executor; typed metrics keep their source-verified HTTP/1.1
client. Binary/multipart Provider projection remains P1.

## Shared transport

Use `internal/sbi` for trusted service resolution, connection lifecycle, cancellation, size/time limits and low-level response handling. The default SBI profile for OpenAMSM is cleartext HTTP/2 prior knowledge without HTTP/1.1 fallback. Preserve the typed metrics HTTP/1.1 listener behavior with an explicit trusted profile, sharing lifecycle and bounds without replacing typed decoding. Preserve native Windows and Linux support. WSL forwarding is acceptance tooling only.

Do not follow redirects, inherit ambient proxies, accept arbitrary URLs, or let caller arguments override trusted headers or credentials. Use [service configuration](service-configuration.md) for trusted service keys and deployment-injected secrets. TLS enrollment, OAuth, NRF discovery and direct PFCP transport are outside this target.

## Shared Payload Engine

Reuse `internal/sbi/payload` for bounded JSON, binary and multipart/related processing. The generic executor owns Catalog-driven validation and composition. Typed handlers may add source-specific checks but must reuse shared payload and transport plumbing where applicable.

- JSON preserves nested values, including arrays, null and number precision. Reject duplicate keys and enforce schemas and bounds.
- Binary and multipart stream through scoped readers and sinks. Multipart uses a JSON root, generated unique Content-IDs and catalog-declared JSON Pointers. Validate part set, media type, root references, count, size and digest; apply context cancellation throughout transfer.
- Existing local file resolver/sink helpers are reusable primitives subject to the future Host contract. The artifact stream channel and reference shape require that published contract before Provider integration. See [payload requirements](provider-payload-extension-v1.md).

## Async and failure boundary

Preserve declared upstream status, including 202/204, and distinguish remote semantic errors from invalid responses, transport errors and cancellation. Host/Runtime owns pending correlation and continuation. No provider polling, retries or workflow lifecycle. See [WORK](WORK.md) for acceptance gates.
