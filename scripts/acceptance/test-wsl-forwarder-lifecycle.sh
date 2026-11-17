#!/usr/bin/env bash
# Lifecycle test for wsl-tcp-forward.sh -- run INSIDE WSL/Linux.
#
#   wsl.exe -d Ubuntu-26.04 -u root -- bash /mnt/c/<repo>/scripts/acceptance/test-wsl-forwarder-lifecycle.sh
#
# It exercises start/status/restart/stop of the optional acceptance forwarder
# wrapper against an ISOLATED state directory and NON-DEFAULT free ports, so an
# existing live acceptance forwarder on the default ports
# (127.0.0.1:17777/19090/29090) is never disturbed: this test never reads,
# writes or kills that PID, and it additionally records that listener's PID
# before the test and re-asserts it afterwards.
#
# Asserted:
#   1. start   -> RUNNING pid on exactly the three expected 127.0.0.1 listeners
#   2. status  -> RUNNING with the same PID, exit 0
#   3. restart -> new PID, old PID gone, still exactly three 127.0.0.1 listeners
#   4. stop    -> NOT RUNNING (status exit 3) and no listeners left
#   5. every declared listener is loopback-only, even with
#      CA_WSL_FORWARD_BIND=0.0.0.0 set in the environment (the bind variable
#      is deliberately ignored by the forwarder)
#   6. the pre-existing default-port forwarder (if any) is still the same PID
#   7. only the three declared ports are opened; no fourth listener
#
# Exit code 0 = all assertions PASS, 1 = FAIL.
set -u

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WRAPPER="${SCRIPT_DIR}/wsl-tcp-forward.sh"
[ -f "$WRAPPER" ] || { echo "FAIL: wrapper not found: $WRAPPER" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "FAIL: python3 is required" >&2; exit 1; }
command -v ss >/dev/null 2>&1 || { echo "FAIL: ss (iproute2) is required" >&2; exit 1; }

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }
pass() { echo "PASS: $*"; }

STATE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ca-wsl-forwarder-lifecycle.XXXXXX")" || {
  echo "FAIL: could not create a temporary state directory" >&2
  exit 1
}

cleanup() {
  local pidfile="${STATE_DIR}/forwarder.pid" pid
  if [ -f "$pidfile" ]; then
    pid="$(tr -d '[:space:]' <"$pidfile" 2>/dev/null || true)"
    if [ -n "${pid:-}" ] && [ -d "/proc/${pid}" ]; then
      kill -TERM "$pid" 2>/dev/null || true
      sleep 0.3
    fi
  fi
  rm -rf "$STATE_DIR"
}
trap cleanup EXIT

# Pick three distinct free high ports for this test. The live acceptance
# forwarder keeps the default ports; these are never used by the test.
PORT_TRIO="$(python3 - <<'PY'
import socket
sockets = []
ports = []
try:
    for _ in range(3):
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        sockets.append(sock)
        ports.append(sock.getsockname()[1])
finally:
    for sock in sockets:
        sock.close()
print(" ".join(str(port) for port in ports))
PY
)" || { echo "FAIL: could not allocate test ports" >&2; exit 1; }
read -r SBI_PORT AMF_PORT SMF_PORT <<<"$PORT_TRIO"
for port in "$SBI_PORT" "$AMF_PORT" "$SMF_PORT"; do
  case "$port" in
    ''|*[!0-9]*) echo "FAIL: invalid test port '$port'" >&2; exit 1 ;;
  esac
done
[ "$SBI_PORT" -ne "$AMF_PORT" ] && [ "$SBI_PORT" -ne "$SMF_PORT" ] && [ "$AMF_PORT" -ne "$SMF_PORT" ] || {
  echo "FAIL: test ports are not distinct: $PORT_TRIO" >&2
  exit 1
}

# Isolated state: never the default ~/.ca-acceptance/wsl-forwarder.
export CA_ACCEPTANCE_STATE_DIR="$STATE_DIR"
export CA_WSL_FORWARD_SBI_PORT="$SBI_PORT"
export CA_WSL_FORWARD_AMF_METRICS_PORT="$AMF_PORT"
export CA_WSL_FORWARD_SMF_METRICS_PORT="$SMF_PORT"
# Backends are irrelevant for the listener lifecycle; use a closed loopback
# port so the test never touches a real NF service.
export CA_WSL_FORWARD_SMF_SBI_TARGET="127.0.0.1:1"
export CA_WSL_FORWARD_AMF_METRICS_TARGET="127.0.0.1:1"
export CA_WSL_FORWARD_SMF_METRICS_TARGET="127.0.0.1:1"
# Deliberately attempt to widen the bind: the forwarder must ignore this and
# still bind only 127.0.0.1.
export CA_WSL_FORWARD_BIND="0.0.0.0"

PIDFILE="${STATE_DIR}/forwarder.pid"
LOGFILE="${STATE_DIR}/forwarder.log"

default_listener_pid() {
  ss -ltnpH 2>/dev/null | grep ':17777 ' | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | head -n 1
}

port_listeners() {
  ss -ltnH 2>/dev/null | awk '{print $4}' |
    grep -E ":(${SBI_PORT}|${AMF_PORT}|${SMF_PORT})$" || true
}

assert_three_loopback_listeners() {
  local label="$1" lines count port matches
  lines="$(port_listeners)"
  count="$(printf '%s\n' "$lines" | grep -c . || true)"
  if [ "$count" -ne 3 ]; then
    fail "$label: expected exactly three declared listeners, got $count: ${lines:-<none>}"
    return
  fi
  for port in "$SBI_PORT" "$AMF_PORT" "$SMF_PORT"; do
    matches="$(printf '%s\n' "$lines" | grep -c ":${port}$" || true)"
    if [ "$matches" -ne 1 ]; then
      fail "$label: expected exactly one listener on port $port, got $matches: $lines"
      continue
    fi
    if ! printf '%s\n' "$lines" | grep -q "^127\\.0\\.0\\.1:${port}$"; then
      fail "$label: port $port is not bound to 127.0.0.1: ${lines}"
      continue
    fi
  done
  if printf '%s\n' "$lines" | grep -qv '^127\.0\.0\.1:'; then
    fail "$label: a declared port is bound to a non-loopback address: ${lines}"
    return
  fi
  pass "$label: exactly three 127.0.0.1 listeners on ${SBI_PORT}/${AMF_PORT}/${SMF_PORT}"
}

assert_no_test_listeners() {
  local label="$1" lines
  lines="$(port_listeners)"
  if [ -n "$lines" ]; then
    fail "$label: expected no listeners on the test ports, got: $lines"
  else
    pass "$label: no listeners remain on the test ports"
  fi
}

echo "== wsl forwarder lifecycle test =="
echo "state dir: $STATE_DIR"
echo "test ports: $SBI_PORT $AMF_PORT $SMF_PORT"
LIVE_DEFAULT_PID="$(default_listener_pid)"
if [ -n "$LIVE_DEFAULT_PID" ]; then
  echo "pre-existing live default-port forwarder pid: $LIVE_DEFAULT_PID (must not be disturbed)"
else
  echo "note: no live default-port forwarder detected; default-port preservation check will be skipped"
fi

# --- 1. start ---------------------------------------------------------------
if ! START_OUT="$(bash "$WRAPPER" start 2>&1)"; then
  echo "$START_OUT" >&2
  fail "start exited non-zero"
else
  if printf '%s\n' "$START_OUT" | grep -q 'forwarder started pid='; then
    PID1="$(tr -d '[:space:]' <"$PIDFILE" 2>/dev/null || true)"
    if [ -n "$PID1" ] && [ -d "/proc/$PID1" ]; then
      pass "start: RUNNING pid=$PID1"
    else
      fail "start: pidfile does not contain a live PID (${PID1:-<empty>})"
    fi
  else
    echo "$START_OUT" >&2
    fail "start: did not report 'forwarder started pid='"
  fi
fi
PID1="${PID1:-}"
assert_three_loopback_listeners "after start"

# --- 2. status -------------------------------------------------------------
if STATUS_OUT="$(bash "$WRAPPER" status 2>&1)"; then
  if printf '%s\n' "$STATUS_OUT" | grep -q 'forwarder: RUNNING'; then
    if [ -n "$PID1" ] && printf '%s\n' "$STATUS_OUT" | grep -q "pid=$PID1"; then
      pass "status: RUNNING pid=$PID1 exit 0"
    else
      fail "status: RUNNING but PID does not match pidfile ($PID1)"
    fi
  else
    fail "status: exit 0 but no RUNNING line"
  fi
else
  fail "status: expected exit 0 while running"
fi
assert_three_loopback_listeners "status"

# --- 3. restart ------------------------------------------------------------
if ! RESTART_OUT="$(bash "$WRAPPER" restart 2>&1)"; then
  echo "$RESTART_OUT" >&2
  fail "restart exited non-zero"
else
  PID2="$(tr -d '[:space:]' <"$PIDFILE" 2>/dev/null || true)"
  if [ -n "$PID2" ] && [ -d "/proc/$PID2" ]; then
    if [ "$PID2" != "$PID1" ]; then
      pass "restart: new PID $PID2 (previous $PID1)"
    else
      fail "restart: PID did not change ($PID2)"
    fi
  else
    fail "restart: pidfile does not contain a live PID (${PID2:-<empty>})"
  fi
  if [ -n "$PID1" ] && [ -d "/proc/$PID1" ]; then
    fail "restart: previous PID $PID1 is still alive"
  fi
fi
PID2="${PID2:-}"
assert_three_loopback_listeners "after restart"

# --- 4. stop ---------------------------------------------------------------
if STOP_OUT="$(bash "$WRAPPER" stop 2>&1)"; then
  if printf '%s\n' "$STOP_OUT" | grep -q 'forwarder stopped'; then
    pass "stop: forwarder stopped (pid=$PID2)"
  else
    fail "stop: did not report 'forwarder stopped'"
  fi
else
  fail "stop: exited non-zero"
fi
if [ -n "$PID2" ] && [ -d "/proc/$PID2" ]; then
  fail "stop: process $PID2 is still alive"
fi
assert_no_test_listeners "after stop"

STATUS_STOP_OUT="$(bash "$WRAPPER" status 2>&1)"
STATUS_STOP_RC=$?
if [ "$STATUS_STOP_RC" -eq 3 ]; then
  pass "status after stop: exit 3 (NOT RUNNING)"
else
  echo "$STATUS_STOP_OUT" >&2
  fail "status after stop: expected exit 3, got $STATUS_STOP_RC"
fi
if printf '%s\n' "$STATUS_STOP_OUT" | grep -q 'forwarder: NOT RUNNING'; then
  pass "status after stop: reports NOT RUNNING"
else
  fail "status after stop: does not report NOT RUNNING"
fi

# --- 5. pre-existing live default-port forwarder untouched ------------------
if [ -n "$LIVE_DEFAULT_PID" ]; then
  NOW_DEFAULT_PID="$(default_listener_pid)"
  if [ "$NOW_DEFAULT_PID" = "$LIVE_DEFAULT_PID" ]; then
    pass "live default-port forwarder untouched (still pid=$LIVE_DEFAULT_PID)"
  else
    fail "live default-port forwarder changed: was pid=$LIVE_DEFAULT_PID, now '${NOW_DEFAULT_PID:-<none>}'"
  fi
fi

echo
if [ "$FAILURES" -eq 0 ]; then
  echo "RESULT: PASS (wsl forwarder lifecycle)"
  exit 0
fi
echo "RESULT: FAIL ($FAILURES failed assertion(s))" >&2
exit 1
