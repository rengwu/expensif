---
type: grilling
blocked_by: [01, 08]
---

# The zero-expense-ever empty state

## Question

What does the daily view show for an account that has never recorded an expense?

Today this is unambiguous. `templates/daily.html:55` branches on `{{if .DailyGroups}}`
and, when there are none, swaps the **entire page** for a centred "No expenses yet."
plus a primary add button. It works because an expense-indexed timeline with no
expenses has no groups at all.

The date-indexed timeline destroys that branch's premise. After
[Re-shape DailyGroups around dates](./04-date-indexed-daily-groups.md), a brand-new
account has **thirty gap-filled groups**, not zero — every day in the window exists.
`{{if .DailyGroups}}` is now always true, and a new user's first sight of the app is
thirty muted "no expenses" ledger lines from [ticket 01](./01-muted-empty-day-design.md),
each with its own `+`, and nothing else.

Resolve, one at a time:

- **Which state wins**, and on what condition. Keeping the page-swap means asking a
  question the window cannot answer — "has this account *ever* had an expense" is not
  "is this window empty". `GetEarliestExpenseDate` (`repository/sqlite.go`, already
  stubbed in both test doubles) answers it in one call, and
  [ticket 07](./07-scroll-island-contract.md) already relies on it for the scroll
  terminal. Is that the condition, or is a 30-muted-row page an acceptable — even
  honest — first run?
- **If the page-swap survives**, where does it live? It is a third top-level branch in
  `HandleDaily`, and [ticket 05](./05-converge-handledaily-branches.md) refused a third
  branch once already (for `<noscript>`). Does that refusal bind here?
- **The near-empty account.** An account whose only expense is four months old has a
  *window* full of empty days but is not new. It gets the timeline, not the empty state
  — confirm that, because it is the case that distinguishes "empty window" from "empty
  account", and it is the one a naive `len(expenses) == 0` check gets wrong.
- **The `?date=` view is not this.** An empty single-day filter already has its own
  fuller "No expenses for this day" state (ticket 05). This ticket must not change it.
- **What the empty state says.** The current copy is "No expenses yet." + "Add one".
  Ticket 08's ledger has a different visual language; does the copy or the affordance
  change with it?

## Answer

**The page-swap survives, re-conditioned.** An account that has *never* recorded an
expense sees the existing "No expenses yet." block, unchanged. Everyone else sees the
timeline, including accounts whose current window happens to be empty.

### The condition is `earliest == ""`, and it is free

`{{if .DailyGroups}}` (`daily.html:8`) is **dead** — after
[ticket 04](./04-date-indexed-daily-groups.md) a brand-new account has thirty gap-filled
groups, so the branch is always true. It is replaced by a flag, not deleted.

`GetEarliestExpenseDate` (`repository/sqlite.go:235`) is `SELECT MIN(date) FROM expenses`
and returns **`""` with a nil error** when the table is empty — `MIN` over no rows is
`NULL`, and the repo maps that to the empty string. So:

```
NoExpensesEver := earliest == ""
```

This costs **nothing**. The handler already calls it: [ticket 07](./07-scroll-island-contract.md)
needs it for the scroll terminal, and `HandleCalendar` already calls it at
`handlers_html.go:130`. No new SQL, no new query, no second source of truth.

The distinction it draws is **"ever", not "in this window."** That is the whole point, and
it is what a naive `len(expenses) == 0` gets wrong.

### It is a template branch, not a third handler branch

[Ticket 07](./07-scroll-island-contract.md) refused a `<noscript>` fallback because *"it
would grow `HandleDaily` a third branch."* That refusal is honoured literally:
`HandleDaily`'s control flow stays at **two** branches (`?date=` and the timeline). It
passes `NoExpensesEver` in page data, and `daily.html` branches on it — swapping the whole
timeline for the empty block exactly where `{{if .DailyGroups}}` used to.

Accepted cost, stated plainly: on a brand-new account the service still walks thirty empty
groups that the template then discards. That is thirty iterations over a query returning
zero rows, once, for a user who has no data. It buys a handler that does not grow.

### The dormant account gets the timeline

An account whose only expenses are four months old has `earliest != ""`, so it is **not**
new: it gets thirty muted rows and reaches its data by scrolling ([ticket 02](./02-window-size-and-pagination.md)'s
infinite scroll, terminating at the earliest expense). No hint, no jump link, no special
case. A dormant user who opens the app is usually about to add something to *today*, which
is precisely what the muted rows afford.

Rejected: swapping the page whenever the *window* is empty. It would hide a real timeline
from a user who has data, replacing a scroll that would have shown them their own expenses
with an "Add one" button.

### `MIN(date)` includes future dates — and this falls out clean

An account whose only expense is next month's rent has `earliest != ""`, so it is not new.
Correct: its timeline is thirty empty rows, but [ticket 03](./03-future-dated-expenses.md)'s
Upcoming section has content, so the page is not blank.

**This also settles a scroll-terminal edge nobody had looked at.** For that account,
`earliest` is a *future* date, so it already sits above the first window's start. Ticket
07's terminal rule ("stop at the earliest expense") therefore fires on the very first
window and the island never fetches. That is the right answer — there is genuinely nothing
older to load — so the rule needs no amendment. Recorded because it looks alarming and
isn't.

### The empty state itself is unchanged

"No expenses yet." + the primary **Add one** button, centred, verbatim from
`daily.html:55`. This block **replaces** the ledger rather than living inside it, so ticket
08's visual language does not bind it, and ticket 08 never touched it. The `/expenses/new`
link carries no date and [ticket 09](./09-validate-expense-dates.md) keeps empty dates
defaulting to today, so it already lands on the right day.

### What this does not touch

- **The `?date=` empty state.** An empty single-day filter keeps the fuller "No expenses
  for this day" state settled in [ticket 05](./05-converge-handledaily-branches.md). It is
  a different question — that day is empty, the account is not — and this ticket changes
  nothing about it.
- **`GET /daily/older`.** The empty state is a property of the *page*, never of a fragment.
  A new account never scrolls, and the endpoint's four `data-state` values
  ([ticket 06](./06-day-card-chrome-drift.md)) are unaffected.
