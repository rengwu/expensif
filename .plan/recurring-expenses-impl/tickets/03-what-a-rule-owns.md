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

## Answer

Built the Rule-ownership operations at the service seam and the one SQL guarantee that belongs
below it:

- **Editing owns only the future.** `Service.UpdateRule(ctx, rule, today)` now reads the stored
  Rule, validates the edit, and rewrites the Rule's strictly future linked Expense in full:
  amount, Description from the Rule name, Currency, Category, Payer and date. A schedule edit
  recomputes the Pending Charge from the occurrence before the existing one through the same
  anchor-aware `stepCharge` arithmetic generation uses; forward and reverse scheduling cannot
  grow separate interval switches. Moving an end date before the recomputed charge disables the
  Rule and removes the future charge rather than rejecting the edit.
- **Ordinary Pending-Charge edits remain ordinary.** When `UpdateExpense` moves the date of the
  Expense currently named by a Rule's `NextChargeDate`, it moves that stored date too. A later
  generation run therefore sees the edited charge, does not mint a duplicate at the old date,
  and disable/delete still own the moved future row. Historical linked Expenses never move a
  schedule.
- **Disable and re-enable are explicit.** `SetRuleEnabled(ctx, id, enabled, today)` deletes the
  one strictly future Pending Charge on disable. Re-enable walks the stopped schedule to the
  first anchored occurrence on or after `today`, skipping every disabled occurrence, then
  mints only that charge. `today` remains a caller argument; neither path reads a clock.
- **Delete is transactional.** SQLite's `DeleteRule(ctx, id, today)` follows the existing user
  deletion shape: inside one transaction it deletes linked Expenses strictly after `today`,
  clears `recurring_rule_id` from every survivor, and drops the Rule. A charge dated today is
  ledger history, not a Pending Charge, so it survives alongside earlier Expenses.

**Done-when, clause by clause.**

- The paired documentation tests sit together:
  `TestGenerateCharges_CatchesUpOnEveryMissedCharge` still produces January through May after an
  outage, while `TestSetRuleEnabled_ReenableSkipsTheDisabledStretch` starts from the same January
  date and, when resumed on 2026-04-20, creates only the 2026-05-15 Pending Charge. Re-running
  generation mints zero.
- `TestUpdateRule_RewritesPendingChargeAndLeavesPastExpensesUntouched` changes every charge field
  plus interval count and Anchor Day, verifies the same Pending Charge moves to 2026-06-20, and
  compares all Expenses through the supplied date byte-for-byte before and after. Running the
  edit on a charge date is deliberate: the due-today Expense proves the strict-future boundary.
- `TestSetRuleEnabled_DisableRemovesOnlyThePendingCharge` proves disabling on a charge date keeps
  that day's Expense and all earlier history while removing only the future one.
- `TestUpdateExpense_MovingPendingChargeKeepsRuleOwnershipInSync` moves a Pending Charge through
  the ordinary Expense API, proves generation remains idempotent, and proves disable finds the
  moved row. `TestUpdateRule_EndDateBeforePendingChargeDisablesRule` covers the edit-time end-date
  boundary.
- `TestDeleteRulePreservesHistoryAndRemovesItsPendingCharge` runs over migrated in-memory SQLite.
  The future charge deliberately differs from the stored date, modelling a direct Expense edit;
  it is deleted, the due-today and past Expenses survive with their Rule references cleared, an
  unrelated Expense survives, and the Rule is gone.

The ownership tests were first run red against the previous API (missing `today`, toggle and
transactional ownership operations), then implemented. Final verification: `go test ./...`,
`go vet ./...`, `go test -race ./internal/service ./internal/repository`, `git diff --check`, and
the recurring-expenses implementation-map linter all pass; the linter reports only its two
pre-existing closed-ticket claim warnings plus the same inert warning for this now-closed
ticket. The required two-axis code review finished with no
Spec findings and no documented-standard violations. Its one remaining judgement-call smell is
the broad `ExpenseRepository` gaining two more recurring-charge methods; I kept that inherited
seam because resolved ticket 02 explicitly placed `MintCharge` there and Pending Charges remain
ordinary Expenses. No TypeScript changed, so the UI checks were not run.

**Deliberately left out:** the Recurring page and Monthly Equivalent (04), live goroutine and
Flash delivery (05), List View bounds (06), and architecture/glossary updates (07). Ticket 04
must derive `today` in the configured Timezone and pass it to edit, toggle and delete handlers.
