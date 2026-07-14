---
type: grilling
blocked_by: [04, 05]
---

# Docs and architecture catch-up

## Question

Reconcile the effort with the project's recorded architecture and vocabulary. Waits on 04
and 05 because the route and the page shape are the architectural facts to record.

- **ADR check.** Read `docs/adr/` — ADR 0001 records the Islands decision. Confirm a
  server-rendered Export page with a plain form needs **no new ADR and no island**, and if
  05 surfaced any island temptation, decide whether it reverses the preference or is
  resisted (as the daily-timeline scroll island was argued through, not assumed).
- **`CONTEXT.md` vocabulary.** Add **Export** as a term, and any subsidiary terms the
  effort minted (e.g. the filter model from 01, a "row" convention from 03). Use the
  `domain-modeling` skill.
- **Nav / feature surface.** Note the new page in whatever guide lists the app's surfaces
  (`AGENTS.md`'s layout section) if it belongs there.

Docs only; this ticket decides *what the spec instructs the impl session to write*, it
does not itself edit shipped code.
