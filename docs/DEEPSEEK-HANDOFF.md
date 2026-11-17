# CLI v1 implementation and validation record

Status: **IMPLEMENTED — LOCAL ACCEPTANCE PASS; REAL CONTEXT E2E BLOCKED**
Repository: `odango-chan/ca-client-tools`
Branch / PR: `feat/openamsm-client-tools-foundation-20261116` / PR #1
Normative contract: [docs/cli-contract.md](cli-contract.md)
NF baseline: [docs/nf-capability-map.md](nf-capability-map.md)

This document records the delivered CLI v1 work and the evidence that remains
outstanding. It is not a request to redesign the CLI or to add a generic SBI
framework to this PR.

## Implemented command surface

```text
ca-client-tools amf ue list --page N --page-size N
ca-client-tools amf gnb list --page N --page-size N
ca-client-tools smf pdu list --page N --page-size N
ca-client-tools smf context query --type <session|policy|up-node|signaling-session|ip-address|up-selection|charging|session-auth>
```

The SMF context query reads one bounded JSON object from stdin. Public output
contains stable CA-facing `type`, `status`, `key` and `context`. OpenAMSM's
`masId` and private route identifiers are validated and consumed internally,
never emitted.

## What changed in this closeout

1. The SMF context transport now uses **cleartext HTTP/2 with prior knowledge
   (h2c)**. The real SBI listener rejects HTTP/1.1; the previous client could
   not have worked against a real SMF.
2. Response identity binding is enforced in `bindEchoKey`:
   - echoed SUPI must equal the requested SUPI (or `imsi-<IMSI>` when the
     request used the IMSI alias);
   - echoed `pduSessionId` must equal the requested PSI;
   - every requested UE IP must be echoed and equal as an address;
   - a missing, extra or different field is a Protocol Error, including for
     `UNAVAILABLE` responses.
3. CLI fixes: a real newline is written after the success envelope; the
   oversized-input test constant was corrected; invalid `--type` is rejected
   before configuration lookup.
4. Tests cover SUPI + PSI, IMSI + PSI, IPv4, IPv6, joint lookup, all eight
   request routes, cross-UE and cross-session responses, AVAILABLE /
   UNAVAILABLE, HTTP errors, timeout, redirect, oversize response, malformed
   JSON, duplicate keys and sensitive-value redaction.
5. All SMF context tests now use an h2c test server and assert HTTP/2 is
   actually used.

## Local validation

| Command | Result |
|---|---|
| `gofmt -w .` | PASS |
| `go test ./...` (Go 1.26.8) | PASS |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| Compiled-CLI fixture acceptance (29 cases) | PASS |
| GitHub Actions | CI_UNAVAILABLE |

The compiled CLI was exercised against a fixture that reproduces the pinned
OpenAMSM handler's echo and status semantics. This is local contract evidence,
not live-NF evidence.

## Real OpenAMSM validation

A real environment was built from the pinned source snapshot (verified tree
hash) and run with 11 NFs and a real UERANSIM UE. An active PDU session existed
for `imsi-999700000000001`, PSI `1`, IPv4 `10.45.0.2`.

| Surface | Result |
|---|---|
| AMF UE List | PASS |
| AMF gNB List | PASS |
| SMF PDU List | PASS |
| Eight SMF Context queries | BLOCKED |

All eight context types returned HTTP 403 `FORBIDDEN` through both the
compiled CLI and a direct `curl --http2-prior-knowledge` call. The token was
confirmed loaded in the running SMF via `gdb`. Root cause: the handler looks up
`X-OpenAMSM-Query-Token` in a case-sensitive header hash, but HTTP/2 requires
lowercase field names. A literal mixed-case field name is rejected by nghttp2
with `GOAWAY PROTOCOL_ERROR`. No RFC-compliant client can satisfy the check.

No server patch was applied. Full evidence:
[real acceptance record](evidence/real-openamsm-acceptance-20261117.md).

## Remaining work

1. Correct the token header lookup in a deployed OpenAMSM revision (owner:
   `env-openamsm`). Do not patch the server inside this repository.
2. Rerun the eight context types with a known SUPI + PSI and record real
   `AVAILABLE` / `UNAVAILABLE`, echoed identities, stdout/stderr and exit
   codes.
3. Prove a live UE-A request / UE-B response rejection.
4. Obtain a usable CI run, or keep `CI_UNAVAILABLE` recorded.

PR #1 does not meet the stated final delivery criterion while the live context
queries are blocked. It should remain open until the upstream lookup is fixed
and the eight contexts are accepted, unless the reviewer explicitly narrows
the delivery to the list surfaces plus client-side contract evidence.

## Source authority

OpenAMSM snapshot:
[`77c0a2f8511c3498d1270f059881b3cf0f7f5eeb`](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb),
archive SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`,
tree SHA-256
`7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d`.

See [docs/openamsm-interface-inventory.md](openamsm-interface-inventory.md)
for exact request/response semantics and the blocker analysis.

## Out of scope for this PR

- UPF `/session-info` and any OpenAMSM server patch.
- TLS, mTLS, OAuth2, token refresh and NRF discovery.
- Mutations and write operations.
- Generic SBI transport, Operation Catalog, Tool Provider and Payload Engine.
- NW Agent workflow/facade/orchestration/relay endpoints.

The forward-looking SBI design remains in
[docs/sbi-client-architecture.md](sbi-client-architecture.md) as a separate,
unimplemented workstream. Later branches in this repository contain prototypes
for that workstream; they are not part of PR #1.
