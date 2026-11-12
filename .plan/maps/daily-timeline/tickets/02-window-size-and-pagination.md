---
type: grilling
blocked_by: []
undermined_by: [06]
---

# Window size and how older days load

## Question

How many days is one window, and how does the next window back arrive?

The bounds are settled: anchor at today, walk backwards, no future days. What is not
settled is `N` and the loading mechanism. `DailyGroups(ctx, 100)`'s expense cap
cannot survive — 100 expenses is an unbounded number of days, and a window of days is
an unbounded number of expenses.

Resolve, one at a time:

- **N.** 30 days? A calendar month? Enough to fill a viewport and no more? Note that
  N is now the page-weight knob: every day in the window costs a card whether or not
  it has an expense.
- **Mechanism.** A `?before=YYYY-MM-DD` query param and a plain "Load older" link at
  the foot (server-rendered, no JS, matches the standing preference)? Or an infinite
  scroll island? The map rules out islands for *collapsing*; that ruling does not
  automatically extend to pagination — but it sets the burden of proof.
- **Append or replace.** Does "Load older" navigate to a new page showing the older
  window, or append to the current one? The former is trivial and stateless; the
  latter needs an island or `hx-`-style swap.
- **Does the current 100-expense limit have a defender?** Check whether anything else
  depends on `DailyGroups`' signature before changing it.

This ticket blocks the query shape, so it also owes an answer to: what does the
handler need to ask the repository for — a window of expenses, or a window of days?

## Answer

**Was undermined by [Redesign the day entry as a ledger row](./08-day-entry-ledger-redesign.md);
re-affirmed by [The infinite-scroll island's contract](./07-scroll-island-contract.md).**
The island was chosen when every expense on this page was already client-rendered by
`data-table`, so an island cost nothing that was not already spent. Ticket 08 drops
`data-table` from the daily view, and the page now renders expenses server-side and works
without JS — which destroyed that premise. Ticket 07 re-opened the mechanism and put it to
the user with the plain-link alternative priced (it would have deleted the island, the
endpoint, and ticket 06 outright). The user chose infinite scroll again, deliberately, on
scroll ergonomics rather than on a JS cost already sunk. The decision below now stands on
that choice, so the `undermined_by` marker is discharged rather than carried.

**Window is a rolling 30 days from today**: `[today-29, today]`. Always 30 cards, today
always first, no ragged first window. Month-alignment was rejected because on the 1st
of a month the first window would be one day tall.

**Older days arrive via a new infinite-scroll React island.** This overrides the map's
standing "server-render unless forced" preference — recorded as a deliberate choice by
the user, not an oversight. Plain-link options (`?days=` growing window, `?before=`
pages) were offered and declined.

~~**The island fetches JSON and renders appended day-cards in React.** A new endpoint
returns `[]domain.DailyGroup` for a date range. Rejected: fetching HTML fragments,
because `ui/src/entries/data-table.tsx` calls `init()` once at module load and
`querySelectorAll`s the document — appended `[data-table-root]` nodes would never
mount, and its `hydrateRoot` is the wrong call for freshly-fetched markup anyway.~~

**Reversed by [Contain the day-card chrome drift](./06-day-card-chrome-drift.md).** The
rejection above rested wholly on `data-table`, which [ticket 08](./08-day-entry-ledger-redesign.md)
removed from this page. An appended ledger day contains no islands at all, so the endpoint
serves **HTML** rendered by the same Go partial as the first window, and the island appends
it. The island is not React. The window size, the island itself, and the termination rule
below all stand.

**The first 30-day window stays server-rendered** by `daily.html`; the island only
appends windows below it. This keeps first paint fast and keeps muted empty-day cards
working without JS. The price is that day-card chrome (header, date, footer total)
is implemented twice — Go template and TSX. Accepted, and contained by ticket 06.

**Scrolling terminates at the earliest expense's day.** `svc.GetEarliestExpenseDate()`
already exists and is already used by `HandleCalendar`. Once a window reaches it, the
island stops requesting and the foot shows a terminal marker. No infinite scroll into
pre-history.

### Facts established (looked up, not asked)

- **`DailyGroups` has exactly one caller** — `handlers_html.go:264`, where `100` is a
  bare literal. Nothing defends the current signature. Ticket 04 may change it freely.
- **The repository needs no new SQL.** `ListExpensesInRange(ctx, start, end)`
  (`sqlite.go:69`) takes a date range with no `LIMIT`, and `HandleCalendar` already
  uses it for exactly this purpose. This also settles the map's "Query cost" fog: one
  `ExpensesInRange` per 30-day window is enough; no `GROUP BY date` aggregate needed.
- **The daily view's tables are already client-rendered React, not hydrated server
  markup.** `templates/partials/data-table.html` emits only `<div data-table-root>`
  with a JSON `<script>` and a `<noscript>` fallback — there is no server-rendered
  `<table>`. So this page already requires JS to display any expense, and the
  Go-template/TSX drift introduced above touches only day-card chrome, never tables.
- The app has **no pagination anywhere** today; this is the first.
- Gap-filling adds only static cards — empty days carry no `data-table` island — so
  window size drives markup weight, not hydration cost.
