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

echo "Running frontend format and lint check on changed files..."

# Get list of changed files in frontend directory that match supported extensions
CHANGED_FILES=$(git diff --name-only --diff-filter=d HEAD | grep "^frontend/.*\.\(ts\|tsx\|js\|jsx\|json\|css\)$" || true)
# ESLint only runs on JS/TS files (not JSON/CSS)
ESLINT_FILES=$(git diff --name-only --diff-filter=d HEAD | grep "^frontend/.*\.\(ts\|tsx\|js\|jsx\)$" || true)

if [ -n "$CHANGED_FILES" ]; then
    # Strip 'frontend/' prefix for running commands inside the directory
    RELATIVE_FILES=$(echo "$CHANGED_FILES" | sed 's/^frontend\///')
    RELATIVE_ESLINT_FILES=$(echo "$ESLINT_FILES" | sed 's/^frontend\///' || true)
    
    # Run prettier on all files, eslint only on JS/TS files
    LINT_FAILED=0
    if ! (cd frontend && echo "$RELATIVE_FILES" | xargs pnpm prettier --write > /dev/null 2>&1); then
        LINT_FAILED=1
    fi
    if [ -n "$RELATIVE_ESLINT_FILES" ]; then
        if ! (cd frontend && echo "$RELATIVE_ESLINT_FILES" | xargs pnpm eslint > /dev/null 2>&1); then
            LINT_FAILED=1
        fi
    fi
    
    if [ "$LINT_FAILED" -eq 1 ]; then
        echo "Format or lint check failed on changed files. Running again with output:"
        (cd frontend && echo "$RELATIVE_FILES" | xargs pnpm prettier --write)
        if [ -n "$RELATIVE_ESLINT_FILES" ]; then
            (cd frontend && echo "$RELATIVE_ESLINT_FILES" | xargs pnpm eslint)
        fi
        exit 1
    fi
    echo "Frontend checks passed on $(echo "$CHANGED_FILES" | wc -l) file(s)."
else
    echo "No changed frontend files to check."
fi

# Frontend tests (only when relevant files changed)
if [ -n "$CHANGED_FILES" ]; then
  echo "Running frontend unit tests on changed tree..."
  if ! (cd frontend && pnpm test > /dev/null 2>&1); then
    echo "Frontend unit tests failed. Running again with output:"
    (cd frontend && pnpm test)
    exit 1
  fi
  echo "Frontend unit tests passed."
else
  echo "No frontend changes requiring unit tests."
fi

# Backend linting with ruff (if installed)
echo "Running backend lint check..."
BACKEND_CHANGED=$(git diff --name-only --diff-filter=d HEAD | grep "^backend/.*\.py$" || true)

if [ -n "$BACKEND_CHANGED" ]; then
    if command -v ruff &> /dev/null || [ -x "backend/.venv/bin/ruff" ]; then
        RUFF_CMD="ruff"
        if [ -x "backend/.venv/bin/ruff" ]; then
            RUFF_CMD="backend/.venv/bin/ruff"
        fi
        
        if ! $RUFF_CMD check backend/ > /dev/null 2>&1; then
            echo "Backend lint check failed:"
            $RUFF_CMD check backend/
            exit 1
        fi
        echo "Backend checks passed on $(echo "$BACKEND_CHANGED" | wc -l) file(s)."
    else
        echo "Skipping backend lint (ruff not installed - run: pip install -r backend/requirements-dev.txt)"
    fi
else
    echo "No changed backend files to check."
fi

# Backend tests (only when Python files changed)
if [ -n "$BACKEND_CHANGED" ]; then
  echo "Running backend unit tests..."
  if ! (cd backend && { if [ -f .venv/bin/activate ]; then \
      # shellcheck disable=SC1091
      . .venv/bin/activate; \
    fi; command -v pytest > /dev/null 2>&1; }); then
    echo "pytest is not available. Install backend dev dependencies (pip install -r backend/requirements-dev.txt)."
    exit 1
  fi
  if ! (cd backend && { if [ -f .venv/bin/activate ]; then \
      # shellcheck disable=SC1091
      . .venv/bin/activate; \
    fi; pytest > /dev/null 2>&1; }); then
    echo "Backend unit tests failed. Running again with output:"
    (cd backend && { if [ -f .venv/bin/activate ]; then \
      # shellcheck disable=SC1091
      . .venv/bin/activate; \
    fi; pytest; })
    exit 1
  fi
  echo "Backend unit tests passed."
else
  echo "No backend changes requiring unit tests."
fi

mkdir -p logs

echo "Starting backend on port ${BACKEND_PORT} (logs: logs/backend.log)"
# Use setsid so the command runs in its own session/process-group; we'll kill the group on exit.
setsid bash -lc "cd backend && BACKEND_PORT=${BACKEND_PORT} ./run_uvicorn.sh" > logs/backend.log 2>&1 &
BACK_PID=$!

echo "Starting frontend on port ${FRONTEND_PORT} (logs: logs/frontend.log)"
setsid bash -lc "cd frontend && FRONTEND_PORT=${FRONTEND_PORT} BACKEND_PORT=${BACKEND_PORT} pnpm dev" > logs/frontend.log 2>&1 &
FRONT_PID=$!

echo "Frontend PID: ${FRONT_PID}, Backend PID: ${BACK_PID}"

# Wait for services to be ready
if command -v curl >/dev/null; then
    wait_for_url() {
        local url="$1"
        local name="$2"
        local max_attempts=60
        local attempt=1

        echo -n "Waiting for $name to be ready at $url..."
        while [ $attempt -le $max_attempts ]; do
            if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q "200"; then
                echo " Ready!"
                return 0
            fi
            echo -n "."
            sleep 1
            attempt=$((attempt + 1))
        done
        echo " Timeout!"
        return 1
    }

    wait_for_url "http://localhost:${BACKEND_PORT}/api/v1/health" "Backend"
    wait_for_url "http://localhost:${FRONTEND_PORT}/api/health" "Frontend"
    # Warm up the main page so the first browser visit is fast
    wait_for_url "http://localhost:${FRONTEND_PORT}/" "Frontend (Warmup)"
else
    echo "curl not found, skipping health checks."
fi

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
