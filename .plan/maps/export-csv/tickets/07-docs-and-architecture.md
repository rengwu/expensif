---
type: grilling
blocked_by: [04, 05, 09]
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

## Answer

**Export sits inside the recorded architecture, not beside it: ADR 0001 covers the preview
island as-is (its daily-timeline note already generalized the rule), and the vocabulary is two
user-facing terms. No new ADR, no `AGENTS.md` change.** This ticket *decides and drafts* the doc
text; the `export-csv-impl` map writes it, exactly as `daily-timeline`'s docs were written in
`daily-timeline-impl` (commit `e6f53ac`, "architecture docs catch up with what shipped"), never
in the planning map.

### ADR — covered by 0001, amend the existing note (no new ADR)

ADR 0001 was **amended with a "Note (daily-timeline effort)"** when the *first* vanilla-TS island
(`daily-scroll`) appeared, and that note already states the general rule: *"React owns the islands
that carry client-side state or markup; an island that only moves server-rendered HTML into the
page does not need it."* Ticket 09's `export-preview` island is precisely that pattern — vanilla
TS, `fetch` → `el.innerHTML`, no state, no markup — so it is **already inside ADR 0001's rule**. A
new ADR was rejected: this is a *second instance* of a pattern 0001 owns, not an architectural
departure, and a fresh ADR would split the "vanilla vs React island" rule across two documents.

The one drift to fix is that the note calls daily-scroll *"the first island that is not React"* —
true when written, stale once there are two. The impl session amends the note to name both and to
mark what differs (the preview is **input-driven**, watching filters, where daily-scroll is
**scroll-triggered**), keeping the shared rule. **This is where the effort's reversal is recorded
as *decided*, not assumed:** the map's standing "a form + link needs no island" preference
(Notes) was reversed in [ticket 05](./05-export-page-ui.md) with the user's explicit acceptance —
the preview earns an island because *live reassurance needs a fetch*, and it stays **vanilla**
because it only moves server-rendered HTML. The ADR records the architectural outcome; the
argument lives in 05. Drafted amendment:

```md
## Note (daily-timeline effort)

The Daily View's infinite-scroll island is the first island that is **not** React: ~30 lines of
vanilla TypeScript that fetch a server-rendered HTML fragment from `GET /daily/older` and append
it — no components, no JSX, no hydration. ...
<!-- existing paragraph stands; then extend its closing point: -->

The Export page's live-preview island (`GET /export/preview`) is the **second** such island and
confirms the pattern is not a one-off: it too is vanilla TS moving a server-rendered fragment into
the page (`fetch` → `innerHTML`), so it holds no state or markup and skips React for the same
reason. It differs only in trigger — it is **input-driven**, re-fetching as the export filters
change, where daily-scroll is scroll-triggered. "React owns the islands" therefore reads: React
owns the islands that carry client-side *state or markup*; an island that only moves
server-rendered HTML into the page does not — whether a scroll appends it or a filter change
replaces it. (The Export page also reverses this effort's own "a form + link needs no island"
default: the live preview earns an island because live reassurance needs a fetch, and stays
vanilla because it moves server HTML — see the export-csv plan, ticket 05.)
```

### CONTEXT.md — two user-facing terms; code seam names ride parenthetically

CONTEXT.md is the **domain glossary** — plain, user-facing terms with an `_Avoid_` line each. So
Export lands as two domain entries, and the code names tickets 01/09 minted (`ExpenseFilter`,
`ExpensesMatching`, `CountMatching`) ride *inside* the Export entry the way **Payer** notes
`paidBy`/`paidById` — not as their own glossary entries, which would seed a user-facing glossary
with Go method names in a register no existing entry uses. Drafted additions to `## Language`:

```md
**Export**:
Downloading the account's expenses as a CSV file, filtered by a date range plus an optional
category and payer. Unlike the List View, whose filtering is client-side, Export filters on the
server (the `ExpensesMatching` query, with a `CountMatching` sibling for the live count). One
canonical US/ISO CSV: ten fixed columns, dot decimals, comma-separated, UTF-8 with a BOM.
_Avoid_: report, dump, backup, download (as a noun for the feature)

**Export Preview**:
The live row count and a raw sample of the first rows shown on the Export page as the filters
change, so the size and contents are visible before committing to a download. The sample shows
each expense's entered amount and currency, not the converted amount — conversion happens only in
the downloaded file.
_Avoid_: preview pane, dry run
```

Two existing entries get a light, consistency-only touch (same class of fix as the ADR note):

- **Island** currently says the daily-scroll is *"a vanilla-TypeScript island"* as if it were the
  only one. Generalize to *"…the Daily View's infinite scroll and the Export page's live preview
  are vanilla-TypeScript islands that fetch and swap server-rendered HTML (see ADR 0001)."*
- **List View** may gain a half-line noting its filtering is client-side, *in contrast to* Export
  — optional, only if it reads naturally; the contrast already lives in the Export entry, so this
  is not required.

### AGENTS.md — no change

`AGENTS.md`'s Layout section documents the **Go package structure** (`internal/web`, `internal/service`,
…), which Export does not alter — it adds handlers and templates *within* `internal/web`, not a new
package. The user-facing pages (Daily, Calendar, Expenses) are **not** enumerated as routes there,
so Export's three routes do not belong in it either; they are documented in the spec. Decided: no
`AGENTS.md` edit. (If the `-impl` map adds a genuinely new package — unlikely — it revisits this.)

### What the impl session writes (the checklist this ticket hands off)

1. Amend `docs/adr/0001-react-islands-architecture.md`'s note per the draft above — **no new ADR**.
2. Add **Export** and **Export Preview** to `CONTEXT.md` `## Language`; generalize the **Island**
   entry; optionally touch **List View**.
3. Leave `AGENTS.md` unchanged.

**Nothing here undermines another ticket** — it records decisions already made (01's seam, 05's
island reversal, 09's contract) against the shipped docs, and mints no new fact those tickets rest
on. This was the last architectural fact the map owed; with it decided, every ticket on the map is
resolved.
