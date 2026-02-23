````skill
---
name: code-reviewer
description: Review implementations in this Next.js + FastAPI project against spec acceptance tests. Provides the review checklist, test verification, lint checks, quality standards, and verdict format. Use when reviewing implemented code, verifying tests, or performing a code review after implementation.
---

# Code Reviewer Skill

## Prerequisites

Before starting any work, read and follow the agent-conduct skill
(`.github/skills/agent-conduct/SKILL.md`). It covers workspace
boundaries, scratch work, terminal safety, and git safety rules
that apply to all agents.

---

You are a review subagent with clean context - no memory of
implementation decisions. Your job is to independently verify that
implemented code meets the specification and quality standards.

## Review Procedure

For each item under review:

### 1. Read the specification

- Read spec.md for the referenced sections (acceptance tests,
  API designs, component specs, architecture decisions).
- Read all implemented source and test files for the item(s).

### 2. Run the tests

Backend:
```
cd backend && python -m pytest tests/ -v
```

Frontend:
```
cd frontend && pnpm test
```

- Run tests for every area that was modified.
- Confirm all tests pass.

### 3. Verify acceptance test coverage

- For every acceptance test listed in spec.md for the referenced
  user stories, confirm there is a corresponding test (pytest
  for backend, Vitest for frontend).
- Do not accept missing, stubbed-out, or circumvented tests.
- Do not accept hardcoded expected results in implementations
  that make tests pass artificially.
- Do not accept test helpers that silently swallow failures.

### 4. Verify implementation correctness

#### Architecture (BFF pattern)

- Confirm the browser NEVER calls FastAPI directly. All backend
  communication must flow through Server Actions or API Routes.
- Confirm Server Actions are in files with `'use server'` at the
  top.
- Confirm client components use `'use client'` and do NOT import
  server-only modules.
- Confirm API Routes (`app/api/*/route.ts`) are only used for
  external consumers (health checks, webhooks), not for frontend
  data fetching.

#### Contract integrity

- Confirm every new endpoint has BOTH a Pydantic response model
  (`backend/api/schemas.py`) AND a matching Zod schema
  (`frontend/lib/contracts.ts`).
- Confirm `backendJson()` is used with the Zod schema for all
  backend calls.
- Confirm contract tests exist in
  `frontend/tests/contracts.test.ts` for new schemas.
- Confirm Pydantic and Zod schemas agree on field names, types,
  and constraints.

#### Backend correctness

- Confirm endpoints use `async def`, declare `response_model`,
  and return Pydantic model instances.
- Confirm the lifespan pattern is used (not deprecated
  `@app.on_event`).
- Confirm Pydantic Settings is used for configuration (not
  raw `os.environ`).
- Confirm routers are properly mounted under `api/v1/`.

#### Frontend correctness

- Confirm `useActionState` is used (not deprecated
  `useFormState`) for form handling.
- Confirm Server Components fetch data on the server and pass
  props to Client Components.
- Confirm state types are explicitly defined for Server Action
  return values.
- Confirm Tailwind CSS v4 semantic tokens are used (not raw
  colour values).
- Confirm shadcn/ui components are used where appropriate.

### 5. Verify code quality

#### Python (backend)

- **Modern Python 3.11+:** Type hints everywhere, `Annotated`
  for FastAPI parameters, async handlers, proper imports.
- **Style:** 88-col line width (ruff), docstrings on all modules
  and public functions, snake_case naming, PascalCase for classes.
- **Import grouping:** stdlib, third-party, local - separated by
  blank lines.
- **Error handling:** No swallowed errors, proper use of
  `HTTPException`, Pydantic validation for inputs.
- **Testing:** pytest + httpx AsyncClient with ASGITransport,
  `@pytest.mark.anyio`, independent tests, status code AND
  payload assertions.

#### TypeScript (frontend)

- **Strict TypeScript:** No `any` types, proper type narrowing,
  `z.infer<>` for Zod-derived types.
- **Import grouping:** React/Next.js, third-party, local `@/`
  imports - separated by blank lines.
- **Component patterns:** `'use client'` only where needed,
  Server Components for data fetching, proper prop typing.
- **Tailwind v4:** Semantic tokens from `@theme`, dark mode
  support, responsive design with mobile-first breakpoints.
- **Testing:** Vitest with `describe`/`it`, `.parse()` and
  `.safeParse()` for contract tests, clear assertions.

### 6. Run the linters

Backend:
```
cd backend && ruff check . && ruff format --check .
```

Frontend:
```
cd frontend && pnpm lint
```

- Confirm no issues are reported for modified files.
- If issues are found, report them in the verdict.

### 7. Return verdict

Return one of:

- **PASS** - Optionally note minor suggestions that do not block
  approval.
- **FAIL** - Provide specific, actionable feedback listing:
  - Which acceptance tests are missing or incorrect.
  - Which spec requirements are not met.
  - Which architecture violations were found (BFF, contracts).
  - Which quality violations were found.
  - Which lint issues remain.

## Review Scope per Phase Type

### Single-item phases

Review the one item's source and test files.

### Parallel batch phases

Review ALL items in the batch together in a single review pass.
Return a per-item verdict (PASS or FAIL with specific feedback
for each).

````
