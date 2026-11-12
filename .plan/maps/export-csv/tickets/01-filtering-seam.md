---
type: grilling
blocked_by: []
---

# The server-side filtering seam

## Question

The Export page filters by date range, category, and payer before writing the CSV, but
`/expenses` filters **client-side** — the server has no notion of a filtered expense set
today. How should a filtered set reach the exporter?

Decide the seam, using the `codebase-design` skill:

- **One method or several?** `ExpensesInRange(start, end)` already exists
  (`service.go:99`). Do category and payer become extra parameters on a widened method,
  a small `ExpenseFilter` struct passed to one query, or successive in-memory filters
  applied to a range result? (Category/payer are equality filters; the range is a SQL
  `BETWEEN`.)
- **Where does conversion happen?** `ConvertedAmount`/`PaidByName` are computed at render
  time. Does the filtering seam return raw `[]Expense` and leave conversion+naming to the
  handler (as the views do), or hand back rows already enriched?
- **Empty and absent filters.** "No category selected" must mean *all categories*, not
  *zero rows*. Pin how an unset filter is represented so a blank form exports everything.
- **Reuse.** Is this seam Export-only, or is it the filtering `/expenses` should have had?
  Decide whether to build it narrow (Export only) or general — but do not scope-creep the
  List view's redesign into this map.

This is foundational: the route (04), the page UI (05), and the test strategy (08) all
depend on the shape chosen here.

## Answer

**One filter value, one query, at the repository — raw rows out, converted by the
handler, Export the only caller.**

- **The seam is a single method taking an `ExpenseFilter` value**, not several methods
  and not a positional parameter list:

  ```go
  type ExpenseFilter struct {
      Start, End string // required date range (YYYY-MM-DD, inclusive)
      Category   string // "" = all categories
      PayerID    int64  // 0  = all payers
  }

  // repository.ExpenseRepository, and a service passthrough
  ExpensesMatching(ctx, ExpenseFilter) ([]domain.Expense, error)
  ```

  The SQL is the existing range query with two optional predicates appended when the
  fields are set — `AND e.category = ?`, `AND e.paid_by = ?`. Payer filtering is pure
  SQL: the repo already `LEFT JOIN`s `users` and populates `PaidByName`
  (`sqlite.go`), so nothing new joins and no name lookup happens in Go. The
  interface stays **deep** — one entry point absorbs range + category + payer + any
  future filter — instead of growing a third near-clone of `ListExpenses` /
  `ListExpensesInRange`.

- **"Unset" is the zero value.** `Category == ""` and `PayerID == 0` mean *all*, so a
  blank Export form exports everything. There is no separate "absent filter" sentinel to
  invent — this is the reason the struct beat a positional param list, where `("", 0)`
  would have to carry the same meaning by convention. (Category/payer are equality
  filters; the range is the required `BETWEEN`.)

- **The seam returns raw `[]domain.Expense`; the handler converts.** `ConvertedAmount`
  is *not* populated by the query. The export handler runs conversion afterward via the
  existing `GetRatesForConversion` / `ConvertWithRates`, exactly as `HandleList` and the
  daily views already do. This keeps the query a pure read and confines every
  rate/rate-date decision to ticket 02's layer, out of the query seam. `PaidByName` *is*
  populated (it rides the JOIN, as today).

- **Blast radius: new method is the one home; the old name delegates.**
  `ExpensesMatching` holds the single copy of the SELECT + scan.
  `ListExpensesInRange(ctx, start, end)` stays in the interface but its body becomes a
  one-line delegate — `ExpensesMatching(ctx, ExpenseFilter{Start: start, End: end})` —
  so the just-shipped daily-timeline callers (`DailyGroupsInRange`, `UpcomingGroups`,
  `HandleDaily`) are **untouched** while the duplicated SQL collapses. Replacing the
  method and migrating those callers was rejected as scope creep against stable code;
  leaving a third fully-separate duplicate was rejected as shallow.

- **Export is the only *filtering* caller for now.** The List view (`/expenses`) keeps
  its client-side `data-table` filtering; wiring it to `ExpensesMatching` is a separate
  effort, explicitly out of this map. The struct is general enough to serve that later
  migration for free (zero-values), but no code is built for a caller that does not yet
  exist.

**Terms minted** (for ticket 07 / `CONTEXT.md`): **`ExpenseFilter`** — a date range plus
optional category and payer, zero-values meaning "all"; **`ExpensesMatching`** — the deep
query it drives. **Facts later tickets depend on:** ticket 04's route inputs map
one-to-one onto `ExpenseFilter`'s fields; ticket 05's controls offer exactly those three
filters; ticket 08 tests the seam at the service, with the sharp case that an unset filter
means *all*, not *none*.
