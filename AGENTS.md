# Expensif — Agent Guide

Go expense tracker: server-rendered HTML with Islands (partial hydration — mostly React, plus one vanilla-TS island; see ADR 0001), SQLite, single binary. Deployed via Docker.

## Commands

| Task | Command |
|---|---|
| Install UI deps | `make dev-install` |
| Dev (Go + Vite, hot reload) | `make dev` |
| Production build | `make prod` |
| Go tests | `go test ./...` |
| UI tests | `cd ui && npx vitest run` |
| Go static check | `go vet ./...` |
| TypeScript check | `cd ui && npx tsc --noEmit` |

There is no `test` script in `ui/package.json`; vitest is installed and invoked via `npx`.

## Layout

- `cmd/server/` — entrypoint.
- `internal/domain/` — core types. `internal/service/` — business logic. `internal/repository/` — SQL. `internal/web/` — HTTP handlers and templates. `internal/rate/` — exchange rates. `internal/db/`, `internal/assets/`.
- `ui/src/` — React islands, built by Vite into `/static/` (generated, gitignored).
- `templates/` — Go HTML templates that mount the islands.

Tests live beside the code they cover (`internal/web/handlers_api_test.go`, `ui/src/components/DataTable/DataTable.test.tsx`).

## Planning memory — `.plan/`

`.plan/` is committed to version control. It is the project's shared planning memory across agent sessions.

- `.plan/<slug>/spec.md` — specs (PRDs) for an effort.
- `.plan/<slug>/tickets.md` — ticket breakdowns.
- `.plan/<slug>/map.md` + `.plan/<slug>/tickets/NN-<slug>.md` — wayfinder maps for large efforts.
- `.plan/handoffs/<YYYY-MM-DD>-<slug>.md` — session handoffs, so a fresh agent can pick up work.

Current contents: `.plan/expensif/` (project spec, brief, testing strategy), `.plan/react-islands/spec.md` (migration spec), `.plan/daily-timeline/` + `.plan/daily-timeline-impl/` (daily view redesign), `.plan/export-csv/` + `.plan/export-csv-impl/` (CSV export), `.plan/recurring-expenses/` + `.plan/recurring-expenses-impl/` (recurring expenses), `.plan/handoffs/` (session history). An effort is planned on a `<slug>` map — or, where a grilling session settles every decision outright, a `<slug>/spec.md` with no map at all — and executed on a matching `<slug>-impl` one.

How far along a map is, and which ticket is ready to claim, is derived from its tickets — never restated here, where it would drift. Read the map's `map.md` and scan its `tickets/`.

Read the most recent handoffs to catch up on project state. The `session-*.md` files there predate this naming convention and are kept as a historical archive — do not rewrite them; write new handoffs using the dated convention above.

## Domain model

- `CONTEXT.md` (repo root) — domain glossary / ubiquitous language.
- `docs/adr/` — architecture decision records.

Both exist and are maintained by the `domain-modeling` skill. Use `CONTEXT.md`'s vocabulary in specs and code; check `docs/adr/` before changing architecture it records.

## Skills

Reusable agent capabilities live in `.pocock-skills/<name>/SKILL.md`, following the [Agent Skills](https://agentskills.io) open standard. They are adapted from [Matt Pocock's skills](https://github.com/mattpocock/skills) (MIT — see `.pocock-skills/LICENSE`), rewritten to be standalone, language-agnostic, and harness-agnostic.

**No agent auto-discovers these — the directory is deliberately outside any tool-specific skills path.** To use one, read `.pocock-skills/<name>/SKILL.md` and follow it. Each skill states its own conventions inline; there is no setup step.

When the user asks for work matching a skill below, read that skill's `SKILL.md` before starting.

**User-invoked** — these orchestrate a session, and only run when asked for by name:

| Skill | Use it to |
|---|---|
| `grill-me` | Stress-test a plan or design with relentless one-at-a-time questions |
| `grill-with-docs` | Same, but update `CONTEXT.md` and ADRs as decisions crystallise |
| `to-spec` | Synthesize the conversation into a spec at `.plan/<slug>/spec.md` |
| `to-tickets` | Break a plan or spec into tracer-bullet tickets |
| `wayfinder` | Chart a big, foggy effort as a map of investigation tickets |
| `handoff` | Compact the conversation into `.plan/handoffs/` |
| `improve-codebase-architecture` | Scan for deepening opportunities, report, then grill through one |
| `writing-great-skills` | Reference for writing and editing skills |

**Model-invoked** — an agent may reach for these on its own:

| Skill | Use it to |
|---|---|
| `codebase-design` | Vocabulary and principles for designing deep modules |
| `domain-modeling` | Build and sharpen `CONTEXT.md` and `docs/adr/` |
| `prototype` | Throwaway code that answers a design question |
| `research` | Investigate against primary sources, capture cited findings |
| `review-code` | Two-axis diff review: Standards and Spec |

Where a skill benefits from subagents it degrades gracefully: run parallel subagents if the environment supports them, otherwise run the same work as sequential passes.

## Conventions

- Conventional commits (`feat:`, `fix:`, `chore:`). Commit `.plan/` alongside code.
- `.iudex/` is gitignored operational state for the ticket pipeline; `.plan/` is tracked documentation.
