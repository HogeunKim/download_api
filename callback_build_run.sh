#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

MODE="${1:-start}"
BUILD_DIR="$SCRIPT_DIR/build"
BIN_PATH="$BUILD_DIR/callback-server"
LOG_PATH="$BUILD_DIR/callback-server.log"
PID_PATH="$BUILD_DIR/callback-server.pid"

mkdir -p "$BUILD_DIR"

is_running() {
  if [[ -f "$PID_PATH" ]]; then
    local pid
    pid="$(cat "$PID_PATH" 2>/dev/null || true)"
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
  fi
  return 1
}

stop_server() {
  if is_running; then
    local pid
    pid="$(cat "$PID_PATH")"
    echo "[callback] 기존 프로세스 종료: PID=$pid"
    kill "$pid" || true
    sleep 1
    if kill -0 "$pid" 2>/dev/null; then
      echo "[callback] 강제 종료: PID=$pid"
      kill -9 "$pid" || true
    fi
  fi
  rm -f "$PID_PATH"
}

start_server() {
  echo "[callback] build 시작..."
  go build -o "$BIN_PATH" ./cmd/callback-server
  chmod +x "$BIN_PATH"
  echo "[callback] build 완료: $BIN_PATH"

  stop_server

  echo "[callback] 백그라운드 실행..."
  nohup "$BIN_PATH" > "$LOG_PATH" 2>&1 &
  local pid=$!
  echo "$pid" > "$PID_PATH"

  sleep 1
  if kill -0 "$pid" 2>/dev/null; then
    echo "[callback] 실행 성공: PID=$pid"
    echo "[callback] 로그 파일: $LOG_PATH"
    echo "[callback] PID 파일: $PID_PATH"
    echo "[callback] 콜백 수신 URL: http://<ubuntu-ip>:9000/download-notify"
    echo "[callback] 이벤트 조회 URL: http://<ubuntu-ip>:9000/events"
  else
    echo "[callback] 실행 실패. 로그 확인: $LOG_PATH"
    exit 1
  fi
}

status_server() {
  if is_running; then
    echo "[callback] 실행 중: PID=$(cat "$PID_PATH")"
    echo "[callback] 로그 파일: $LOG_PATH"
  else
    echo "[callback] 실행 중인 프로세스 없음"
  fi
}

case "$MODE" in
  start)
    start_server
    ;;
  stop)
    stop_server
    echo "[callback] 중지 완료"
    ;;
  restart)
    stop_server
    start_server
    ;;
  status)
    status_server
    ;;
  *)
    echo "Usage: ./callback_build_run.sh [start|stop|restart|status]"
    exit 1
    ;;
esac
