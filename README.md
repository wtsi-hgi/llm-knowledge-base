# Next.js + shadcn/ui → FastAPI Monorepo

This project is a full-stack app with a Next.js (React) frontend using the
**App Router**, Tailwind CSS v4 + shadcn/ui, and a FastAPI backend.

### Highlights

- **Server Actions** call FastAPI directly from the server without exposing
  backend URLs to the browser.
- **Typed contracts**: FastAPI responses are validated on the frontend with
  `zod` so regressions are caught immediately.
- **Modern styling**: Tailwind v4 `@theme` tokens power shadcn/ui components,
  theme switching, and animation primitives.
- **Tests built-in**: Vitest covers the shared contracts, while pytest + httpx
  validate the FastAPI routers.

---

## Directory Structure

- `frontend/` — Next.js 16 (App Router) + React 19 + shadcn/ui + Tailwind CSS v4 (TypeScript)
  - `app/` — App Router pages, layouts, and API routes
  - `components/` — React components including shadcn/ui
  - `lib/` — Utility functions
- `backend/` — FastAPI + Uvicorn (Python 3.11)

---

## Developer Setup

### Prerequisites
- Node.js 20+ (in your PATH)
- Python 3.11+ (in your PATH)
- pnpm (in your PATH)

### 1. Frontend Setup
```bash
cd frontend
pnpm install

# (optional) Add more shadcn components:
pnpm dlx shadcn@latest add [component-name]

pnpm dev
```

Visit `http://localhost:3000` to see the Server Action-powered greeting page.

Server Actions call `FastAPI` via the shared `backendJson` helper. If you still
need a browser-accessible endpoint, the `/app/api/*` routes proxy requests with
response validation before returning data to the client.

### 2. Backend Setup
```bash
cd backend
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt

# For development (linting, testing):
pip install -r requirements-dev.txt
```

**Start the server:**
```bash
uvicorn main:app --reload
```

Visit `http://localhost:8000` to see the FastAPI Hello World endpoint, and try
the example query endpoint:

- `http://localhost:8000/hello?name=YourName` => { "message": "Hello, YourName from FastAPI!" }

### 3. Environment Variables

Copy `.env.example` to `.env` and customize as needed:
```bash
cp .env.example .env
```

The app will work with defaults, but you can customize:
- `FRONTEND_PORT` - Frontend dev server port (default: 3000)
- `BACKEND_PORT` - Backend server port (default: 8000)
- `BACKEND_URL` - Full backend URL (optional, overrides BACKEND_PORT)
- `CORS_ORIGINS` - Comma/semicolon-separated CORS origins. Defaults to
  `http://localhost:3000`. Wildcards are rejected automatically when
  `CORS_ALLOW_CREDENTIALS=true`.
- `CORS_ALLOW_CREDENTIALS` - Whether credentials/cookies are allowed. Defaults
  to `false` for safety.
- `LOG_LEVEL` - Backend logging level (default `INFO`).

### 4. Linting, Formatting & Tests

To ensure code quality, run the following commands in the `frontend/` directory:

- **Linting**: `pnpm lint` (Checks for errors)
- **Formatting**: `pnpm format` (Fixes formatting issues)
- **Unit tests**: `pnpm test`

In the `backend/` directory run:

- **Unit tests**: `pytest`

The `run-dev.sh` script automatically runs linting **and** the relevant unit
tests (frontend Vitest + backend pytest) before starting the development
servers. If any check fails, the script reruns it with full output and aborts
the startup.

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

You can run both the frontend and backend dev servers together with `run-dev.sh`
at the repository root. The script starts both servers, writes logs to
`./logs/`, and stops both cleanly if it is interrupted (SIGINT/SIGTERM).

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
