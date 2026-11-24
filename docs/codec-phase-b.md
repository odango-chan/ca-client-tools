# CA unified codec Phase B — Free5GC facade and worker

Status: implementation candidate / Phase B  
Tracking: `ca-client-tools` Phase B (new PR, OPEN)  
Base: `main` at `22cff6b` (`Merge pull request #36`). PR #33 (Phase A
architecture/coverage audit) and PR #38 (ValidateEPCO) remain separate open
PRs; neither is merged, copied or required by this branch.
Pinned modules:

- `github.com/free5gc/nas v1.3.1`
- `github.com/free5gc/ngap v1.2.0`

The Go checksums are fixed in `go.sum`:

```
github.com/free5gc/nas v1.3.1 h1:UMPymaxkr0KT+a3FTXC8gg6jcUbi6BvHjHOjuEL9NSI=
github.com/free5gc/ngap v1.2.0 h1:tZPokNiF/J97dmq0DVATcE1lZi7stCo7LZaosSB8otQ=
```

The module source is not copied, forked or modified. No APER fail-closed patch is
applied in this phase. The facade observes the unpatched extension behaviour and
fails closed for byte-level qualification.

## 1. Why this shape

Phase A established that the Create / Modify / Release traffic is entirely on the
Free5GC `ext=0` path and re-encodes byte-identically. It also established two
real limits of the unpatched modules:

1. `free5gc/ngap v1.2.0` APER discards extensible-SEQUENCE extension additions
   instead of consuming or rejecting them. This is silent data loss.
2. `free5gc/nas v1.3.1` can parse an ePCO IE successfully and re-encode it
   lossily; duplicate IEs are silently skipped; `IEToDoList` is the only typed
   partial-decode signal.

Phase B therefore does not add a second protocol implementation. It adds:

- a facade that calls the upstream Go APIs and returns a typed view plus explicit
  status and diagnostics;
- a byte-identity gate (re-encode and compare) used only as evidence, never as a
  replacement for the caller's bytes;
- a canonical JSON projection with a generated type registry;
- a framed long-lived worker so Python hosts use the same Go facade.

## 2. Packages and API

```
protocol/codec
  nas.go        DecodeNAS / EncodeNAS
  ngap.go       DecodeNGAPPDU / EncodeNGAPPDU
  n2.go         DecodeN2Transfer / EncodeN2Transfer, explicit root list
  json.go       ProjectToJSON / ProjectFromJSON
  capability.go capabilities and JSON-encode gate
  zz_*_generated.go  generated upstream type/IE/root metadata
  internal/typegen   generator for the metadata
  internal/capgen    generator for docs/codec-capabilities.json
  worker/       framed stdin/stdout IPC

internal/cli/codec.go   hidden `ca-client-tools codec worker`
python/ca_codec_worker  Python process client
python/tests            Python IPC acceptance
```

Go consumers use the upstream types directly. There is no CA-owned NAS message
model, no per-message hand-written Go encoder and no ASN.1 interpreter.

```go
result := codec.DecodeNAS(n1Bytes)          // raw bytes are copied and retained
if result.Status == codec.StatusSuccess {
    typed := result.Decoded.(message.GSMMessage)
    // result.ReencodeEligible is true only after byte-identical re-encode.
}

result = codec.DecodeNGAPPDU(ngapBytes)
result = codec.DecodeN2Transfer("PDUSessionResourceSetupRequestTransfer", n2Bytes)
```

`DecodeN2Transfer` requires the root. It never guesses the root from bytes. The
41 upstream transfer roots are listed by `codec.N2Roots()`.

`EncodeNAS`, `EncodeNGAPPDU` and `EncodeN2Transfer` always produce a new slice.
They never write into or return the caller's input buffer.

## 3. Status and diagnostics

| Status | Meaning |
| --- | --- |
| `SUCCESS` | The declared scope decoded; for NAS/NGAP/N2 the exact byte round-trip also matched. `qualified=true`, `reencodeEligible=true`. |
| `PARTIAL` | NAS only: `ParseGSM` returned `*message.Error` with a non-empty `IEToDoList`. The typed value exists but is incomplete; it is not qualified and is not re-encode eligible. |
| `UNSUPPORTED_MESSAGE` | The upstream module has no decoder/encoder for the 5GSM type, NGAP procedure/message or N2 root. |
| `UNSUPPORTED_IE` | A well-formed NAS IEI or N2 protocol IE ID is outside the generated upstream set. |
| `DUPLICATE_IE` | A duplicate NAS IEI or NGAP/N2 IE, rejected because upstream may silently skip it. |
| `INVALID` | Truncated/malformed APER or NAS, missing mandatory IE, or trailing bits/octets that the strict consumer check rejects. |
| `NOT_QUALIFIED` | A typed projection exists, but the facade cannot prove lossless/complete handling. The raw bytes remain authoritative and no re-encoded output is offered. |

`PARTIAL` is detected only with `errors.As(err, *message.Error)` and
`len(IEToDoList) > 0`. No NAS error string matching is used.

Machine-readable diagnostics carry `code`, `message`, optional `iei`, `ieId` and
`offset`. Important codes include:

- `RAW_BYTES_PRESERVED`
- `NAS_IE_TODO`, `NAS_UNKNOWN_IEI`, `NAS_DUPLICATE_IEI`, `NAS_EPCO_REENCODE_LOSSY`
- `NGAP_APER_EXTENSION_UNQUALIFIED`, `NGAP_TRAILING_BITS`,
  `NGAP_ROUND_TRIP_MISMATCH`

## 4. NAS behaviour

`DecodeNAS` accepts only 5GSM (`EPD=0x2e`) with a valid PSI/PTI. It validates the
4-octet header before calling `message.ParseGSM`.

The upstream `ParseGSM` dispatch is used for all 20 5GSM message types it
implements. The facade does not whitelist Create / Modify / Release.

A generated structural IEI table (derived from the pinned module's own field and
IEI declarations) is walked before/after `ParseGSM` to detect:

- unknown IEIs (upstream aborts with an untyped error, so the facade reports the
  IEI and offset);
- duplicate IEIs (upstream silently skips the second occurrence);
- truncated TLV/TLV-E framing.

`PARTIAL` uses the upstream typed error and exposes `ieToDo`. It is never treated
as a complete message.

### ePCO

A successful ePCO decode is **decode-projection-only**:

- `structuralParse=true`
- `projectionValid=true` for the fields the upstream type models
- raw bytes are preserved
- `reencodeEligible=false`
- `reencodeVerified=false`
- diagnostic `NAS_EPCO_REENCODE_LOSSY`

`EncodeNAS` refuses to emit bytes when `ExtendedProtCfgOpts` is non-nil, because
the upstream encoder drops unmodelled options. A decode `SUCCESS` is not
automatically a re-encode licence.

## 5. NGAP and N2 behaviour

### Full NGAP PDU

`DecodeNGAPPDU` calls `message.Parse`. The 120 message types in the pinned
`ngap/message` package are exposed by the capability manifest. A typed value is
re-encoded only to compare bytes with the input:

- equal: `SUCCESS` / qualified / re-encode eligible;
- different: `NOT_QUALIFIED`, no re-encode eligibility, extension diagnostic.

This is deliberately not a shallow root-extension-bit check. The known APER
defect applies to nested SEQUENCEs too; the byte round-trip is used as the
conservative gate. It does not prove arbitrary R19 conformance.

### Standalone N2 transfer

The root is explicit. The 41 `*Transfer` types in `ngap/ie` are enumerated. The
facade adds:

1. strict bit consumption (`ByteOffset()*8 + BitOffset()`), allowing only
   sub-octet zero padding;
2. a structural scan of ProtocolIE-container roots using the generated upstream
   IE-ID set, so unknown and duplicate N2 IEs are classified before parsing;
3. a root extension-bit check for roots the generator marks extensible: bit 1 is
   `NOT_QUALIFIED`, not a successful decode;
4. byte-identical re-encode as the final qualification gate; a mismatch means
   `NOT_QUALIFIED` and the raw bytes remain authoritative.

The root extension check does **not** claim to solve nested extension additions.
The re-encode gate is the additional evidence, and the limitation is reported in
every capability response.

## 6. Canonical JSON projection

`ProjectToJSON` and `ProjectFromJSON` operate on the generated registry of 2,439
Free5GC exported structs. The projection is a value mapping, not a protocol
model. Conventions:

| ASN.1 / Go shape | JSON |
| --- | --- |
| CHOICE / interface | `{"_choice":"ngap.ie.CauseNas","value":{...}}` |
| SEQUENCE / struct | object keyed by exported Go field name |
| SEQUENCE OF / slice | array; `null` distinguishes nil from `[]` |
| INTEGER / ENUMERATED | JSON number (Python `int` preserves exact values) |
| BIT STRING | `{"hex":"...","bitLength":32}` |
| OCTET STRING / `[]byte` | lowercase hex string; `null` for nil |
| optional / absent | `null` or omitted; both map to Go zero/nil |
| extension additions | not representable by the unpatched upstream types; decode fails qualification |

Unknown JSON fields and unknown `_choice` names are rejected. The mapper does not
silently drop unmapped fields.

JSON Encode is gated. Only types with a dedicated projection round-trip test are
advertised in the capability manifest. Other types are decode-projection-only and
the worker returns `NOT_QUALIFIED` / `JSON_ENCODE_NOT_QUALIFIED` before encoding.

## 7. Worker IPC

Command:

```
ca-client-tools codec worker
```

The command is hidden from CLI v1 help. It is a long-lived process over
stdin/stdout, opens no network port, adds no Tool Provider, needs no database,
configuration centre or plugin manager. Diagnostics go to stderr only.

### Frame

All integers are big-endian.

```
+----------------+  4 octets: body length N (1 + 4 + J + B)
| body length    |
+----------------+  1 octet: frame version = 1
| version        |
+----------------+  4 octets: JSON length J
| json length    |
+----------------+  J octets: UTF-8 JSON request/response
| JSON           |
+----------------+  B octets: raw binary segment (decode input / encode output)
| binary         |
+----------------+
```

The frame length is bounded (64 MiB by default; JSON at most 4 MiB). Binary is
raw bytes; it is never Base64 encoded. Requests are serial; one response is
written per request.

### Request

```json
{
  "requestId": "host-42-17",
  "operation": "decode",
  "kind": "n2",
  "root": "PDUSessionResourceSetupRequestTransfer",
  "options": {}
}
```

`kind` is `nas`, `ngap` or `n2`. `encode` adds `type` (Free5GC qualified type
name) and `message` (canonical JSON). `operation` may also be `capabilities` or
`ping`.

### Response

```json
{
  "requestId": "host-42-17",
  "operation": "decode",
  "status": "SUCCESS",
  "protocol": "N2_SM",
  "typeName": "ngap.ie.PDUSessionResourceSetupRequestTransfer",
  "root": "PDUSessionResourceSetupRequestTransfer",
  "rawLength": 45,
  "rawPreserved": true,
  "complete": true,
  "qualified": true,
  "reencodeEligible": true,
  "reencodeVerified": true,
  "message": { "...": "canonical projection" },
  "diagnostics": []
}
```

The raw input is not echoed in the response; the caller already owns it and the
facade guarantees it was not modified. An encode response carries the output in
the binary segment and `length` in JSON.

## 8. Python lifecycle

`python/ca_codec_worker/client.py` provides `CodecWorker`:

- starts one worker process on `start()` / first request and reuses it;
- serial request/response with request IDs;
- a reader thread so `timeout` is enforced without blocking forever;
- timeout and explicit `cancel()` terminate the worker; the next request starts a
  fresh process (the protocol is serial, so a late frame cannot be safely
  matched);
- abnormal exit becomes `CodecWorkerExited`;
- `close()` closes stdin, waits, then terminates/kills the process;
- `with CodecWorker(...)` performs cleanup automatically.

No Base64 or shell interpolation is used.

## 9. Coverage and tests

The authoritative machine-readable manifest is
[`codec-capabilities.json`](codec-capabilities.json). It records, per message or
root: upstream implemented, CA tested, CA qualified, JSON encode supported, N2
protocol IE IDs and known limitations. It is generated by
`go run ./protocol/codec/internal/capgen`; a Go test fails if the checked-in file
is stale.

Current totals: 20 NAS 5GSM messages (12 tested/qualified, 6 JSON-encode
tested), 120 NGAP PDU messages (1 self-round-trip tested, 0 globally qualified,
1 JSON-encode tested), and 41 N2 transfer roots (7 tested/qualified for the
real-vector subset, 7 JSON-encode tested). The manifest lists 14 distinct
recognized-but-unimplemented NAS IE types across 8 messages, including
`IPHdrCompressionCfg`, `EthHdrCompressionCfg`, `ATSSSCntr`, `SvcLvlAACntr`,
`ReqMBSCntr`, `ReceivedMBSCntr`, `ServingPLMNRateCtrl`, `PDUSessPairID`, `RSN`,
`SMPDUDNReqCntr`, `ReattemptIndicator` and `CtrlPlaneOnlyInd`. A message carrying
one of them is `PARTIAL`.

Evidence used:

- real OpenAMSM NAS encoder vectors for 0xC2, 0xCB, 0xD3 and 0xC3;
- CA/real-business NAS vectors for 0xC1, 0xC9, 0xD1 and 0xD4;
- real OpenAMSM/asn1c N2 transfer vectors for Setup Request, Modify Request,
  Setup Response (IPv4, IPv6, multi-QFI), Setup Unsuccessful, Modify Response,
  Release Command and Release Response;
- Phase A ext=1 and trailing-octet vectors, all rejected/not qualified;
- ePCO lossy vector, duplicate IEI vectors, unknown IEI and truncation vectors;
- Go worker versus native facade JSON comparison;
- Python worker versus the built binary for NAS and N2, including raw binary
  round-trip and `NOT_QUALIFIED` gating.

The self-encoded JSON round-trip tests prove facade consistency, not 3GPP
conformance. The real OpenAMSM and asn1c vectors remain the interoperability
evidence.

Commands:

```sh
go test ./... -count=1
go vet ./...
gofmt -l .
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go build -o /tmp/ca-client-tools ./cmd/ca-client-tools
CA_CLIENT_TOOLS_BIN=/tmp/ca-client-tools python3 -m unittest discover -s python/tests -v
```

## 10. Current limits

1. APER extension additions are not decoded or preserved. Root `ext=1` is
   `NOT_QUALIFIED`; nested additions are not independently proven. A future
   fail-closed upstream patch remains a prerequisite for general R19 trust.
2. NGAP full-PDU coverage is source availability, not a conformance suite. No
   full-PDU message is globally `CA Qualified`; `NGReset` is tested for the
   self-round-trip path only.
3. JSON Encode is intentionally narrower than Decode. Only tested types are
   admitted; the rest are decode-projection-only.
4. NAS v1 covers N1 SM 5GSM only. There is no 5GMM, NAS security, ciphering,
   integrity or key management.
5. Unknown N2 IEs cannot be retained/re-emitted. They fail closed.
6. Trailing NAS octets are treated as an unknown IEI by upstream; the facade
   reports `UNSUPPORTED_IE` rather than silently ignoring them.

## 11. Phase C gate

Phase C may start only after:

- this Phase B PR is reviewed and its Windows/Linux CI is green;
- the regression vectors in section 9 remain byte-identical;
- the consumer migration keeps raw N1/N2 bytes and Content-ID metadata
  authoritative;
- no consumer treats `SUCCESS` as a licence to overwrite the original wire
  bytes, especially for ePCO or extension-bearing NGAP.
