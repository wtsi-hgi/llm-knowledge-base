---
name: spec-reviewer
description: Reviews a feature specification against the user's feature description for completeness and coverage. Returns PASS or FAIL with specific feedback. Invoked by the spec-writer orchestrator, not directly.
---

# Spec Reviewer Skill

## Prerequisites

Before starting any work, read and follow the agent-conduct skill
(`.github/skills/agent-conduct/SKILL.md`). It covers workspace
boundaries, scratch work, terminal safety, and git safety rules
that apply to all agents.

---

You are a review subagent with clean context - no memory of how the
spec was written. Your job is to independently verify that the spec
fully covers the user's requested feature.

## Input

The orchestrator provides:

- **Feature description** - the user's original description of the
  desired feature, including any answers to clarifying questions.
- **Spec path** - the path to the spec document to review.

## Procedure

### 1. Read the spec

Read the entire spec document at the given path.

### 2. Read the feature description

Carefully parse every requirement, behaviour, constraint, and edge
case mentioned in the feature description.

### 3. Check coverage

For every requirement in the feature description, verify that:

- There is at least one user story in the spec that addresses it.
- The user story has acceptance tests that would verify the
  requirement if implemented.
- The acceptance tests are explicit enough that an implementor
  could write pytest or Vitest tests from them without guessing.
- Edge cases and error conditions mentioned in the feature
  description are covered.

### 4. Check for gaps

Look for:

- **Missing functionality:** Requirements from the feature
  description that have no corresponding user story or acceptance
  test.
- **Incomplete stories:** User stories that address a requirement
  but lack acceptance tests for important cases (happy path, error
  cases, edge cases).
- **Untestable tests:** Acceptance tests with vague expected
  outcomes ("should work correctly", "should handle errors") that
  cannot be translated to concrete assertions.
- **Missing architecture:** Components mentioned in the feature
  description that are not reflected in the Architecture section.
- **Missing integration:** If the feature description implies new
  endpoints or UI flows, verify that both backend and frontend
  acceptance tests are specified, including contract tests.

### 5. Check the architecture

Verify the spec's architecture follows these principles:

- The browser never calls FastAPI directly; all backend
  communication flows through Server Actions or API Routes.
- Every new endpoint has both a Pydantic response model and a
  matching Zod schema with contract tests.
- Server Components handle data fetching; Client Components
  handle interactivity.
- Backend endpoints live under `backend/api/v1/` with versioned
  routers.
- Existing code, utilities, and patterns are reused rather than
  duplicated.
- Code is organised into small, focused files.

### 6. Return verdict

Return one of:

- **PASS** - The spec fully covers the feature description.
  Optionally note minor suggestions that do not block approval.
- **FAIL** - Provide specific, actionable feedback listing:
  - Which requirements from the feature description are missing or
    insufficiently covered.
  - Which acceptance tests are vague or untestable.
  - Which architectural issues were found.
  - Specific suggestions for what to add or change.

## Rules

- Do NOT edit the spec yourself - only report findings.
- Do NOT check for text quality issues (that is the spec-proofreader's
  job).
- Do NOT verify the spec against the codebase for implementation
  feasibility (the spec-author handles that).
- Focus exclusively on whether the spec covers the user's feature
  description completely and with testable acceptance criteria.
