---
type: task
blocked_by: [04, 05, 06]
claimed_by: sd4c2f970daae
claimed_at: 2026-08-01T19:27:48Z
---

# Docs and architecture catch up with what shipped

## Question

The map's last ticket. Make the documentation describe what was actually built, then leave the
repo in a state where a fresh agent gets an accurate picture without reading this map.

Do:

- **`CONTEXT.md`** gains the vocabulary this effort introduced: **Recurring Rule**, **Pending
  Charge**, **Anchor Day**, **Catch-up**, and **Monthly Equivalent**. Follow the file's existing
  conventions — a plain user-facing definition, an `_Avoid_` line where a tempting wrong synonym
  exists — and place them near the views they relate to. Two are worth stating carefully because
  they are the effort's load-bearing ideas: a **Pending Charge** is an ordinary future-dated
  Expense and *not* a distinct type or state, and **Catch-up** covers time the app was absent
  and never a stretch a human deliberately paused. Related terms can ride inside an entry rather
  than earning their own, the way **Payer** carries `paidBy`.

- **Assess whether an ADR is warranted, and be willing to conclude it isn't.** This effort adds
  no Island and introduces no new architectural pattern — the generation goroutine deliberately
  mirrors the existing rate fetcher rather than inventing anything. The likely right outcome is
  no new ADR, possibly a note on an existing one. Write a new ADR only if something genuinely
  architectural was decided that a future reader would otherwise re-litigate; say plainly in the
  answer which way you went and why.

- **`AGENTS.md`**'s `.plan/` inventory was updated when this map was charted, so it should
  already name this effort — verify it still describes it accurately, and check nothing else in
  that file drifted. Do not restate how far along the map is or how many tickets are resolved —
  that is derived from the tickets, and a count written down is true for a week and wrong
  forever.

- **Sweep for drift.** Look for comments, templates or ticket-era TODOs that describe shipped
  work as pending ("until generation ships", "ticket 02 supplies…"), and rewrite them in the
  present tense. Check for dead code, unused template partials, and anything the spec described
  that did not survive contact with implementation.

- **Flag rather than fix** anything you find that contradicts the spec or looks like a decision
  quietly deviated from during implementation. This map has no planning tickets to mark
  undermined, so a contradiction comes back to the human — name it in the answer.

Docs-only work still runs the checks: `go vet ./...` and `go test ./...` must be green, and if a
template was touched, load the affected pages to prove they still parse — the Go server caches
templates at startup, so restart before checking.

**Done when:** `CONTEXT.md` carries the new vocabulary, the ADR question is decided either way
with reasoning, `AGENTS.md` describes this effort without stating progress, the drift sweep is
done, and anything contradicting the spec is flagged for the human rather than silently
corrected.
