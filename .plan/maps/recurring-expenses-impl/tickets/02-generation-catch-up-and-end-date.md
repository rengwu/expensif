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

## Answer

Built, in `internal/service/recurring.go` (with the seam it needs in
`internal/repository/`):

- **`Service.GenerateCharges(ctx, today) (int, error)`** — the generation API. `today` is the
  current date in the user's Timezone and is always an argument; nothing under it reads a
  clock. It walks every *enabled* rule forward from its stored `NextChargeDate`, minting each
  charge on its own true date until it lays down the one falling after `today` — the Pending
  Charge — then stops, and returns how many Expenses it created across all rules.
- **Catch-up is not a separate mode.** A rule last generated in January simply takes several
  turns of the same loop before it reaches a future date, and each turn advances by
  `nextCharge` — recomputed from the stored anchor, never accumulated onto a clamped date.
- **`ExpenseRepository.MintCharge(ctx, expense) (created bool, err error)`** — the new seam,
  implemented as an `INSERT … ON CONFLICT (recurring_rule_id, date) WHERE recurring_rule_id IS
  NOT NULL DO NOTHING`. Idempotence is therefore *derived from the data*, not from a state flag
  the service maintains: ticket 01's partial unique index is what decides, exactly as the spec
  intended when it called double-generation "structurally impossible". A conflicting insert
  also leaves the existing row alone, so a Pending Charge a human edited survives every
  subsequent run — which ticket 03 depends on.

**Done-when, clause by clause.**

- *Idempotent for a given date* — `TestGenerateCharges_IsIdempotentForTheSameDate` runs
  generation four times on one date; runs 2–4 mint 0 and the set of Expenses never moves.
- *Four backfilled Expenses and one pending* — `TestGenerateCharges_CatchesUpOnEveryMissedCharge`
  is the spec's named case verbatim: a monthly rule with `NextChargeDate` 2026-01-15, generated
  with 2026-04-20, yields Jan/Feb/Mar/Apr 15 plus 2026-05-15, exactly one Expense after the
  supplied date, the rule advanced to 05-15, and re-running mints nothing.
  `TestGenerateCharges_CatchUpKeepsTheAnchor` is the anchor canary *on the path generation
  walks*: anchored on the 31st from Jan 31 it must produce Feb 28 → **Mar 31** → Apr 30.
- *End-date boundary* — `TestGenerateCharges_EndDateBoundary`: the charge landing exactly on the
  end date is generated and the rule stays enabled; once it is behind us the next computed date
  falls past the end date, so the rule comes back **disabled** having minted nothing, and stays
  that way however far ahead generation is later run.
- *Disabled Rules untouched* — `TestGenerateCharges_DisabledRuleGeneratesNothing`: no charge, no
  catch-up for the months it sat paused, and `NextChargeDate` left exactly where it was.
- *A Pending Charge flows through the Ledger and Export unchanged* — checked rather than
  assumed, and checked by control: `TestHandleDaily_PendingChargeIsIndistinguishableFromAHandEnteredExpense`
  renders the Ledger twice over identical data, once with the rule reference and once without,
  and requires the two responses to be **byte-identical** (plus asserting the charge's day
  actually reaches Upcoming). `TestHandleExportCSV_PendingChargeExportsLikeAHandEnteredExpense`
  does the same for the CSV rows. Since the rule reference is the only difference between the
  two runs, any differing byte would be an existing view having learned a new concept. Neither
  needed a line of production change.
- Also `TestGenerateCharges_MaterializesOneChargeAhead` (the whole "a Pending Charge is an
  ordinary Expense" clause: the minted row is asserted field-for-field),
  `TestGenerateCharges_CountsEveryRulesCharges` (the count spans rules; one rule's pause does
  not stop another's schedule) and, over real SQL,
  `TestMintChargeLeavesAnAlreadyMintedDateAlone`.

**Both canaries were verified red, not assumed.** Against a rule that advances *past* its
Pending Charge, the idempotence assertions fail with a second charge appearing on the second
run; against arithmetic that takes the day from the date it advances from, the catch-up series
returns the predicted `2026-03-28`. Restored and green: `go test ./...`, `go vet ./...`, gofmt
clean. No TypeScript changed, so the UI checks were not run. Nothing was run against the real
app: generation has no caller until ticket 05, and the one genuinely SQL-dependent piece (the
`ON CONFLICT` against a *partial* index) is covered by a repository test on real SQLite.

**Decisions I had to make, for a human to overrule if they're wrong:**

1. **`NextChargeDate` is the date of the Pending Charge**, not the one after it. Once generation
   has minted a rule's future charge, the rule's stored date equals that charge's date, so the
   Recurring page's "next charge" and the row sitting in Upcoming are the same date. The
   alternative — advancing past the pending charge — makes the rule alone unable to say whether
   that charge was already minted, and was the shape the leading-red run confirmed breaks
   idempotence.
2. **The returned count is every Expense created, the backfilled and the pending alike** — five
   in the spec's catch-up case, not four. It is literally "the count of Expenses minted", and
   the steady-state run reports 1. Ticket 05 owns how that becomes Flash wording; if the human
   wants "4 missed charges added", it must distinguish them there, or this signature changes.
3. **A self-disabled rule keeps the computed next date** (past its end date) rather than being
   left on its last charge. It never moves backwards, and `Enabled = false` is what tells a
   reader it will not happen — but **ticket 04 should not render a next-charge date for a
   disabled rule** without deciding it reads sensibly.
4. **Generation reads `ListRules` and skips disabled rules in Go**, rather than adding a
   "which rules are due" query. A rule whose next charge is in the *future* but not yet
   materialized still needs generating, so a `next_charge_date <= today` query would miss
   exactly the newly-created rule; the index still serves the `enabled` half. Worth revisiting
   only if rules ever number in the thousands.
5. **A rule with a broken schedule or unparseable stored date is reported, not skipped
   silently**, and never blocks the others: every rule is attempted and the first error comes
   back alongside the count. Stored rules are validated on create, so this is defence against a
   hand-edited database.

**Deliberately left out:** the transactional delete, re-enable semantics and edit-rewrites-the-
Pending-Charge (03); the page and Monthly Equivalent (04); the goroutine, the real clock and the
Flash (05); the List View's bounding (06). One observable consequence worth naming for ticket
03: deleting a Pending Charge by hand while its rule is still enabled will see it re-minted on
the next run, because the rule still says it charges then — disabling the rule is the way to
stop it, which is precisely what 03 specifies.
