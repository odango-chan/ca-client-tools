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
check_blob "lib/nas/common/types.c" "288d5f004653ec66cfbd3df8e7145cb5b9a08cb7"
check_blob "lib/core/ogs-pkbuf.h" "74971b5fb29086d00c1a70bb79da2182ca3cfe25"
check_blob "lib/core/ogs-core.h" "f72cc364b29dc069df5c470c7088a56555e6dc7d"

if [[ ! -f "$BUILD_ROOT/lib/core/core-config.h" ]] ||
   ! find "$INSTALL_ROOT" -type f \( -name 'libogsnas-5gs.so*' -o -name 'libogsnas-5gs.dylib' \) -print -quit 2>/dev/null | grep -q .; then
  echo "[INFO] OpenAMSM native build/install is missing; invoking env-openamsm build"
  "$ENV_OPENAMSM_ROOT/bin/env-openamsm" build
fi

LIB_FILE="$(find "$INSTALL_ROOT" -type f \( -name 'libogsnas-5gs.so*' -o -name 'libogsnas-5gs.dylib' \) -print -quit)"
if [[ -z "$LIB_FILE" ]]; then
  echo "[FAIL] cannot locate installed libogsnas-5gs in $INSTALL_ROOT" >&2
  exit 2
fi
LIB_DIR="$(dirname -- "$LIB_FILE")"
if [[ ! -e "$LIB_DIR/libogsnas-5gs.so" && ! -e "$LIB_DIR/libogsnas-5gs.dylib" ]]; then
  echo "[FAIL] missing unversioned libogsnas-5gs linker name in $LIB_DIR" >&2
  exit 2
fi

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
  -I"$SOURCE_ROOT/lib" \
  -I"$SOURCE_ROOT/lib/nas/common" \
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

CC_VERSION="$("$CC" --version 2>&1 | sed -n '1p')"
python3 - "$OUT_DIR" "$FIXTURE_DIR" "$ENV_OPENAMSM_ROOT" "$EXPECTED_VERSION" \
  "$EXPECTED_TREE_SHA256" "$EXPECTED_ARCHIVE_SHA256" "$CC_VERSION" "$LIB_DIR" <<'PY'
import hashlib
import json
import os
import platform
import sys

out_dir, fixture_dir, env_root, version, tree_sha, archive_sha, cc_version, lib_dir = sys.argv[1:]

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
            "lib/nas/common/types.c": "288d5f004653ec66cfbd3df8e7145cb5b9a08cb7",
            "lib/core/ogs-pkbuf.h": "74971b5fb29086d00c1a70bb79da2182ca3cfe25",
            "lib/core/ogs-core.h": "f72cc364b29dc069df5c470c7088a56555e6dc7d",
        },
        "decoder": "lib/nas/5gs/decoder.c:ogs_nas_5gsm_decode",
        "qos_flow_parser": "lib/nas/5gs/types.c:ogs_nas_parse_qos_flow_descriptions",
        "bitrate_semantics": "lib/nas/common/types.c:ogs_nas_bitrate_to_uint64",
    },
    "environment": {
        "platform": platform.platform(),
        "compiler": cc_version,
        "library_directory": lib_dir,
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
