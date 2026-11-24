# Upstream PR materials: fail closed on APER extension additions

**Status:** PREPARED - NOT SUBMITTED. No Free5GC repository has been modified or
contacted. This bundle is the complete submission package; filing it needs an
explicit go-ahead.

**Target:** `github.com/free5gc/ngap`, base tag `v1.2.0`.

**Patch:** [`failclosed-v2.patch`](failclosed-v2.patch) (SHA-256
`805d16c16168a6f02498b7ee165c9659425a629e0241b7aa6a475b5a4cb8fb96`).
The copy stored as [`../free5gc-ngap-aper-ext-failclosed-20261124.patch`](../free5gc-ngap-aper-ext-failclosed-20261124.patch)
is identical.

## 1. Problem

`aper.ReadSequencePreambleBitMap` reads the extension bit of an extensible
SEQUENCE and then discards it (`// TODO: Support extensible sequence`). When a
peer sets that bit, the decoder neither consumes the extension-addition list nor
reports it: the additions are silently dropped, a truncated addition list is
accepted, and a nested addition shifts the parent parse. A codec that claims
byte preservation cannot start from such a decode.

## 2. Change

Three small, additive changes; no ASN.1 model, no wire change for ext=0:

1. `aper/unmarshal.go`: add `ErrExtensibleSequenceUnsupported` and return it,
   wrapped, when the extension bit is 1. Encoding is untouched - it still writes
   a zero extension bit, which is correct while the codec has no additions to
   write.
2. `ie/protocol_err.go`, `nrppa/ie/protocol_err.go`: add `Unwrap()` to
   `TransferSyntaxErr`. Without it the generated `Read` functions wrap the
   sentinel into a type that only keeps the message text, so `errors.Is` cannot
   classify the failure. This is required for the fail-closed behavior to be
   usable, and it is backwards compatible.
3. Tests: `aper/extension_bit_test.go` (helper level) and
   `ie/extension_failclosed_test.go` (root, nested, ext=0 regression, and the
   `errors.Is` chain).

The `ReadSequencePreambleBitMap` signature is deliberately unchanged, so no
generated call site changes and the diff cannot drift with code regeneration.

## 3. Exact correspondence to v1.2.0

Verified on a clean module-cache copy of `github.com/free5gc/ngap@v1.2.0`:

| Item | Value |
| --- | --- |
| `aper/unmarshal.go` blob before the patch | `51218552dde932ac3e4c4636a88654f6481469ad` |
| `index` line in the patch | `5121855..53f4dc8` |
| `git apply --check` | clean |
| `git apply` | clean, 5 files |
| files changed | `aper/unmarshal.go`, `aper/extension_bit_test.go` (new), `ie/protocol_err.go`, `nrppa/ie/protocol_err.go`, `ie/extension_failclosed_test.go` (new) |

`free5gc/ngap` carries its own `aper/` package. The standalone `free5gc/aper`
module does not contain `ReadSequencePreambleBitMap` at the pinned version, so
this is a single-repository change; no second PR is needed.

Log: [`logs/apply-and-pin.log`](logs/apply-and-pin.log).

## 4. Test results

`go test ./...` on the patched clean copy (Go 1.26.8, linux/amd64):

```
ok  github.com/free5gc/ngap
ok  github.com/free5gc/ngap/aper
ok  github.com/free5gc/ngap/ie
ok  github.com/free5gc/ngap/message
?   github.com/free5gc/ngap/nrppa/ie  [no test files]
ok  github.com/free5gc/ngap/nrppa/message
ok  github.com/free5gc/ngap/util
```

Logs: [`logs/patched-go-test.log`](logs/patched-go-test.log),
[`logs/patched-focused-tests.log`](logs/patched-focused-tests.log).

## 5. Standards vectors

ext=0 and ext=1 vectors, decoded on both the unpatched and the patched build:
[`logs/vectors-unpatched.log`](logs/vectors-unpatched.log),
[`logs/vectors-patched.log`](logs/vectors-patched.log).

| Vector | Unpatched v1.2.0 | Patched |
| --- | --- | --- |
| Setup Request ext=0 (45 octets) | decoded, re-encode byte-identical | decoded, re-encode byte-identical |
| Setup Request ext=1, empty addition list (`80...00`) | accepted, addition bit and byte dropped | rejected, `errors.Is(err, aper.ErrExtensibleSequenceUnsupported)` |
| Setup Request ext=1, one addition (`80...0202aabb`) | accepted, addition bytes dropped | rejected, sentinel |
| Setup Request ext=1, truncated additions | accepted | rejected, sentinel |
| Setup Request ext=0 + two trailing octets | accepted, trailing bytes dropped | accepted (strict consumption stays a caller duty) |
| Modify Request ext=0 | decoded, byte-identical | unchanged |
| Modify Request ext=1, empty additions | accepted, bit dropped | rejected, sentinel |
| Release Command ext=0 (`10`) | decoded, byte-identical | unchanged |
| Release Command ext=1 (`9000`) | accepted, bit dropped | rejected, sentinel |
| Nested `QosFlowSetupRequestItem` ext=0 (`004000091d00`) | decoded, byte-identical | unchanged |
| Nested item ext=1, empty additions (`804000091d0000`) | accepted, bit dropped | rejected, sentinel |
| Nested item ext=1, one addition (`804000091d008002aabb`) | accepted, addition dropped | rejected, sentinel |

The ext=1 vectors are legal X.691 encodings, not synthetic garbage: pycrate
0.8.1 decodes every one of them to the same root values as its ext=0 form
([`logs/pycrate-ext-vectors.log`](logs/pycrate-ext-vectors.log)). The
unpatched build re-encodes each ext=1 vector as its ext=0 form, which is the
silent data loss the patch converts into a hard error.

## 6. Compatibility and risk

- ext=0 traffic: byte-for-byte unchanged; all ten audited OpenAMSM/asn1c vectors
  still decode and re-encode identically.
- ext=1 traffic: previously "decoded" with data loss, now returns an error. Any
  consumer that depended on the lossy behavior was already producing wrong
  values, so this is a correctness fix rather than a behavior regression.
- New exported identifiers only (`ErrExtensibleSequenceUnsupported`,
  `TransferSyntaxErr.Unwrap`); no signature changes, so generated code and
  downstream builds are unaffected.
- Residual gap, stated in the PR: this is fail-closed, not lossless. A peer that
  legitimately uses extension additions still cannot be decoded; that needs the
  larger X.691 implementation described in the APER report §5.1.

## 7. Maintain a narrow fork, or submit upstream?

| | Submit upstream | Maintain a narrow fork |
| --- | --- | --- |
| Cost now | one PR, one review cycle | fork branch + pin in `go.mod` + replace directive |
| Cost per Free5GC release | rebase only if `aper`/`protocol_err` changes | rebase/verify every release plus provenance and checksum bookkeeping |
| Consumer impact | `go.mod` can point at the upstream tag | every consumer inherits the fork and its audit trail |
| Risk | review may ask for the full X.691 implementation; the fail-closed subset may be accepted as a first step | fork drifts, or is forgotten and silently pins stale code |
| Recommendation | **first choice** | fallback only if upstream rejects a fail-closed first step |

Because the change is three additive edits with no signature changes, the
maintenance delta is small either way. The deciding factor is trust: using the
upstream tag keeps `go.mod` auditable without a private fork. The fail-closed
subset is also a strict improvement over the current silent drop, which makes it
a reasonable first upstream step even if the full addition support follows
later.

## 8. Submission checklist

- [x] clean-copy apply and full `go test ./...`
- [x] standards vectors on both builds, with pycrate confirming the ext=1
      vectors are legal
- [x] change description, compatibility and risk notes (this file)
- [x] maintenance-cost assessment
- [ ] explicit authorization to open the upstream PR
- [ ] upstream discussion of the full extension-addition support (out of scope
      for the fail-closed patch)
