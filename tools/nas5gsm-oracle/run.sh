#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd -P)"
ENV_OPENAMSM_ROOT="${ENV_OPENAMSM_ROOT:-${1:-}}"

if [[ -z "$ENV_OPENAMSM_ROOT" ]]; then
  cat >&2 <<'EOF'
Usage:
  ENV_OPENAMSM_ROOT=/path/to/env-openamsm tools/nas5gsm-oracle/run.sh

The runner uses the immutable upstream/openamsm snapshot and the existing
native build/install managed by env-openamsm. It does not modify upstream source.
EOF
  exit 2
fi

ENV_OPENAMSM_ROOT="$(cd -- "$ENV_OPENAMSM_ROOT" && pwd -P)"
SOURCE_ROOT="$ENV_OPENAMSM_ROOT/upstream/openamsm"
BUILD_ROOT="$ENV_OPENAMSM_ROOT/build/openamsm"
BUILD_SOURCE_ROOT="$ENV_OPENAMSM_ROOT/build/openamsm-source"
PATCH_DIR="$ENV_OPENAMSM_ROOT/patches/openamsm"
INSTALL_ROOT="$ENV_OPENAMSM_ROOT/.runtime/install"
EXPECTED_VERSION="2.8.0"
EXPECTED_TREE_SHA256="7dfe5a3db4d4bc920bfaa7b844b80336698f90f6c7086b060b93afa4c8556c8d"
EXPECTED_ARCHIVE_SHA256="6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f"

if [[ ! -d "$SOURCE_ROOT" || ! -f "$ENV_OPENAMSM_ROOT/upstream.lock.yaml" ]]; then
  echo "[FAIL] ENV_OPENAMSM_ROOT does not contain the imported OpenAMSM snapshot" >&2
  exit 2
fi

grep -Fq "version: \"$EXPECTED_VERSION\"" "$ENV_OPENAMSM_ROOT/upstream.lock.yaml"
grep -Fq "archive_sha256: $EXPECTED_ARCHIVE_SHA256" "$ENV_OPENAMSM_ROOT/upstream.lock.yaml"
grep -Fq "tree_sha256: $EXPECTED_TREE_SHA256" "$ENV_OPENAMSM_ROOT/upstream.lock.yaml"
grep -Fq "version : '$EXPECTED_VERSION'" "$SOURCE_ROOT/meson.build"

TREE_OUTPUT="$(python3 "$ENV_OPENAMSM_ROOT/scripts/hash_upstream_tree.py" "$SOURCE_ROOT")"
ACTUAL_TREE_SHA256="$(printf '%s\n' "$TREE_OUTPUT" | awk '/^tree_sha256:/ {print $2; exit}')"
if [[ "$ACTUAL_TREE_SHA256" != "$EXPECTED_TREE_SHA256" ]]; then
  echo "[FAIL] OpenAMSM source tree drift: $ACTUAL_TREE_SHA256" >&2
  exit 2
fi

check_blob() {
  local relative_path="$1"
  local expected_sha="$2"
  local actual_sha
  actual_sha="$(git -C "$ENV_OPENAMSM_ROOT" hash-object "$SOURCE_ROOT/$relative_path")"
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    echo "[FAIL] OpenAMSM source blob drift: $relative_path ($actual_sha)" >&2
    exit 2
  fi
}

# Git blob identities for the exact C decoder/parser and the headers consumed
# by this harness. The archive-comment commit is not treated as a Git commit.
check_blob "lib/nas/5gs/decoder.c" "fced527f7abc33365fbd3b913b1bce0cbdb80627"
check_blob "lib/nas/5gs/ies.c" "0eede97b829f854d80ac55d65005ff0632a1c8ad"
check_blob "lib/nas/5gs/types.c" "4eb990c5e7e2b7217c7eae546ee2a8e33b8c03e8"
check_blob "lib/nas/5gs/types.h" "e7a29acb5cd9872e6280edda136e74c3e614910e"
check_blob "lib/nas/5gs/message.h" "0dd6a721ea9e80d11286eac1727cd688f3e81634"
check_blob "lib/nas/5gs/ogs-nas-5gs.h" "afac3b90942800fd1cc117e3b4b8da7622d5f729"
check_blob "lib/nas/common/ogs-nas-common.h" "6b85aa4d6afecad578ccc1107f1152724bbc7f35"
check_blob "lib/nas/common/types.h" "1bb31383ebfacddb607965f93551949cedf1643d"
check_blob "lib/nas/common/types.c" "288d5f004653ec66cfbd3df8e7145cb5b9a08cb7"
check_blob "lib/core/ogs-pkbuf.h" "74971b5fb29086d00c1a70bb79da2182ca3cfe25"
check_blob "lib/core/ogs-core.h" "f72cc364b29dc069df5c470c7088a56555e6dc7d"
check_blob "lib/core/ogs-log.h" "547f1962011c9dec5f48a3d3d97c509c4638eee7"

check_patch_blob() {
  local relative_path="$1"
  local expected_sha="$2"
  local actual_sha
  actual_sha="$(git -C "$ENV_OPENAMSM_ROOT" hash-object "$PATCH_DIR/$relative_path")"
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    echo "[FAIL] OpenAMSM build patch drift: $relative_path ($actual_sha)" >&2
    exit 2
  fi
}

# env-openamsm builds a disposable staged tree with this exact patch set.
# Pin every patch blob, not just upstream.lock.yaml's imported source archive.
check_patch_blob "0001-mas-query-token-lowercase.patch" "a00f11f53392fd663b8d3c91c3accee94c40b6ed"
check_patch_blob "0006-owner-qos-delta.patch" "87a695181760e6d0e7f54257f04fd1bc6d5e2e30"
check_patch_blob "0007-ssm-n1-modify-parse.patch" "3fd59a90a9e567e764dd96499f7aa654ed9de097"
check_patch_blob "0008-nw-agent-scheme-c-modify.patch" "f4f67fac3c2c68ec67cda666479abe636ce6e481"
check_patch_blob "0009-non-gbr-qos-delta.patch" "7b1bd51d0ef4275e3a5f182462bed239eae0fe16"
check_patch_blob "0010-relay-fail-closed.patch" "91de6e63e016634b54207d1556fc0e111b7f4bc9"
check_patch_blob "0011-ue-release-pure-mas.patch" "037bb46c55747b6d2ed6aa297d1563b111fb6e59"
check_patch_blob "0012-nw-agent-scheme-c-release.patch" "481b1dc20f7fd4e0d584dbab965c95d51a666add"
check_patch_blob "0013-ue-release-pfcp-timeout.patch" "70852d24026d7998a9ccd35c9a5c38240e9a3fa9"
check_patch_blob "0014-release-finalize-success-only.patch" "da6afd96849823d56284bdc10f63875c9377ee5a"
check_patch_blob "0015-release-finalize-fail-closed.patch" "799527c880a39228b1e5b5031607ad47859d6d62"
check_patch_blob "0016-relay-transaction-correlation.patch" "fee8f4fdfaf005d6c61ac578ccd43d5106a54599"
check_patch_blob "0017-pfcp-deletion-outcome.patch" "b162012ec2abf4f3b6536258926936e954ea30dd"
check_patch_blob "0018-release-state-observability.patch" "1b25ecaa979f75c1f7ca6352ac11f5c710170692"
check_patch_blob "0019-agent-restart-fail-closed.patch" "c2fb932cd9900a6c2723fec3bb7fcd0ef72880bb"
check_patch_blob "0020-upf-session-info.patch" "0272b2511698f0043be3cba0fa5f74033aaa400d"
check_patch_blob "0021-pfcp-unknown-reconcile.patch" "7b983e3c150045949a5a4ba8c91591fe78590d3e"
check_patch_blob "0022-ssm-parse-release-readonly.patch" "8cc2c728b9c5e84924ae18e0c2f0e5b04d4f9b6f"
check_patch_blob "0023-mas-durable-receipt.patch" "1da2f6a4078dee1a80ee660458b28ad844abadaa"

PATCH_COUNT=0
for patch_file in "$PATCH_DIR"/*.patch; do
  [[ -f "$patch_file" ]] || continue
  PATCH_COUNT=$((PATCH_COUNT + 1))
done
if [[ "$PATCH_COUNT" != "19" ]]; then
  echo "[FAIL] unexpected OpenAMSM patch count: $PATCH_COUNT (want 19)" >&2
  exit 2
fi

PATCH_IDENTITY="$(python3 - "$EXPECTED_TREE_SHA256" "$PATCH_DIR" <<'PY'
import hashlib
import pathlib
import sys

source_identity, patch_dir = sys.argv[1:]
digest = hashlib.sha256()
digest.update((source_identity + "\n").encode("utf-8"))
for patch in sorted(pathlib.Path(patch_dir).glob("*.patch"), key=lambda item: item.name):
    digest.update(("patch:" + patch.name + "\n").encode("utf-8"))
    digest.update(patch.read_bytes())
print(digest.hexdigest())
PY
)"

# Always invoke the repository build driver: it verifies upstream, recreates
# the staged source if the source+patch identity changed, then installs libraries.
"$ENV_OPENAMSM_ROOT/bin/env-openamsm" build
ACTUAL_PATCH_IDENTITY="$(cat "$BUILD_ROOT/.env-openamsm-source-identity")"
if [[ "$ACTUAL_PATCH_IDENTITY" != "$PATCH_IDENTITY" ]]; then
  echo "[FAIL] built OpenAMSM source+patch identity differs from the pinned set" >&2
  exit 2
fi

check_staged_blob() {
  local relative_path="$1"
  local expected_sha="$2"
  local actual_sha
  actual_sha="$(git -C "$ENV_OPENAMSM_ROOT" hash-object "$BUILD_SOURCE_ROOT/$relative_path")"
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    echo "[FAIL] OpenAMSM staged NAS source differs after patch application: $relative_path ($actual_sha)" >&2
    exit 2
  fi
}

# The environment patches are part of the build identity, but none may alter
# the NAS decoder/parser sources used as the independent oracle.
check_staged_blob "lib/nas/5gs/decoder.c" "fced527f7abc33365fbd3b913b1bce0cbdb80627"
check_staged_blob "lib/nas/5gs/ies.c" "0eede97b829f854d80ac55d65005ff0632a1c8ad"
check_staged_blob "lib/nas/5gs/types.c" "4eb990c5e7e2b7217c7eae546ee2a8e33b8c03e8"
check_staged_blob "lib/nas/5gs/types.h" "e7a29acb5cd9872e6280edda136e74c3e614910e"
check_staged_blob "lib/nas/5gs/message.h" "0dd6a721ea9e80d11286eac1727cd688f3e81634"
check_staged_blob "lib/nas/5gs/ogs-nas-5gs.h" "afac3b90942800fd1cc117e3b4b8da7622d5f729"
check_staged_blob "lib/nas/common/ogs-nas-common.h" "6b85aa4d6afecad578ccc1107f1152724bbc7f35"
check_staged_blob "lib/nas/common/types.h" "1bb31383ebfacddb607965f93551949cedf1643d"
check_staged_blob "lib/nas/common/types.c" "288d5f004653ec66cfbd3df8e7145cb5b9a08cb7"
check_staged_blob "lib/core/ogs-pkbuf.h" "74971b5fb29086d00c1a70bb79da2182ca3cfe25"
check_staged_blob "lib/core/ogs-core.h" "f72cc364b29dc069df5c470c7088a56555e6dc7d"
check_staged_blob "lib/core/ogs-log.h" "547f1962011c9dec5f48a3d3d97c509c4638eee7"

LIB_FILE="$(find "$INSTALL_ROOT" -type f \( -name 'libogsnas-5gs.so*' -o -name 'libogsnas-5gs*.dylib' \) -print | sed -n '1p')"
if [[ -z "$LIB_FILE" ]]; then
  echo "[FAIL] cannot locate installed libogsnas-5gs in $INSTALL_ROOT" >&2
  exit 2
fi
LIB_DIR="$(dirname -- "$LIB_FILE")"
case "$(uname -s)" in
  Darwin)
    NAS5GS_LINKER_NAME="$LIB_DIR/libogsnas-5gs.dylib"
    NAS_COMMON_LINKER_NAME="$LIB_DIR/libogsnas-common.dylib"
    CORE_LINKER_NAME="$LIB_DIR/libogscore.dylib"
    ;;
  Linux)
    NAS5GS_LINKER_NAME="$LIB_DIR/libogsnas-5gs.so"
    NAS_COMMON_LINKER_NAME="$LIB_DIR/libogsnas-common.so"
    CORE_LINKER_NAME="$LIB_DIR/libogscore.so"
    ;;
  *)
    echo "[FAIL] unsupported host for OpenAMSM C oracle: $(uname -s)" >&2
    exit 2
    ;;
esac
for linker_name in "$NAS5GS_LINKER_NAME" "$NAS_COMMON_LINKER_NAME" "$CORE_LINKER_NAME"; do
  if [[ ! -e "$linker_name" ]]; then
    echo "[FAIL] missing unversioned OpenAMSM linker name: $linker_name" >&2
    exit 2
  fi
done

CC="${CC:-cc}"
command -v "$CC" >/dev/null 2>&1 || { echo "[FAIL] C compiler not found: $CC" >&2; exit 2; }

OUT_DIR="${OPENAMSM_ORACLE_OUT:-}"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nas5gsm-openamsm-oracle.XXXXXX")"
else
  mkdir -p "$OUT_DIR"
  OUT_DIR="$(cd -- "$OUT_DIR" && pwd -P)"
fi

ORACLE_BIN="$OUT_DIR/openamsm-oracle"
"$CC" -std=gnu11 -D_GNU_SOURCE -O2 \
  -I"$BUILD_SOURCE_ROOT/lib" \
  -I"$BUILD_SOURCE_ROOT/lib/nas/common" \
  -I"$BUILD_ROOT/lib" \
  "$SCRIPT_DIR/openamsm-oracle.c" \
  -L"$LIB_DIR" -Wl,-rpath,"$LIB_DIR" \
  -logsnas-5gs -logsnas-common -logscore \
  -o "$ORACLE_BIN"

case "$(uname -s)" in
  Darwin)
    export DYLD_LIBRARY_PATH="$LIB_DIR${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}"
    ;;
  *)
    export LD_LIBRARY_PATH="$LIB_DIR${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    ;;
esac

FIXTURE_DIR="$REPO_ROOT/testdata/nas5gsm/openamsm-oracle"
for name in modify-request modify-command modify-complete; do
  "$ORACLE_BIN" "$FIXTURE_DIR/$name.hex" > "$OUT_DIR/$name.actual.json"
  diff -u "$FIXTURE_DIR/$name.expected.json" "$OUT_DIR/$name.actual.json"
  echo "[PASS] OpenAMSM C oracle: $name"
done

NAS_COMMON_LIB_FILE="$(find "$LIB_DIR" -type f \( -name 'libogsnas-common.so*' -o -name 'libogsnas-common*.dylib' \) -print | sed -n '1p')"
CORE_LIB_FILE="$(find "$LIB_DIR" -type f \( -name 'libogscore.so*' -o -name 'libogscore*.dylib' \) -print | sed -n '1p')"
if [[ -z "$NAS_COMMON_LIB_FILE" || -z "$CORE_LIB_FILE" ]]; then
  echo "[FAIL] cannot locate OpenAMSM NAS/common/core shared libraries in $LIB_DIR" >&2
  exit 2
fi

CC_VERSION="$("$CC" --version 2>&1 | sed -n '1p')"
ENV_OPENAMSM_COMMIT="$(git -C "$ENV_OPENAMSM_ROOT" rev-parse HEAD)"
python3 - "$OUT_DIR" "$FIXTURE_DIR" "$ENV_OPENAMSM_ROOT" "$ENV_OPENAMSM_COMMIT" "$EXPECTED_VERSION" \
  "$EXPECTED_TREE_SHA256" "$EXPECTED_ARCHIVE_SHA256" "$CC_VERSION" "$LIB_DIR" \
  "$PATCH_IDENTITY" "$PATCH_DIR" "$BUILD_SOURCE_ROOT" "$ORACLE_BIN" \
  "$LIB_FILE" "$NAS_COMMON_LIB_FILE" "$CORE_LIB_FILE" <<'PY'
import hashlib
import json
import os
import pathlib
import platform
import sys

(out_dir, fixture_dir, env_root, env_commit, version, tree_sha, archive_sha, cc_version,
 lib_dir, patch_identity, patch_dir, staged_root, oracle_bin, nas5gs_lib,
 nas_common_lib, core_lib) = sys.argv[1:]

def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()

cases = {}
for name in ("modify-request", "modify-command", "modify-complete"):
    fixture = os.path.join(fixture_dir, name + ".hex")
    expected = os.path.join(fixture_dir, name + ".expected.json")
    actual = os.path.join(out_dir, name + ".actual.json")
    with open(actual, encoding="utf-8") as stream:
        actual_json = stream.read().strip()
    cases[name] = {
        "input_sha256": sha256(fixture),
        "expected_sha256": sha256(expected),
        "actual_sha256": sha256(actual),
        "actual": actual_json,
        "result": "PASS",
    }

def git_blob_sha1(path):
    data = open(path, "rb").read()
    return hashlib.sha1(b"blob " + str(len(data)).encode("ascii") + bytes([0]) + data).hexdigest()

patches = []
for patch in sorted(pathlib.Path(patch_dir).glob("*.patch"), key=lambda item: item.name):
    patches.append({
        "path": patch.name,
        "git_blob_sha1": git_blob_sha1(patch),
        "sha256": sha256(patch),
    })

nas_source_files = (
    "lib/nas/5gs/decoder.c",
    "lib/nas/5gs/ies.c",
    "lib/nas/5gs/types.c",
    "lib/nas/5gs/types.h",
    "lib/nas/5gs/message.h",
    "lib/nas/5gs/ogs-nas-5gs.h",
    "lib/nas/common/ogs-nas-common.h",
    "lib/nas/common/types.h",
    "lib/nas/common/types.c",
    "lib/core/ogs-pkbuf.h",
    "lib/core/ogs-core.h",
    "lib/core/ogs-log.h",
)
staged_nas_blob_shas = {
    relative: git_blob_sha1(os.path.join(staged_root, relative))
    for relative in nas_source_files
}

manifest = {
    "oracle": "OpenAMSM native C decoder and QoS flow parser",
    "source": {
        "version": version,
        "archive_sha256": archive_sha,
        "tree_sha256": tree_sha,
        "archive_comment_commit": "c27e1761ae09957a474cc2b0208d227cde64aabf",
        "archive_comment_commit_is_verified_git_commit": False,
        "source_blob_shas": {
            "lib/nas/5gs/decoder.c": "fced527f7abc33365fbd3b913b1bce0cbdb80627",
            "lib/nas/5gs/ies.c": "0eede97b829f854d80ac55d65005ff0632a1c8ad",
            "lib/nas/5gs/types.c": "4eb990c5e7e2b7217c7eae546ee2a8e33b8c03e8",
            "lib/nas/5gs/types.h": "e7a29acb5cd9872e6280edda136e74c3e614910e",
            "lib/nas/5gs/message.h": "0dd6a721ea9e80d11286eac1727cd688f3e81634",
            "lib/nas/5gs/ogs-nas-5gs.h": "afac3b90942800fd1cc117e3b4b8da7622d5f729",
            "lib/nas/common/ogs-nas-common.h": "6b85aa4d6afecad578ccc1107f1152724bbc7f35",
            "lib/nas/common/types.h": "1bb31383ebfacddb607965f93551949cedf1643d",
            "lib/nas/common/types.c": "288d5f004653ec66cfbd3df8e7145cb5b9a08cb7",
            "lib/core/ogs-pkbuf.h": "74971b5fb29086d00c1a70bb79da2182ca3cfe25",
            "lib/core/ogs-core.h": "f72cc364b29dc069df5c470c7088a56555e6dc7d",
            "lib/core/ogs-log.h": "547f1962011c9dec5f48a3d3d97c509c4638eee7",
        },
        "decoder": "lib/nas/5gs/decoder.c:ogs_nas_5gsm_decode",
        "qos_flow_parser": "lib/nas/5gs/types.c:ogs_nas_parse_qos_flow_descriptions",
        "bitrate_semantics": "lib/nas/common/types.c:ogs_nas_bitrate_to_uint64",
    },
    "build": {
        "source_plus_patch_identity_sha256": patch_identity,
        "patches": patches,
        "staged_nas_source_blob_shas": staged_nas_blob_shas,
        "nas_source_unchanged_by_patches": True,
    },
    "environment": {
        "platform": platform.platform(),
        "compiler": cc_version,
        "library_directory": lib_dir,
        "env_openamsm_repo_commit": env_commit,
        "libraries": {
            "libogsnas-5gs": sha256(nas5gs_lib),
            "libogsnas-common": sha256(nas_common_lib),
            "libogscore": sha256(core_lib),
        },
        "oracle_harness_sha256": sha256(oracle_bin),
        "env_openamsm_root": env_root,
    },
    "cases": cases,
    "result": "NAS_5GSM_MODIFY_OWNER_ORACLE_PASS",
}
with open(os.path.join(out_dir, "manifest.json"), "w", encoding="utf-8") as stream:
    json.dump(manifest, stream, indent=2, sort_keys=True)
    stream.write("\n")
PY

echo "[PASS] NAS_5GSM_MODIFY_OWNER_ORACLE_PASS"
echo "Evidence manifest: $OUT_DIR/manifest.json"
