# Codec Phase B acceptance evidence (2026-11-24)

Status: LOCAL ACCEPTANCE / branch `feat/free5gc-codec-phase-b-20261124`

## 1. Windows amd64 (native)

- Windows 11 (10.0.26200.0), PowerShell 5.1
- Go 1.26.6 windows/amd64
- Python 3.14.6
- `CGO_ENABLED=0`
- `go test ./... -count=1`: PASS (all packages)
- `go vet ./...`: PASS
- `go build -o dist/ca-client-tools.exe ./cmd/ca-client-tools`: PASS
- Python `unittest discover -s python/tests -v`: 7 tests PASS
- `WINDOWS_ACCEPTANCE_OK` marker written by the local script.

Codec-specific results:

- NAS real OpenAMSM vectors 0xC2/0xCB/0xD3/0xC3 and CA vectors
  0xC1/0xC9/0xD1/0xD4 decode and re-encode byte-identically.
- ePCO vector decodes but is decode-projection-only and cannot be re-encoded by
  `EncodeNAS`.
- NAS PARTIAL is reached through `*message.Error.IEToDoList`.
- Ten N2 OpenAMSM/asn1c vectors plus the IPv6 two-QFI vector round-trip
  byte-identically.
- N2 `ext=1` root vectors are `NOT_QUALIFIED`; trailing octets are `INVALID`.
- Go worker/native facade JSON comparison passes.
- Python worker NAS/N2 encode results equal the input bytes; timeout and
  abnormal-exit handling pass.

## 2. Linux amd64

- WSL2 Ubuntu 26.04 LTS, kernel `6.18.58.1-microsoft-standard-WSL2`, x86_64
- Go 1.26.8 linux/amd64
- Python 3.14.4
- The tests run linux/amd64 binaries under the Linux kernel, not a Windows
  cross-compile. WSL is an acceptance environment only; the product has no WSL
  dependency.
- `CGO_ENABLED=0 go test ./... -count=1`: PASS
- `CGO_ENABLED=0 go vet ./...`: PASS
- `CGO_ENABLED=0 go build -o /tmp/ca-client-tools ./cmd/ca-client-tools`: PASS
- Python worker acceptance: 7 tests PASS
- The repository CI also runs native `ubuntu-latest` for Go tests, vet, build and
  the same Python worker acceptance.

## 3. macOS

No native macOS execution environment was available in this session. No macOS
claim is made. The code has no platform-specific build tags or CGO dependency;
the CI workflow does not currently run macOS.

## 4. What is not proven by these runs

- No claim of 3GPP Release 19 conformance for all 120 NGAP messages or all NAS
  IEs. Availability comes from the pinned modules; qualification is limited to
  the vectors and round-trip gates listed in
  [`docs/codec-capabilities.json`](../codec-capabilities.json).
- No live SCTP/N2 capture was taken in this phase. The OpenAMSM/asn1c vectors are
  independent of Free5GC and remain the interoperability evidence.
- Nested APER extension additions are not independently proven. The unpatched
  APER limitation remains.
