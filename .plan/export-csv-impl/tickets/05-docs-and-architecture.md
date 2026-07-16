---
type: task
blocked_by: [02, 04]
---

# Docs and architecture catch up with what shipped

## Question

Deliver the spec's **docs and architecture** obligations (07), now that the routes, the
conversion machinery, and the island all exist. The draft text lives in the planning map's
[ticket 07](../../export-csv/tickets/07-docs-and-architecture.md) — this ticket writes it into
the shipped docs, exactly as `daily-timeline`'s docs landed in `daily-timeline-impl`. Blocked by
02 and 04 — the final download behaviour and the island are the facts being recorded.

Build (docs only — no code changes, so the suite should stay green untouched):

- **Amend `docs/adr/0001-react-islands-architecture.md`'s existing "Note (daily-timeline
  effort)"** — **no new ADR**. Name the `export-preview` island as the **second** vanilla-TS
  island (fixing the stale "the first island that is not React"), mark that it is
  **input-driven** where daily-scroll is **scroll-triggered**, and keep the shared rule: React
  owns the islands that carry client-side *state or markup*; an island that only moves
  server-rendered HTML into the page does not. Record the effort's "a form + link needs no
  island" reversal here as *decided* (argued in planning ticket 05), not assumed. Use ticket
  07's drafted amendment.
- **`CONTEXT.md`** — add **Export** and **Export Preview** to `## Language` as two user-facing
  terms, with the code seam names (`ExpensesMatching`, `CountMatching`) riding *inside* the
  Export entry the way **Payer** notes `paidBy` — not as their own glossary entries. Generalize
  the **Island** entry to name both vanilla islands. Optionally touch **List View** to note its
  client-side filtering contrasts with Export, only if it reads naturally. Use ticket 07's
  drafted text; apply `domain-modeling`.
- **`AGENTS.md`** — **no change** (Export adds handlers/templates within `internal/web`, not a
  new package; the user-facing pages aren't enumerated as routes there).
- **Leftover sweep** — remove any prototype throwaways, dead template branches, or stray island
  registrations the earlier tickets left behind (as daily-timeline's docs ticket did); report if
  the sweep finds nothing.

Testing: docs only, so no new tests; run `go test ./...` / `go vet ./...` to confirm nothing
regressed, and `cd ui && npx tsc --noEmit` if any TS was touched by the sweep.

Done when: a fresh agent reading `AGENTS.md`, `CONTEXT.md`, and the ADRs gets an accurate picture
of the shipped Export feature — its three routes, its second vanilla island, and the vocabulary —
and `git grep` finds no leftover prototype scaffolding.
