# OpenAMSM C oracle for the scoped 5GSM Modify codec

This runner compares the exact binary fixtures used by `protocol/nas5gsm` with
the native C decoder and QoS-flow parser from the fixed OpenAMSM 2.8.0 source
snapshot imported by `odango-chan/env-openamsm`.

It does **not** modify `upstream/openamsm/`. The runner always invokes the
existing `env-openamsm build` command so the installed libraries are guaranteed
to correspond to the pinned source-plus-patch identity and current toolchain.
The build driver reuses the staged source when its identity is unchanged.

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
| `lib/nas/5gs/ogs-nas-5gs.h` | `afac3b90942800fd1cc117e3b4b8da7622d5f729` |
| `lib/nas/common/ogs-nas-common.h` | `6b85aa4d6afecad578ccc1107f1152724bbc7f35` |
| `lib/nas/common/types.h` | `1bb31383ebfacddb607965f93551949cedf1643d` |
| `lib/nas/common/types.c` | `288d5f004653ec66cfbd3df8e7145cb5b9a08cb7` |
| `lib/core/ogs-pkbuf.h` | `74971b5fb29086d00c1a70bb79da2182ca3cfe25` |
| `lib/core/ogs-core.h` | `f72cc364b29dc069df5c470c7088a56555e6dc7d` |
| `lib/core/ogs-log.h` | `547f1962011c9dec5f48a3d3d97c509c4638eee7` |

## Native build patch identity

The native libraries are built by `env-openamsm` from a disposable staged
copy at `build/openamsm-source`, not directly from the imported source
directory. The build driver applies the 19 files under
`patches/openamsm/` before compiling.

The runner therefore also:

- pins all 19 patch files by their Git blob SHA-1;
- recomputes the same source-plus-patch identity used by
  `scripts/build.sh`;
- invokes `bin/env-openamsm build` before using the libraries and checks the
  build identity marker;
- checks that the staged `decoder.c`, `ies.c`, `types.c`, `types.h`,
  `message.h`, common bitrate implementation and required core headers are
  byte-identical to the pinned imported source.

The inspected patch set changes SMF orchestration/handlers and related tools;
the staged-source checks ensure it has not changed the NAS decoder/parser
being used as the C oracle. The runner also pins the repository-owned build
entrypoint and helper scripts (`bin/env-openamsm`, `scripts/build.sh`,
`scripts/hash_upstream_tree.py`, `scripts/lib/paths.sh`,
`scripts/lib/host.sh`, `scripts/verify-upstream.sh`, and
`scripts/check-host.sh`) by Git blob SHA-1. The generated manifest records
the env-openamsm repository HEAD, build-driver blob identities, complete patch
list and blob identities, source-plus-patch identity, staged NAS source
identities, shared-library hashes and harness hash. Local absolute checkout
paths are deliberately omitted from the manifest.

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
