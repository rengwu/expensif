---
type: task
blocked_by: []
claimed_by: s72bcbbe89d41
claimed_at: 2026-08-01T19:13:57Z
---

# Bounding the List View to today

## Question

Stop the List View counting money that hasn't been spent.

The List View's total and Category breakdown are computed over every Expense with no date bound
at all. Once Rules exist, every enabled one permanently contributes a future-dated Pending
Charge, so the headline figure would overstate by a full cycle of subscriptions and stay wrong.

**Exclude future-dated Expenses from the List View's total and its Category breakdown.** The
number means "what I have actually spent", and it should agree with itself: total and breakdown
must be bounded identically, or the page contradicts its own summary.

This is deliberate and spec'd, not a side effect — which is why it is its own ticket rather than
a quiet line inside another. It also fixes the same latent distortion for **hand-entered**
future-dated Expenses, which can already skew that total today, before any of this effort ships.
That is why this ticket is unblocked and takeable on its own: it stands as a bug fix even if
nothing else on this map is built yet.

Two things to decide and record in the answer: whether future-dated Expenses still appear as
**rows** in the list (only the summary is bounded) or are filtered out entirely — the spec bounds
the *total and breakdown* and says nothing about rows, and showing a row you cannot find in the
total is arguably worse than either — and whether "today" here is computed in the user's
configured Timezone, which it should be, for the same reason it is everywhere else.

Do not widen this into a redesign of the summary area. A split into spent-to-date and upcoming
figures was considered during grilling and ruled out as a larger job than the recurring feature
itself.

Tested at the existing HTML handler seam, the way the other HTML handler tests drive real
requests against a mock repository: given Expenses on both sides of today, the total and the
Category breakdown count only the past ones.

**Done when:** the List View's total and Category breakdown exclude future-dated Expenses and
agree with each other; the row-visibility question is decided and recorded; and the existing
List View tests still pass.

## Answer

The List View now ends at today, and it ends there **once**. `HandleList`
(`internal/web/handlers_html.go`) builds a single `domain.ExpenseFilter{Start: "0001-01-01", End:
data.Today}` and reads everything through it: the rows via `SampleMatching(ctx, spent, 100)`, the
converted total and the Category breakdown from those same rows, and the unconverted fallback
total via a new `SumMatching(ctx, spent)`. Total and breakdown cannot disagree, because they are
not bounded twice — they are bounded once and read from the same set.

**The rows are bounded too — the decision this ticket asked for.** The rows are the evidence for
the total; a row visible above a total that excludes it makes the page argue with itself, and the
list is ordered newest-first, so those rows would sit at the very top, permanently, one per
enabled Rule. Nothing is lost: a Pending Charge is a future-dated Expense, and the Ledger shows
future days above today with the same edit and delete links (the Calendar and every Export are
unchanged too), so the one place the spec designates for forward visibility is still the place it
happens. Because rows are bounded at the query rather than filtered after it, the page still shows
100 rows — a future charge no longer eats one of the slots.

The cost of that choice is that a page titled "All Expenses" now hides something, so it says so:
one muted line in the summary card — "Up to today — future-dated expenses stay on the Daily view
until their date arrives" — which is a sentence, not the spent-to-date/upcoming *split* the
grilling ruled out. No figure was added, no second total, no new section. If a human would rather
the page said nothing, deleting that `<div>` in `templates/list.html` is the whole reversal.

**"Today" is `data.Today`** — `time.Now()` in the Preferences Timezone, the same value the Ledger
and the Recurring page bound on, for the reason it is used everywhere else: a charge due today
must not read as future because the server sits west of the user. The bound is inclusive of today
(`BETWEEN … AND today`), because today's spending is spent.

**Why a new repository method.** The converted total and breakdown come from the fetched rows, so
they bounded themselves once the fetch was bounded. The *fallback* total — the headline when rates
are unavailable — came from `TotalExpenses`, an unbounded `SUM(amount)`, and leaving it alone
would have kept the page overstating on exactly the days conversion is down. `SumMatching` is
`CountMatching`'s sibling: same `filterWhere` helper, same by-construction agreement with the rows.
It is the only widening in this change, and `TotalExpenses` stays exactly as it was for
`/api/summary`, which the map puts out of scope (it is still unbounded and still unconverted — a
`curl` during verification confirmed 115 where the page says 15).

**Done-when, clause by clause.**

- *Total and breakdown exclude future-dated Expenses and agree* —
  `TestHandleList_ExcludesFutureDatedExpenses` (`internal/web/handlers_html_test.go`) drives a real
  `GET /expenses` through `routes()` against the mock repository with three Expenses — past, today,
  and a future Pending Charge with `RecurringRuleID: 7` — and asserts `Total: $15.00`, a breakdown
  of `food: $10.00` and `transport: $5.00`, and no "subscriptions" anywhere.
  `TestHandleList_FallbackTotalIsBoundedToo` asserts the same bound on the no-rates branch, and
  `TestHandleList_BoundIsTodayInThePreferencesTimezone` pins the timezone: at 2026-08-02T23:00Z it
  is already Aug 3 in Auckland, so that day's Expense counts.
- *Row visibility decided and recorded* — above, and asserted: the Netflix row is absent while
  "lunch" and "bus" are present.
- *Existing List View tests still pass* — there were no HTML List View tests to keep (this is the
  first); the JSON `/api/expenses` list tests in `handlers_api_test.go` are untouched and green, as
  is the rest of `go test ./...`. `go vet ./...` clean. No TypeScript changed — the DataTable island
  renders server-supplied props and fetches nothing — so the UI checks were not run.

**All three tests were verified red**, not assumed: widening the filter's `End` to `9999-12-31`
fails the first two; computing the bound as `h.now().UTC()` fails the third. At the SQL seam,
`TestSumMatchingAgreesWithExpensesMatching` (`internal/repository/expenses_test.go`) checks the sum
equals the rows' total for four filters including one matching nothing, where SQLite's `SUM` is
NULL and an unguarded scan would error where zero is the answer.

**Driven against the real app** on a scratch DB under the session scratchpad (`DATA_DIR=…
PORT=8177`; `~/.expensif` was never written to — an early attempt on :8090 hit another session's
server and its writes were refused, so nothing landed anywhere). Three Expenses (Jul 1, today,
Aug 31): the page showed `Total: $15.00`, `food: $10.00`, no subscriptions row and no Netflix row,
with the Daily view still showing Netflix in Upcoming.

**Doubts for a human:**

1. **The empty state can now read "No expenses yet." when Expenses exist** — only if *every*
   Expense is future-dated, which realistically means a fresh install whose first Rule has just
   minted its Pending Charge. The clarifier line sits directly above it, so the page is not silent
   about where they went. Rewording that state felt like the redesign this ticket forbids.
2. **The fallback total still mixes currencies** and still covers all history while the rows show
   100 — both pre-existing, both untouched. I bounded it by date and changed nothing else about it.

**Deliberately left out:** any splitting of the summary into spent and upcoming figures; any change
to `/api/summary` or `/api/expenses`; the CONTEXT.md and ADR vocabulary landing, which the map
assigns to ticket 07. No Island, and no other view's behaviour changed.
