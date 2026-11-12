---
type: grilling
blocked_by: [04]
---

# Should HandleDaily's two branches converge

## Question

`HandleDaily` (`internal/web/handlers_html.go:237`) is two handlers wearing one coat.

The `?date=` branch calls `ExpensesInRange(date, date)`, sums a total by hand, and
hand-builds a one-element `[]domain.DailyGroup`. The unfiltered branch calls
`DailyGroups(ctx, 100)`. They share only the currency-conversion loop that runs after
them both.

Once ticket 04 gives the service a date-windowed call, the filtered branch is a
window of size 1. The duplication becomes obvious and removable — *or* the two
branches turn out to be genuinely different pages that happen to share a template.

Resolve:

- Is `?date=` a window of one day, or a different view? It has a back-link
  (`BackHref`/`BackLabel`, set from `?from=calendar`) that the timeline has no
  concept of, and it is the only current caller of the empty-day markup.
- If it converges: does a single-day window with no expenses still show the muted
  card, or the fuller "No expenses for this day" state it shows today? Two different
  designs for the same zero state is a smell — but landing on the calendar's date
  cell and seeing a *muted* card may read as a broken link.
- If it does not converge, say so plainly in the spec and leave the branch alone.
  Convergence is not a goal; a smaller total surface is.

The answer determines whether ticket 01's design has one consumer or two.

## Answer

**The branches do not converge. `?date=` is a different view, and the spec says so
plainly and leaves the branch alone.** What it shares with the timeline is *rendering*,
not the handler: both draw a populated day through ticket 08's day-entry partial, so the
ledger row has one implementation. Convergence of markup, not of control flow.

### Why not a window of one day

A window of one day would be a *subrange of the timeline*, and half of `?date=`'s domain
lies outside the timeline entirely. The timeline ends at today and paginates back only to
the earliest expense (tickets 02, 07). But the calendar makes **every** cell a link —
`templates/calendar.html:60,91`, `href="/?date={{.Date}}&from=calendar"` — across a range
running a year back to a year forward (`handlers_html.go:144-152`), zero-spend days
included. So a legitimate calendar click lands on days the timeline can never render:

- a **future** zero-spend day — Upcoming is ungapped (ticket 03), so no row exists for it;
- a day **before the earliest expense** — the scroll terminates there (tickets 02, 07).

Feeding those through `DailyGroupsInRange(date, date)` would violate ticket 04's contract,
which is a gap-filled window *ending at today*. `?date=` is a single-day detail view for
any date in ±1 year; the timeline is a bounded rolling window. They answer different
questions and merely happen to share a template today. Convergence is not a goal — a
smaller total *surface* is — and the surface that actually duplicates is the row, which is
already deduplicated by rendering both through the one partial.

The `?date=` branch keeps `ExpensesInRange(date, date)`, its hand-summed total, and its
one-element `[]domain.DailyGroup`. It does **not** call `DailyGroupsInRange`. The
currency-conversion loop (`handlers_html.go:271-285`) stays shared, unchanged: it runs over
`groups` after both branches, and a one-element group falls through it clean.

### What the view renders

Through ticket 08's day-entry partial, keyed on emptiness:

- **A populated day** renders `day-entry-prototype` **verbatim** — date gutter, two rails,
  expense rows, the add-row, total column. The rails become short hairlines and the total
  column sits mostly empty; that is acceptable, because the page title stays "Daily View"
  and the back-link supplies the context a lone row would otherwise lack. No `solo` flag,
  no rail-suppressing conditional — that conditional is exactly the chrome-drift shape
  ticket 06 just deleted, and it is not coming back as a boolean.
- **An empty day** renders the **fuller "No expenses for this day" state**
  (`daily.html:44-50` today), *not* ticket 01's muted ledger line.

### Two zero-state designs, on purpose — not a smell

The ticket calls two designs for one zero state a smell. It is not, and ticket 01's own
rationale is why. Ticket 01 muted the empty day *because* "22 of 30 days are empty — they
are the page's background, not its content." Muting was always a response to context. Under
`?date=` the empty day **is** the content — the entire answer to the click — so it is
foreground, and the same muting that reads as calm background on the timeline reads as a
*broken link* when it is the only thing on the page. That is the precise failure the ticket
worried about, and it is avoided by rendering the state that announces itself. One design in
two contexts is not a duplicate when the contexts invert figure and ground.

So **ticket 01's muted-empty-day design has exactly one consumer: the timeline.** Ticket
08's populated-day design has two: the timeline and `?date=`. That is the answer to this
ticket's closing question.

### Findings, left as-is under "leave the branch alone"

- **The `else` back-link is unreachable from any link in the app.** The calendar always
  appends `&from=calendar`, so `handlers_html.go:250` ("Back to daily view") only fires for a
  hand-typed `/?date=X`. It is harmless defensive code; the spec notes it but does not
  touch it, since this ticket's mandate is to leave the branch alone.
- **A future `?date=` needs no special handling.** Because the view does not gap-fill or
  window, ticket 03's future-date awkwardness never reaches it — it just shows that day's
  expenses or the empty state. Not-converging buys this for free.

### Consequences for other tickets

- **Ticket 10** now has a *third* invoker of the day-entry partial (timeline, `/daily/older`,
  and `?date=`). The cheap "same partial" assertion ticket 06 substituted for its both-paths
  test can name all three; this does not expand 10's surface, only its list of callers.
- **The "Today's row" fog** (map, Not yet specified) gains a consumer: when today is
  distinguished, `?date=today` renders through the same partial and should distinguish it
  too. Recorded there, not resolved here.
