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
