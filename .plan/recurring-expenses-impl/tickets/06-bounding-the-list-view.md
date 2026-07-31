---
type: task
blocked_by: []
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
