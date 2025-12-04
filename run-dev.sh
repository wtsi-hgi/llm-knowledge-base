#!/usr/bin/env bash
# Start frontend and backend dev servers and stop them cleanly on exit.
set -euo pipefail

usage() {
  cat <<-EOF
Usage: $0 [--frontend-port PORT] [--backend-port PORT]

Starts frontend and backend in development mode.

Options:
  -f, --frontend-port PORT   Port for frontend dev server (default: 3000)
  -b, --backend-port PORT    Port for backend uvicorn server (default: 8000)
  -h, --help                 Show this help

Examples:
  # start frontend on 3000 and backend on 8000
  $0

  # custom ports
  $0 --frontend-port 4000 --backend-port 9000

Logs are written to ./logs/frontend.log and ./logs/backend.log
EOF
}

FRONTEND_PORT=3000
BACKEND_PORT=8000

while [[ ${#} -gt 0 ]]; do
  case "$1" in
    -f|--frontend-port)
      FRONTEND_PORT=${2:-}
      shift 2
      ;;
    -b|--backend-port)
      BACKEND_PORT=${2:-}
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

echo "Running frontend lint check..."
(cd frontend && pnpm lint)
echo "Lint check passed."

mkdir -p logs

echo "Starting backend on port ${BACKEND_PORT} (logs: logs/backend.log)"
# Use setsid so the command runs in its own session/process-group; we'll kill the group on exit.
setsid bash -lc "cd backend && BACKEND_PORT=${BACKEND_PORT} ./run_uvicorn.sh" > logs/backend.log 2>&1 &
BACK_PID=$!

echo "Starting frontend on port ${FRONTEND_PORT} (logs: logs/frontend.log)"
setsid bash -lc "cd frontend && FRONTEND_PORT=${FRONTEND_PORT} BACKEND_PORT=${BACKEND_PORT} pnpm dev" > logs/frontend.log 2>&1 &
FRONT_PID=$!

echo "Frontend PID: ${FRONT_PID}, Backend PID: ${BACK_PID}"
echo "Tail logs with: tail -F logs/frontend.log logs/backend.log"

cleanup() {
  echo "Stopping services..."
  # kill process groups if possible
  if [[ -n "${FRONT_PID:-}" ]]; then
    echo "Killing frontend group (PID ${FRONT_PID})"
    kill -TERM -"${FRONT_PID}" 2>/dev/null || kill -TERM "${FRONT_PID}" 2>/dev/null || true
  fi
  if [[ -n "${BACK_PID:-}" ]]; then
    echo "Killing backend group (PID ${BACK_PID})"
    kill -TERM -"${BACK_PID}" 2>/dev/null || kill -TERM "${BACK_PID}" 2>/dev/null || true
  fi
  # wait for processes to exit
  wait 2>/dev/null || true
  echo "Stopped."
}

trap 'cleanup; exit' INT TERM EXIT

# Wait until signals are received; sleep in a loop so trap can fire.
while true; do
  sleep 1
done
