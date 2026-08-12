#!/usr/bin/env bash
# Run DocForge API + worker in one terminal (Ctrl+C stops both).
# Expects binaries in bin/ and infra already up (make infra).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/apps/api/configs/local.env}"
BIN="$ROOT/bin"
LOG_DIR="${LOG_DIR:-$ROOT/.run}"
mkdir -p "$LOG_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — run: make env" >&2
  exit 1
fi

if [[ ! -x "$BIN/api" || ! -x "$BIN/worker" ]]; then
  echo "Missing bin/api or bin/worker — run: make build-go" >&2
  exit 1
fi

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

PIDS=()
STOPPING=0

kill_tree() {
  local pid="$1"
  local sig="${2:-TERM}"
  [[ -z "$pid" ]] && return 0
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  local child
  while IFS= read -r child; do
    [[ -n "$child" && "$child" != "$pid" ]] && kill_tree "$child" "$sig"
  done < <(pgrep -P "$pid" 2>/dev/null || true)
  kill -s "$sig" "$pid" 2>/dev/null || true
}

any_pid_alive() {
  local pid
  for pid in "${PIDS[@]:-}"; do
    kill -0 "$pid" 2>/dev/null && return 0
  done
  return 1
}

stop_all() {
  if [[ "$STOPPING" == 1 ]]; then
    for pid in "${PIDS[@]:-}"; do
      kill_tree "$pid" KILL
    done
    exit 130
  fi
  STOPPING=1
  trap - INT TERM EXIT

  echo ""
  echo "[run-all] Stopping processes..."
  for pid in "${PIDS[@]:-}"; do
    kill_tree "$pid" TERM
  done

  local deadline=$((SECONDS + 10))
  while any_pid_alive && (( SECONDS < deadline )); do
    sleep 0.25
  done

  if any_pid_alive; then
    echo "[run-all] Sending SIGKILL..."
    for pid in "${PIDS[@]:-}"; do
      kill_tree "$pid" KILL
    done
    sleep 0.5
  fi

  echo "[run-all] Done. Logs: $LOG_DIR"
  exit 0
}

trap stop_all INT TERM EXIT

start_one() {
  local name="$1"
  local bin="$2"
  echo "[run-all] starting $name → $LOG_DIR/$name.log"
  "$bin" >>"$LOG_DIR/$name.log" 2>&1 &
  PIDS+=("$!")
}

start_one api "$BIN/api"
start_one worker "$BIN/worker"

echo "[run-all] API :8080  worker consuming RabbitMQ  logs in $LOG_DIR/"
echo "[run-all] Frontend: make run-web   Infra: make infra-logs"
echo "[run-all] Ctrl+C to stop."

wait
