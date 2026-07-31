---
type: task
blocked_by: [01]
claimed_by: scc81d1cfe1ee
claimed_at: 2026-07-31T13:42:35Z
---

# Generation: one charge ahead, catch-up, and the end date

## Question

Make Rules mint Expenses. This is the heart of the effort and the ticket the spec's primary
test seam exists for. Still no page and no goroutine — generation is a service call the tests
drive directly.

Build **the service's generation API, taking the current date as an explicit argument.** It
must never read the clock internally. This is a testability requirement first and a correctness
one second: charge dates are Timezone-sensitive and the Timezone lives in Preferences, so the
only caller that knows the real "today" is the one that will be wired up in
[Live generation](./05-live-generation-goroutine-and-flash.md).

The behaviour:

- **One charge ahead.** After generation runs, every enabled Rule has exactly one Expense
  materialized with a date after the supplied date — its **Pending Charge** — and no more.
  Running generation twice with the same date changes nothing; this idempotence is the property
  most worth testing, because the goroutine will call it repeatedly.

- **A Pending Charge is an ordinary Expense.** Future-dated, carrying the Rule's amount,
  currency, category, payer and its name as the Description, plus the rule reference. There is
  no new type, no status column, and nothing that distinguishes it from a hand-entered Expense
  to any existing view. That is why the Ledger, the Calendar View and Export need no changes at
  all — verify that claim rather than assuming it.

- **Catch-up.** When the supplied date is far enough ahead that several charges came due, mint
  **every one of them on its own true date**, in order, and advance the Rule past all of them.
  Return the count of Expenses minted so a caller can report it. Test the case the spec names:
  a monthly Rule last generated in January, generation run with an April date, producing four
  dated Expenses and leaving exactly one pending.

- **The end date.** When the newly computed next charge date would fall after the Rule's end
  date, the Rule **disables itself** instead of scheduling. A Rule with no end date is
  open-ended. Test the boundary precisely: a charge falling exactly *on* the end date is
  generated; the one after it is not, and the Rule comes back disabled.

- **Disabled Rules generate nothing** — no Pending Charge, no catch-up, not even a date
  advance. The clock stops. What happens when one is re-enabled belongs to
  [What a rule owns](./03-what-a-rule-owns.md), which is where the trap lives.

All of it is asserted at the service seam by supplying dates, per the spec's Testing Decisions.
Prior art: the existing service tests, which already inject a rate-fetcher double and drive
behaviour through it. Assert *which Expenses exist after generation runs* — never how the
scheduler looped or which intermediate dates it considered.

**Done when:** generation is idempotent for a given date; a monthly Rule produces the four
backfilled Expenses and one pending charge described above; the end-date boundary behaves as
specified; disabled Rules are untouched; and an existing Ledger and Export test proves a
Pending Charge flows through them unchanged.
