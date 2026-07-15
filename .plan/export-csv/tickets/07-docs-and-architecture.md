---
type: grilling
blocked_by: [04, 05, 09]
claimed_by: claude-session-3ef0a2a8
claimed_at: 2026-07-15T19:54:27Z
---

# Docs and architecture catch-up

## Question

Reconcile the effort with the project's recorded architecture and vocabulary. Waits on 04
and 05 (route + page shape) and 09 (the preview endpoint), which are the architectural facts
to record.

- **ADR check.** Read `docs/adr/` — ADR 0001 records the Islands decision. Note that ticket
  05 **did** surface an island (the live preview) and the user accepted it, **reversing** the
  effort's "a form + link needs no island" preference. Decide whether this island is covered
  by ADR 0001 as-is (partial hydration of one region, like the existing `data-table` /
  scroll islands) or warrants a new/amended ADR, and record the reversal as a *decided*
  reversal — argued through (ticket 05), not assumed away.
- **`CONTEXT.md` vocabulary.** Add **Export** as a term, and any subsidiary terms the
  effort minted (e.g. the filter model from 01, a "row" convention from 03). Use the
  `domain-modeling` skill.
- **Nav / feature surface.** Note the new page in whatever guide lists the app's surfaces
  (`AGENTS.md`'s layout section) if it belongs there.

Docs only; this ticket decides *what the spec instructs the impl session to write*, it
does not itself edit shipped code.
