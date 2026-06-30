#!/usr/bin/env bash
# Simple helper to run uvicorn with an optional BACKEND_PORT environment variable
set -euo pipefail

PORT=${BACKEND_PORT:-8000}

if [ -f ".venv/bin/activate" ]; then
  # shellcheck disable=SC1091
  source .venv/bin/activate
fi

echo "Starting uvicorn on port ${PORT}"
uvicorn main:app --host 0.0.0.0 --port "${PORT}" --reload
