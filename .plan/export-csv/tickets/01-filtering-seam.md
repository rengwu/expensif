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
