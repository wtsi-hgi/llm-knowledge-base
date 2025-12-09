# Next.js + shadcn/ui → FastAPI Monorepo

This project is a full-stack app with a Next.js (React) frontend using the
**App Router**, Tailwind CSS v4 + shadcn/ui, and a FastAPI backend.

### Highlights

- **Server Actions first**: Next.js Server Actions call FastAPI directly from
  the server without exposing backend URLs to the browser.
- **Typed contracts**: FastAPI responses are validated on the frontend with
  `zod` so regressions are caught immediately.
- **Modern styling**: Tailwind v4 `@theme` tokens power shadcn/ui components,
  theme switching, and animation primitives.
- **Tests built-in**: Vitest covers the shared contracts, while pytest + httpx
  validate the FastAPI routers.

If you are new to this stack, think of it as:

- Next.js handles **routing, rendering, and UI**.
- FastAPI exposes a **typed JSON API**.
- Zod bridges the two by validating **everything that crosses the boundary**.

---

## Directory Structure

-- `frontend/` — Next.js 16 (App Router) + React 19 + shadcn/ui + Tailwind CSS v4 (TypeScript)
  - `app/` — App Router pages, layouts, and API routes
    - `app/page.tsx` — Home page that calls Server Actions
    - `app/actions.ts` — Server Actions that talk to FastAPI
    - `app/api/health/route.ts` — Health check proxy for external monitors
  - `components/` — React components including shadcn/ui
    - `components/hello-form.tsx` — Client component with a form that calls a Server Action
    - `components/health-status.tsx` — Small status indicator for backend health
    - `components/ui/` — shadcn/ui primitives (button, card, input, etc.)
  - `lib/` — Shared frontend utilities and contracts
    - `lib/backend-client.ts` — Thin `fetch` wrapper used by Server Actions
    - `lib/contracts.ts` — Zod schemas mirroring FastAPI responses
    - `lib/greeting-state.ts` — State machine for the greeting form
-- `backend/` — FastAPI + Uvicorn (Python 3.11)
  - `main.py` — FastAPI app with lifespan + logging
  - `config.py` — Pydantic settings (ports, URLs, log level, etc.)
  - `api/` — Versioned routers and schemas
    - `api/v1/greetings.py` — `GET /api/v1/hello` greeting endpoint
    - `api/v1/health.py` — `GET /api/v1/health` health endpoint
    - `api/schemas.py` — Pydantic models shared across routers
  - `tests/` — pytest suite using `httpx.AsyncClient`

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

### How the pieces fit together

At a high level, a greeting request flows like this:

1. User types a name into `HelloForm` (`components/hello-form.tsx`) and submits.
2. The form calls the `requestGreeting` Server Action in `app/actions.ts`.
3. `requestGreeting` calls `backendJson` from `lib/backend-client.ts` with the
  FastAPI URL and the relevant Zod schema from `lib/contracts.ts`.
4. FastAPI serves the request from `backend/api/v1/greetings.py` and returns
  a JSON payload.
5. `backendJson` validates the JSON with Zod and returns a typed object to the
  Server Action, which updates the greeting state.

The health check uses a slightly different path:

1. `Home` (`app/page.tsx`) calls the `fetchHealth` Server Action.
2. `fetchHealth` calls the FastAPI `/api/v1/health` endpoint using `backendJson`.
3. The `/app/api/health/route.ts` API Route exists **only** for external
  monitors that need a simple unauthenticated `GET /api/health` endpoint.

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

To update dependencies:

```bash
cd frontend && pnpm update --latest

cd ..
source backend/.venv/bin/activate && pip install --upgrade pip && pip install --upgrade -r backend/requirements.txt -r backend/requirements-dev.txt
```

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

## Health Checks & Monitoring

### External Monitoring
External monitoring services (e.g., AWS ALB, UptimeRobot) should check the
frontend's health endpoint:

- **URL**: `GET /api/health`
- **Success**: `200 OK` `{"status": "healthy"}`
- **Failure**: `503 Service Unavailable` `{"status": "unhealthy"}`

### Why a Proxy Route?
While most of the application uses **Server Actions** to communicate with the
backend, the health check requires a dedicated API Route
(`/app/api/health/route.ts`) for specific reasons:

1.  **Protocol Compatibility**: Server Actions use a specialized POST protocol
    internal to Next.js. Standard load balancers and monitoring tools expect a
    simple HTTP `GET` request returning a 200 OK status code.
2.  **Network Isolation**: In production, the FastAPI backend is often deployed
    in a private network, inaccessible to the public internet. The frontend acts
    as a gateway.
3.  **Status Codes**: The proxy route explicitly translates backend connectivity
    issues into standard HTTP 503 status codes, which automated monitors rely on
    to detect failures.

**Note**: Other application features do **not** use proxy routes. They use
Server Actions to communicate directly from the Next.js server to the FastAPI
backend. This keeps the backend API private, reduces the public attack surface,
and maintains type safety without manually defining API routes for every
feature.

---

## Notes
- Each developer should create their own `.venv` in `backend/` and install
  dependencies there.
- Do not share virtual environments between users.
- The `pnpm` binary can be shared, but each user’s global packages/cache are
  separate by default.
