# Next.js + shadcn/ui → FastAPI Monorepo

This project is a full-stack app with a Next.js (React) frontend (with
shadcn/ui) and a FastAPI backend.

---

## Directory Structure

- `frontend/` — Next.js 16 + React 19 + shadcn/ui (TypeScript)
- `backend/` — FastAPI + Uvicorn (Python 3.11)

---

## Developer Setup

### Prerequisites
- Node.js 24.x (in your PATH)
- Python 3.11.x (in your PATH)
- pnpm (in your PATH)

### 1. Frontend Setup
```bash
cd frontend

# (optional) Scaffold the shadcn components (this will add files to `frontend/components/ui`):
pnpm dlx shadcn@latest init
pnpm dlx shadcn@latest add button

pnpm dev
```

Visit `http://localhost:3000` to see the Hello World page.

### 2. Backend Setup
```bash
cd backend
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
uvicorn main:app --reload
```

Visit `http://localhost:8000` to see the FastAPI Hello World endpoint, and try the example query endpoint:

- `http://localhost:8000/hello?name=YourName` => { "message": "Hello, YourName from FastAPI!" }

## Port configuration

- Frontend: set `FRONTEND_PORT` environment variable to choose the dev port (default `3000`). Example:

```bash
FRONTEND_PORT=4000 pnpm dev
```

- Backend: set `BACKEND_PORT` environment variable (default `8000`). Example:

```bash
BACKEND_PORT=9000 ./run_uvicorn.sh
```

Run both services together

You can run both the frontend and backend dev servers together with `run-dev.sh` at the repository root. The script starts both servers, writes logs to `./logs/`, and stops both cleanly if it is interrupted (SIGINT/SIGTERM).

```bash
# default ports (frontend:3000, backend:8000)
./run-dev.sh

# custom ports
./run-dev.sh -f 4000 -b 9000
```

Logs are written to `./logs/frontend.log` and `./logs/backend.log`.
Follow them with: `tail -F logs/frontend.log logs/backend.log`.

---

## Production Deployment

### Frontend
```bash
cd frontend
pnpm build
pnpm start
```

### Backend
```bash
cd backend
source .venv/bin/activate
uvicorn main:app --host 0.0.0.0 --port 8000
```

---

## Notes
- Each developer should create their own `.venv` in `backend/` and install
  dependencies there.
- Do not share virtual environments between users.
- The `pnpm` binary can be shared, but each user’s global packages/cache are
  separate by default.
