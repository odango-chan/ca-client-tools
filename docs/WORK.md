# Work

## Overall

Design: ACCEPTED for the OpenAMSM read-only CLI v1 baseline.

Implementation: **COMPLETE for the CLI v1 read-only scope** on
`feat/openamsm-client-tools-foundation-20261116` (PR #1).

Acceptance (2026-11-17):

- Go 1.26.8 `gofmt`, `go test ./...`, `go vet ./...`, `go build ./...`:
  **PASS**.
- Compiled-CLI contract acceptance against a protocol-faithful fixture:
  **PASS**.
- Real OpenAMSM deployment (pinned snapshot, native build, 11 NFs, UERANSIM
  UE with an active PDU session):
  - AMF UE List, AMF gNB List, SMF PDU List: **PASS**.
  - Eight SMF Context queries: **BLOCKED** by an OpenAMSM token-header
    defect (mixed-case lookup over HTTP/2), not by client code. No server
    patch was applied.
- GitHub Actions: **CI_UNAVAILABLE**.

Evidence: [docs/evidence/real-openamsm-acceptance-20261117.md](evidence/real-openamsm-acceptance-20261117.md).

## Current scope matrix

| Scope | Design | Implementation | Acceptance |
|---|---|---|---|
| OpenAMSM ownership and read-only scope | ACCEPTED | DOCUMENTED | Review |
| SMF context query (eight types) | ACCEPTED | IMPLEMENTED | Local contract PASS; live BLOCKED |
| Response identity binding (SUPI/PSI/IP) | ACCEPTED | IMPLEMENTED | Unit + compiled-CLI PASS; live BLOCKED |
| AMF `/ue-info`, `/gnb-info` metrics | ACCEPTED | IMPLEMENTED | Live PASS |
| SMF `/pdu-info` metrics | ACCEPTED | IMPLEMENTED | Live PASS |
| CLI v1 process contract | ACCEPTED | IMPLEMENTED | Local PASS + live list PASS |
| Generic SBI transport / catalog / payload engine | Separate workstream | NOT_STARTED in this PR | NOT_RUN |
| UPF `/session-info` | **OUT OF SCOPE** | NOT_IMPLEMENTED | NOT_RUN |

## Live environment facts

The real SMF SBI listener is cleartext HTTP/2 with prior knowledge (h2c). The
client must use h2c; HTTP/1.1 is rejected. The metrics listeners are plain
HTTP/1.1.

The real token check is currently unusable by any RFC-compliant HTTP/2 client:
the handler looks up `X-OpenAMSM-Query-Token` in a case-sensitive header hash,
while HTTP/2 requires lowercase field names. The token was confirmed loaded in
the running SMF. This is an OpenAMSM defect owned by `env-openamsm`.

## Existing blockers

- OpenAMSM context-query token header lookup (see inventory §3.4). Blocks all
  eight live context queries and therefore the final delivery criterion.
- GitHub Actions logs are unavailable; local acceptance is recorded separately
  as `LOCAL_ACCEPTANCE_PASS` and CI as `CI_UNAVAILABLE`.

## Next actions

1. Fix the token header lookup in a deployed OpenAMSM revision (case-insensitive
   lookup or lowercase field name). Do not patch inside this repository.
2. Rerun the eight context queries with a known SUPI + PSI and record
   `AVAILABLE` / `UNAVAILABLE`, echoed-key binding and CLI exit codes.
3. Prove UE-A request / UE-B response rejection against the live server.
4. Keep the generic SBI transport, Operation Catalog and Payload Engine in
   their separate workstream; do not merge them into PR #1.

## Authority / evidence links

- [Real acceptance evidence](evidence/real-openamsm-acceptance-20261117.md)
- [OpenAMSM source findings at pinned snapshot](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/docs/openamsm-source-findings.md)
- [OpenAMSM SMF context-query OpenAPI v0.10](https://github.com/odango-chan/env-openamsm/blob/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml)
- docs/architecture.md
- docs/cli-contract.md
- docs/openamsm-interface-inventory.md
- docs/nf-capability-map.md
