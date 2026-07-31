---
type: task
blocked_by: []
claimed_by: sefb73a827842
claimed_at: 2026-07-31T11:48:16Z
---

# The rule, its store, and the anchor arithmetic

## Question

Deliver the **Recurring Rule** as a persisted domain type with correct schedule arithmetic —
the foundation every later ticket stands on. No generation, no page, no goroutine. What this
ticket proves is that a Rule can be stored and round-tripped, and that asking it "when do you
next charge?" is right in the cases that matter.

Build:

- **The domain type.** `RecurringRule` carrying name, amount, currency, category, payer
  (optional), interval unit (months | weeks | days), interval count, anchor day, next charge
  date, end date (optional), and enabled. The name **is** the Description of the Expenses this
  Rule will mint — there is no separate description field, and adding one is a spec deviation.
  Validation mirrors `CreateExpense`'s existing shape: amount > 0, category required, name
  required, interval count >= 1. Currency and payer default from Preferences the way the add
  form already defaults them.

- **The migration.** A `recurring_rules` table plus a nullable rule reference on `expenses`,
  both applied through the existing additive `pragma_table_info` pattern in the db package —
  match it exactly rather than inventing a migration mechanism. Add the **uniqueness constraint
  on (rule reference, date)**: this is what makes double-generation structurally impossible
  rather than merely unlikely, and it is safe precisely because the minimum interval is one day,
  so a Rule can never legitimately owe two charges on the same date.

- **Repository CRUD** for Rules — create, get, list, update, delete — alongside the existing
  expense/user/preference repositories, with a service passthrough. The *transactional* delete
  that preserves past Expenses belongs to
  [What a rule owns](./03-what-a-rule-owns.md); a plain delete is enough here.

- **The anchor arithmetic.** Given an anchor day, an interval unit and count, and a date to
  advance from, compute the next charge date. Months clamp: each occurrence is
  `min(anchor day, days in that target month)`. Weeks and days are plain multiples of 24 hours
  and need no calendar reasoning. **The next date is always computed from the stored anchor,
  never by adding an interval to the previously clamped date** — that distinction is the entire
  defence against drift and the single most important line of code in this effort.

Where the arithmetic physically lives is yours to choose (a pure function with no dependencies
is the obvious shape), but per the spec's Testing Decisions it is **observed through the
service**, not tested directly at a lower seam.

Tests land before implementation. The leading red is the anchor series:

> Jan 31 → Feb 28 → **Mar 31** → Apr 30 → May 31

Mar 31 is the whole assertion. A drifting implementation returns Mar 28 here and stays plausible
forever after, so if you write only one test in this ticket, write that one. Cover a leap-year
February, a 30-day anchor into February, an ordinary mid-month anchor that never clamps, and
week/day intervals crossing a month boundary.

Repository tests run real SQL over in-memory SQLite — the existing expense and rate repository
tests are the prior art and already migrate a fresh database per test. Prove the migration
applies cleanly to a database that already has expenses in it, and prove the uniqueness
constraint actually rejects a duplicate (rule, date) pair; a mock repository cannot demonstrate
either.

**Done when:** a Rule can be created, read back and listed through the service; the anchor
series above passes; the migration is proven against a populated database; and the uniqueness
constraint is proven to reject duplicates.

## Answer

Built, in `internal/domain/models.go`, `internal/db/db.go`, `internal/repository/` and
`internal/service/recurring.go`:

- **`domain.RecurringRule`** with every field the spec names, plus `domain.IntervalUnit`
  (`months` | `weeks` | `days`). No description field — the Name is the Description of what the
  Rule mints. `domain.Expense` gains `RecurringRuleID`, which is the *only* thing that
  distinguishes a minted Expense from a hand-entered one.
- **The migration**, appended to `Migrate` in the existing additive style: a `recurring_rules`
  table, `ALTER TABLE expenses ADD COLUMN recurring_rule_id` behind the same
  `pragma_table_info` check the other columns use, and the uniqueness constraint as a
  **partial** unique index on `(recurring_rule_id, date) WHERE recurring_rule_id IS NOT NULL`.
  Partial because SQLite treats NULLs as distinct anyway, and stating it makes the intent
  explicit: rules cannot double-charge a date, hand-entered Expenses are untouched (two coffees
  on one day stay legal). Also an index on `(enabled, next_charge_date)`, which is the cheap
  "which Rules are due" query the spec says `next_charge_date` is persisted for.
- **Repository CRUD** (`RecurringRuleRepository`, added to the `Repository` composition and to
  `Repos`) with a service passthrough. `DeleteRule` is the plain delete; the transactional one
  is ticket 03's.
- **The anchor arithmetic**, `nextCharge` in `internal/service/recurring.go`, observed through
  `Service.NextChargeDate(rule, after)`. For months it takes only the *month* from the date it
  advances from and the *day* from the stored anchor, clamped to that month's length. Weeks and
  days are plain day multiples.

**Done-when, clause by clause.** A Rule is created, read back and listed through the service
(`TestCreateRule_RoundTripsThroughGetAndList`, plus the CRUD round-trip over real SQL). The
anchor series passes (`TestNextChargeDate_AnchorSeriesNeverDrifts`, feeding each result back in
as the next input, which is the path generation will take). The migration is proven against a
populated database (`TestMigrateOverPopulatedDatabase` builds the pre-effort schema, seeds three
expenses, migrates, and asserts all three survive with the recorded total intact and no rule
reference; it also re-migrates to prove idempotence). The uniqueness constraint is proven to
reject duplicates (`TestRuleDateUniquenessRejectsDuplicates`), including the three cases that
must still be allowed: the same rule on another date, another rule on the same date, and
unlinked Expenses repeating a date.

**The leading red was verified, not assumed.** Before writing the real arithmetic I ran the
series test against a deliberately drifting implementation (anchor taken from the previous
date's day) and watched it fail with exactly the predicted `2026-03-28`. The canary bites.

Also checked live: the migration ran against a *copy* of the real dev database (13 expenses),
the server booted and served 200, and every table and index arrived. The dev database itself was
never opened — per the map's warning, only the copy under a scratch `DATA_DIR`.

**Decisions I had to make, for a human to overrule if they're wrong:**

1. **The service's rule API takes a `domain.RecurringRule`, not flat arguments.** `CreateExpense`'s
   precedent is flat parameters, but a Rule has eleven fields and eleven positional arguments is
   a bug factory. Validation still mirrors `validateExpenseInput`'s shape.
2. **Currency defaults from Preferences; the payer deliberately does not.** The spec says both
   default "the way the add form already defaults them" — and the add form defaults the payer in
   its *handler*, by pre-selecting `prefs.UserID` in the form (`HandleAdd`), not in the service.
   Defaulting it in the service would make "no payer" inexpressible, since an unset payer and a
   cleared one are both 0. **Ticket 04 must pre-select the default payer in the Recurring form's
   handler**, the way `HandleAdd` does, or that spec line goes unimplemented.
3. **An unset anchor day defaults to the start date's day-of-month**, so a form asking only
   "when does it first charge?" is sufficient. An anchor outside 1–31 is rejected.
4. **An end date before the first charge is rejected** (reusing `ErrInvalidRange`); an end date
   *equal* to it is allowed, being the degenerate one-charge Rule. The spec only says what
   happens when a *computed* date passes the end date, so this is a validation call, not a
   scheduling one.
5. **`Enabled` is not defaulted on create** — the caller states it. A zero-valued struct
   therefore creates a *disabled* Rule that generates nothing; ticket 04's form must send the
   checkbox state explicitly.

**Deliberately left out:** generation (02), the transactional delete and re-enable semantics
(03), the page and Monthly Equivalent (04), and any handler or template. `UpdateExpense` still
does not write `recurring_rule_id`, which is what lets a Pending Charge be edited as an ordinary
Expense without severing it from its Rule — noted here because ticket 03 depends on it.

One cleanup beyond the strict ticket, flagged for transparency: the expense column list and row
scan were repeated at three sites in `sqlite.go`, and adding the rule reference meant editing all
three. I extracted them into one `expenseColumns` + `scanExpense` pair (mirroring the
`ruleColumns` + `scanRule` I was writing anyway), so the next column lands once. Pure refactor,
covered by the existing repository and handler tests.
