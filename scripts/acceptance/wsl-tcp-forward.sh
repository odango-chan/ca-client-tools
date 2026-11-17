#!/usr/bin/env bash
# Lifecycle wrapper for the minimal loopback-only WSL TCP forwarder used by the
# Windows-native 5GC acceptance. Run it INSIDE the WSL distribution that hosts
# the OpenAMSM deployment, for example from Windows:
#
#   wsl.exe -d <Distro> -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh start
#   wsl.exe -d <Distro> -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh status
#   wsl.exe -d <Distro> -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh stop
#
# State (pid file + log) defaults to ~/.ca-acceptance/wsl-forwarder and can be
# overridden with CA_ACCEPTANCE_STATE_DIR.
#
# The forwarder binds loopback only and forwards exactly three services. It is
# byte-transparent, so the SMF SBI h2c stream is not terminated or rewritten.
# Target addresses/ports can be overridden with:
#   CA_WSL_FORWARD_SMF_SBI_TARGET       (default 10.30.20.4:7777)
#   CA_WSL_FORWARD_AMF_METRICS_TARGET   (default 10.30.20.5:9090)
#   CA_WSL_FORWARD_SMF_METRICS_TARGET   (default 10.30.20.4:9090)
#   CA_WSL_FORWARD_SBI_PORT             (default 17777)
#   CA_WSL_FORWARD_AMF_METRICS_PORT     (default 19090)
#   CA_WSL_FORWARD_SMF_METRICS_PORT     (default 29090)
set -u

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PY="${SCRIPT_DIR}/wsl-tcp-forward.py"
STATE_DIR="${CA_ACCEPTANCE_STATE_DIR:-$HOME/.ca-acceptance/wsl-forwarder}"
PIDFILE="${STATE_DIR}/forwarder.pid"
LOGFILE="${STATE_DIR}/forwarder.log"
SBI_PORT="${CA_WSL_FORWARD_SBI_PORT:-17777}"
AMF_PORT="${CA_WSL_FORWARD_AMF_METRICS_PORT:-19090}"
SMF_PORT="${CA_WSL_FORWARD_SMF_METRICS_PORT:-29090}"
SBI_TARGET="${CA_WSL_FORWARD_SMF_SBI_TARGET:-10.30.20.4:7777}"
AMF_TARGET="${CA_WSL_FORWARD_AMF_METRICS_TARGET:-10.30.20.5:9090}"
SMF_TARGET="${CA_WSL_FORWARD_SMF_METRICS_TARGET:-10.30.20.4:9090}"

mkdir -p "$STATE_DIR"

running_pid() {
  [ -f "$PIDFILE" ] || return 1
  local pid
  pid="$(tr -d '[:space:]' <"$PIDFILE" 2>/dev/null)"
  [ -n "$pid" ] || return 1
  [ -d "/proc/$pid" ] || return 1
  tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null | grep -q 'wsl-tcp-forward.py' || return 1
  echo "$pid"
}

listen_lines() {
  ss -ltn 2>/dev/null | grep -E "127\.0\.0\.1:(${SBI_PORT}|${AMF_PORT}|${SMF_PORT}) " || true
}

probe_backends() {
  local name target host port
  while read -r name target; do
    [ -n "$name" ] || continue
    host="${target%:*}"
    port="${target##*:}"
    if command -v nc >/dev/null 2>&1; then
      if nc -z -w 2 "$host" "$port" 2>/dev/null; then
        echo "  backend $name ${host}:${port} REACHABLE"
      else
        echo "  backend $name ${host}:${port} UNREACHABLE"
      fi
    else
      echo "  backend $name ${host}:${port} (nc not installed; not probed)"
    fi
  done <<EOF
smf-sbi $SBI_TARGET
amf-metrics $AMF_TARGET
smf-metrics $SMF_TARGET
EOF
}

do_start() {
  local pid
  if pid="$(running_pid)"; then
    echo "forwarder already running pid=$pid"
    return 0
  fi
  rm -f "$PIDFILE"
  # Detach from the invoking wsl.exe session; output goes to the log file.
  setsid nohup python3 -u "$PY" \
    --pidfile "$PIDFILE" \
    --sbi-listen "$SBI_PORT" \
    --metrics-amf-listen "$AMF_PORT" \
    --metrics-smf-listen "$SMF_PORT" \
    --smf-sbi "$SBI_TARGET" \
    --amf-metrics "$AMF_TARGET" \
    --smf-metrics "$SMF_TARGET" \
    >>"$LOGFILE" 2>&1 &
  local waited=0
  while [ "$waited" -lt 60 ]; do
    sleep 0.1
    waited=$((waited + 1))
    if [ -f "$PIDFILE" ]; then
      pid="$(tr -d '[:space:]' <"$PIDFILE")"
      if [ -n "$pid" ] && [ -d "/proc/$pid" ]; then
        echo "forwarder started pid=$pid"
        echo "  log=$LOGFILE"
        echo "  pidfile=$PIDFILE"
        listen_lines
        probe_backends
        return 0
      fi
    fi
  done
  echo "ERROR: forwarder did not become ready in 6s; see $LOGFILE" >&2
  tail -n 20 "$LOGFILE" >&2 || true
  return 1
}

do_stop() {
  local pid
  if ! pid="$(running_pid)"; then
    echo "forwarder not running (stale pidfile removed)"
    rm -f "$PIDFILE"
    return 0
  fi
  kill -TERM "$pid" 2>/dev/null || true
  local waited=0
  while [ "$waited" -lt 50 ]; do
    sleep 0.1
    waited=$((waited + 1))
    [ -d "/proc/$pid" ] || break
  done
  if [ -d "/proc/$pid" ]; then
    echo "forwarder pid=$pid did not stop on SIGTERM; sending SIGKILL"
    kill -KILL "$pid" 2>/dev/null || true
    sleep 0.2
  fi
  rm -f "$PIDFILE"
  echo "forwarder stopped (was pid=$pid)"
  return 0
}

do_status() {
  local pid
  if pid="$(running_pid)"; then
    echo "forwarder: RUNNING pid=$pid"
    echo "  pidfile=$PIDFILE"
    echo "  log=$LOGFILE"
    echo "  listeners:"
    listen_lines | sed 's/^/    /'
    probe_backends
    return 0
  fi
  echo "forwarder: NOT RUNNING"
  echo "  pidfile=$PIDFILE"
  echo "  log=$LOGFILE"
  return 3
}

case "${1:-}" in
  start)   do_start ;;
  stop)    do_stop ;;
  restart) do_stop; do_start ;;
  status)  do_status ;;
  *)
    echo "Usage: $0 {start|stop|restart|status}" >&2
    exit 2
    ;;
esac
