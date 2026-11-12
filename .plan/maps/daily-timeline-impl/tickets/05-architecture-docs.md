---
type: task
blocked_by: [03]
---

# Architecture docs catch up with what shipped

## Question

Deliver the spec's **Further Notes** doc obligations, now that the code exists:

- Amend ADR 0001 with a note (not a new ADR) that the scroll island is the first
  non-React island — vanilla TS fetching server-rendered HTML — and why that doesn't
  reverse the decision.
- Touch up `AGENTS.md`'s "React Islands" architecture sentence to match.
- Update `CONTEXT.md`: the Daily View definition (date-indexed, every day appears),
  and add the terms the implementation firmed up — candidates from the planning map:
  **Ledger**, **Upcoming**, **Window**, **Rail** — per the `domain-modeling` skill.

Also sweep for leftovers the earlier tickets should have deleted (prototype handlers
or templates, dead template branches, the unused data-table registration on the daily
view) and remove any found.

Done when: a fresh agent reading `AGENTS.md`, `CONTEXT.md`, and the ADRs gets an
accurate picture of the shipped daily view.

## Answer

Docs only — no code changed, so the suite stayed green untouched (`go test ./...`,
`go vet ./...` pass; no TypeScript, so no `tsc`/`vitest`).

- **ADR 0001** gains a "Note (daily-timeline effort)" paragraph: the scroll island is
  the first non-React island (vanilla TS fetching a server-rendered fragment from
  `GET /daily/older` and appending it), and *why* that doesn't reverse the decision —
  Go still owns routing, data, and the document; the fragment renders through the same
  Go partial as the first window, so no view logic moved client-side. The framing that
  changed: "React owns the islands" becomes "React owns the islands that carry
  client-side *state or markup*" — an island that only moves server HTML into the page
  doesn't need it. A note, not a new ADR, as the spec directed.
- **AGENTS.md**'s one-line architecture summary now reads "server-rendered HTML with
  Islands (partial hydration — mostly React, plus one vanilla-TS island; see ADR 0001)"
  instead of the flat "React Islands".
- **CONTEXT.md**: **Daily View** redefined as date-indexed (every day in the Window
  appears, empty days included, rendered as a Ledger). Four terms added — **Ledger**,
  **Rail**, **Window**, **Upcoming** — each tight, with `_Avoid_` lists, cross-linking
  by capitalised reference. **Island**'s definition widened from "React component" to
  "interactive region", noting the vanilla-TS case, since a glossary that still said
  "React" would contradict the ADR note.

**The leftover sweep found nothing to delete.** The prototype throwaways were already
removed when the ledger landed (ticket 02); a grep for `prototype`/`scratch`/dead
markers turned up only the `localPath` comment's "protocol-relative" and a test name.
The daily handler registers `dailyScrollIsland`, not `data-table` — the daily view's
data-table registration is already gone; the two surviving `data.Islands = append(…,
"data-table")` are on the List and Users views, both correct (List is the intentional
bulk-delete surface). The daily template's dead `{{if .DailyGroups}}` branch is already
replaced by `NoExpensesEver`. So earlier tickets cleaned up after themselves; this
ticket only had docs to write.
