---
type: task
blocked_by: [02]
claimed_by: sb4c38ea7dc23
claimed_at: 2026-07-31T14:44:22Z
---

# What a rule owns: edit, delete, disable and re-enable

## Question

Implement the ownership rule the spec settles: **the future belongs to the Rule, the past
belongs to the ledger.** Every operation here is a variation on that one sentence, and each has
a way of going wrong that silently damages recorded history.

Build, at the service seam:

- **Editing a Rule rewrites its Pending Charge** — every field, not only the amount: name into
  Description, currency, category, payer. When the interval, count or anchor day changed,
  recompute the pending date too. The point is that a price rise is corrected in one place
  *before* it ever becomes history.

- **Editing never touches a past Expense.** Assert this directly rather than trusting it: a Rule
  with several generated Expenses behind it, edited, and every past Expense byte-identical
  afterwards. This is what guarantees a CSV exported last month still matches the app.

- **Deleting a Rule** follows the existing user-deletion precedent exactly — one transaction
  that clears the rule reference on the Rule's past Expenses, deletes its Pending Charge, and
  drops the Rule. Read that precedent before writing this; it already solves the same problem
  for payers, and matching it is worth more than a marginally tidier alternative. Past Expenses
  survive as ordinary unlinked Expenses.

- **Disabling removes the Pending Charge** and stops the clock. Upcoming must not advertise a
  charge that isn't going to happen. Past Expenses stay — pausing something never erases the
  months already paid for.

- **Re-enabling resumes from today.** The next charge is the first anchor occurrence on or after
  today, and **nothing is backfilled for the disabled stretch.**

That last point is the trap this ticket exists for. Catch-up and disable are both correct rules
that meet here, and the wrong answer dumps months of invented Expenses into the ledger the
moment someone un-pauses a subscription they paused in the spring. The test that pins it: a
monthly Rule disabled in January, re-enabled with an April date, producing **exactly one**
Pending Charge and **zero** backfilled Expenses — deliberately the same scenario as
[Generation's](./02-generation-catch-up-and-end-date.md) four-charge catch-up test, differing
only in that the Rule was disabled. Put the two tests next to each other; the pair is the
documentation.

The transactional delete is proven at the repository seam over in-memory SQLite, because
"past Expenses survived, the pending one didn't, and the rule reference was cleared" is a SQL
guarantee a mock repository cannot demonstrate. Everything else is asserted at the service seam.

**Done when:** the two paired catch-up/re-enable tests pass; editing rewrites the Pending Charge
in full and provably leaves past Expenses untouched; and the delete transaction is proven to
preserve history while removing the Rule and its Pending Charge.
