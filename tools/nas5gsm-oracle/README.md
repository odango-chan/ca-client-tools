# OpenAMSM C oracle for the scoped 5GSM Modify codec

This runner compares the exact binary fixtures used by `protocol/nas5gsm` with
the native C decoder and QoS-flow parser from the fixed OpenAMSM 2.8.0 source
snapshot imported by `odango-chan/env-openamsm`.

It does **not** modify `upstream/openamsm/`. If the native build is missing, the
runner invokes the existing `env-openamsm build` command, which builds from its
disposable staged copy.

## Fixed source identity

The source is an imported ZIP archive, not a verified upstream Git checkout:

| Identity | Value |
| --- | --- |
| OpenAMSM version | `2.8.0` |
| Archive SHA-256 | `6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f` |
| Imported source-tree SHA-256 | `7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d` |
| ZIP comment commit | `c27e1761ae09957a474cc2b0208d227cde64aabf` — archive metadata only, **not** asserted to be a Git commit |

The runner verifies the full source-tree hash and these Git blob identities
before compiling:

| Source file | Git blob SHA |
| --- | --- |
| `lib/nas/5gs/decoder.c` | `fced527f7abc33365fbd3b913b1bce0cbdb80627` |
| `lib/nas/5gs/ies.c` | `0eede97b829f854d80ac55d65005ff0632a1c8ad` |
| `lib/nas/5gs/types.c` | `4eb990c5e7e2b7217c7eae546ee2a8e33b8c03e8` |
| `lib/nas/5gs/types.h` | `e7a29acb5cd9872e6280edda136e74c3e614910e` |
| `lib/nas/5gs/message.h` | `0dd6a721ea9e80d11286eac1727cd688f3e81634` |
| `lib/nas/common/types.c` | `288d5f004653ec66cfbd3df8e7145cb5b9a08cb7` |
| `lib/core/ogs-pkbuf.h` | `74971b5fb29086d00c1a70bb79da2182ca3cfe25` |
| `lib/core/ogs-core.h` | `f72cc364b29dc069df5c470c7088a56555e6dc7d` |

## What is compared

For each fixture, the C harness calls the actual OpenAMSM entry points:

1. `ogs_nas_5gsm_decode()` in `lib/nas/5gs/decoder.c`
2. `ogs_nas_parse_qos_flow_descriptions()` in `lib/nas/5gs/types.c`

It emits a normalized projection of the decoded 5GSM header and QoS-flow
parameters. The expected output is checked in beside the exact input bytes.

| Fixture | NAS message | Oracle fields |
| --- | --- | --- |
| `modify-request.hex` | UE PDU Session Modification Request (`0xC9`) | PSI, PTI, QFI, operation, E bit, GFBR/MFBR units and values |
| `modify-command.hex` | SMF PDU Session Modification Command (`0xCB`) | PSI, PTI, QFI, operation, E bit, 5QI, GFBR/MFBR units and values |
| `modify-complete.hex` | UE PDU Session Modification Complete (`0xCC`) | PSI, PTI and message type |

The request and command fixtures are the same raw bytes as the Go golden
vectors. The C side does not call the Go encoder or decoder.

## Run

On a machine with the `env-openamsm` checkout and its normal native build
environment:

```bash
ENV_OPENAMSM_ROOT=/path/to/env-openamsm \
  bash tools/nas5gsm-oracle/run.sh
```

The runner verifies the fixed source identity, uses or creates the normal
`.runtime/install` build, compiles the small harness against the installed
`libogsnas-5gs`, `libogsnas-common` and `libogscore` libraries, then compares
all three outputs.

To choose an evidence directory:

```bash
ENV_OPENAMSM_ROOT=/path/to/env-openamsm \
OPENAMSM_ORACLE_OUT=/path/to/evidence \
  bash tools/nas5gsm-oracle/run.sh
```

A successful run writes `manifest.json` with source identity, compiler/platform,
fixture hashes and actual C-decoder output, and prints
`NAS_5GSM_MODIFY_OWNER_ORACLE_PASS`.

## Gate status

The harness and immutable fixtures are checked in, but **adding a runner is not
the same as running it**. Until a successful run manifest is attached to
ca-client-tools Issue #25 / PR #26, the C-oracle gate remains `NOT_RUN` and PR
#26 must not be merged on the strength of Go CI alone.

This is a protocol-codec oracle only. It does not claim the separate
`OPENAMSM_A_UE_MODIFY_E2E_PASS` or `OPENAMSM_C_UE_MODIFY_E2E_PASS` gates.
