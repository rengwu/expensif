---
type: task
blocked_by: []
---

# The date-indexed service, wired end to end

## Question

Deliver the spec's **Service layer** and **clock seam** decisions as a working page:
`DailyGroupsInRange(ctx, start, end)` (gap-filled, newest-first, `Expenses` never
nil) and `UpcomingGroups(ctx, after)` (ungapped) replace `DailyGroups(ctx, limit)`,
which is deleted. `ErrInvalidRange` on an inverted range, `ErrInvalidDate` reused for
unparseable endpoints, both mapped to 400. The daily handler's unfiltered branch
computes today once in the Preferences timezone via an injectable clock, asks for the
rolling 30-day window plus upcoming groups, and passes a `NoExpensesEver` flag
(earliest expense date `== ""`).

This is a tracer bullet, not a service-only slice: the **existing card template
renders the new groups unchanged**, so the page becomes date-indexed — empty days
appear as the already-existing "No expenses for this day" cards, future expenses
still render above today (uncapped; the ledger chrome is the next ticket's job) —
and the change is demoable in a browser.

Tests come **first**, red, per the spec's Testing Decisions:
`TestDailyGroupsInRange_GapFillsEmptyDays`, the gap-fill cases, the today/upcoming
partition asserted from both sides in one test, the never-nil invariant in one named
test, `start > end` → error → 400, and the frozen-clock timezone test
(`2026-07-11T23:00Z`, `Pacific/Auckland`, window ends `2026-07-12`).

Done when: all named tests green, the old signature gone, and the browser shows a
gap-filled 30-day window on a scratch DB.

## Answer

Delivered as specified. The tests landed first and red (compile failure — the
methods did not exist), then the implementation turned them green.

- `DailyGroupsInRange` and `UpcomingGroups` live in the service, sharing a
  `groupByDate`/`newDailyGroup` pair; `DailyGroups(ctx, limit)` is deleted.
  `ErrInvalidRange` added; unparseable endpoints return the existing
  `ErrInvalidDate`. The gap-fill walk iterates UTC-parsed dates newest-first and
  hands an empty day `[]domain.Expense{}`, never nil.
- The handler grew the clock seam (`now func() time.Time`, defaulted in the
  constructor) and `nowInTZ` became a method on it; the calendar handler uses the
  same seam. `HandleDaily`'s unfiltered branch anchors at `Today` from page data,
  walks `[today−29, today]`, prepends `UpcomingGroups(today)`, and sets
  `NoExpensesEver` from the earliest-expense date (`""` means never).
- The `{{if .DailyGroups}}` branch in the daily template — dead once every window
  has thirty groups — became `{{if not .NoExpensesEver}}` **in this ticket**, not
  02: without it a brand-new account would have seen thirty empty cards, a
  regression the tracer bullet could not ship with.
- Verified on a scratch DB end to end: empty database shows "No expenses yet.";
  seeded, the page renders 29 gap-filled empty-day cards, the future expense above
  the past ones, a 40-day-old expense correctly outside the window; `?date=` and
  the calendar are unchanged.
- **For ticket 03:** `httpDateRangeError` maps `ErrInvalidDate`/`ErrInvalidRange`
  to 400 in the HTML layer, but through `HandleDaily` the path is defensive-only —
  its range is handler-computed, so no request can reach it. The HTTP-level 400
  test lands with `/daily/older`, where the range is user-controlled.
- The card markup and the `data-table` island are untouched — the new groups render
  through the old template until the ledger (ticket 02) replaces it.
