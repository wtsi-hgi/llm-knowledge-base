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
pnpm install
# (Optional) Install shadcn/ui:
pnpm dlx shadcn-ui@latest init
# Add a component, e.g.:
pnpm dlx shadcn-ui@latest add button
pnpm dev
```
Visit http://localhost:3000 to see the Hello World page.

### 2. Backend Setup
```bash
cd backend
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
uvicorn main:app --reload
```
Visit http://localhost:8000 to see the FastAPI Hello World endpoint.

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
- For shadcn/ui usage, see https://ui.shadcn.com/docs/installation
