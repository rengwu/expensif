# Recurring Expenses — spec

Synthesized from a `grill-me` session (eleven decisions, all resolved) rather than from a
planning wayfinder map — the interview did that map's job, so there is no
`.plan/recurring-expenses/map.md` and no fog to chart. Code lands under a separate
`recurring-expenses-impl` map, the way `daily-timeline` → `daily-timeline-impl` and
`export-csv` → `export-csv-impl` ran. Uses `CONTEXT.md` vocabulary throughout and respects
ADR 0001 (React Islands).

## Problem Statement

Most of what I spend is predictable, and Expensif makes me type all of it in by hand. Rent,
Netflix, the phone bill, insurance, the gym — these arrive on the same day every month for
the same amount, and every single one of them is a manual Expense I have to remember to
record. When I forget, my totals are quietly wrong: the Daily View, the List View and any
Export I take are all missing my largest and most regular spending, and I have no way to
tell *which* months are incomplete without going through a statement line by line.

The app also can't tell me the one number I actually want from this category of spending —
what my subscriptions cost me per month. Because each charge is just another Expense
indistinguishable from a coffee, there is nothing that knows "these recur", and so nothing
that can add them up. The Ledger's **Upcoming** section, which exists to show me future-dated
days, is permanently empty for the very expenses whose future dates I already know.

## Solution

A new **Recurring** page listing every **Recurring Rule** — the things that charge me on a
schedule — each showing its name, next charge amount and date, interval, and whether it's
enabled. At the top sits a single **Monthly Equivalent**: every enabled Rule normalized to a
per-month figure and converted into my Preferred Currency, so "≈ $148/month on subscriptions"
is answerable at a glance.

A Rule is not a passive reference row. It **mints real Expenses** — ordinary ones, appearing
in the Ledger, the Calendar View, the List View and every Export exactly like a hand-entered
Expense, because that is what they are. Each Rule keeps exactly one charge materialized ahead
of today (its **Pending Charge**), so what's coming is visible in **Upcoming** before it
happens and can be corrected while it's still in the future. When its date passes it stops
being pending and simply becomes history; the next one is minted behind it.

Generation runs in a single background goroutine — once at startup, then on a ticker — the
same shape as the existing exchange-rate fetcher. The startup run is what makes the app
self-healing: if it was down across a charge date, **Catch-up** backfills every Expense that
was missed, on its true date, and says so via a Flash message. That catch-up covers time the
*app* was absent and never time *I* deliberately paused, because disabling a Rule stops its
clock entirely.

## User Stories

1. As someone tracking household expenses, I want a Recurring page listing everything that
   charges me on a schedule, so that I can see my committed spending in one place.
2. As someone tracking household expenses, I want to create a Recurring Rule with a name, so
   that I can recognise it in the list.
3. As someone tracking household expenses, I want the Rule's name to become the Description
   of every Expense it mints, so that I don't maintain two labels for the same thing.
4. As someone tracking household expenses, I want to set the charge amount on a Rule, so that
   the Expenses it creates have the right value without my editing each one.
5. As someone tracking household expenses, I want to set the Currency on a Rule, so that a
   subscription billed in USD and one billed in MYR are both recorded truthfully.
6. As someone tracking household expenses, I want a Rule's Currency to default to my Preferred
   Currency, so that the common case needs no thought.
7. As someone tracking household expenses, I want to set the Category on a Rule, so that its
   Expenses land in the right Category and appear correctly in my breakdowns.
8. As someone tracking household expenses, I want to optionally set a Payer on a Rule, so that
   the household knows who covers which subscription.
9. As someone tracking household expenses, I want the Payer to default to the one in
   Preferences, so that Rules behave like the add form I already use.
10. As someone tracking household expenses, I want to express an interval as "every n months",
    "every n weeks" or "every n days", so that quarterly, fortnightly and odd cycles are all
    expressible.
11. As someone tracking household expenses, I want to see each Rule's next charge date, so
    that I know what's coming and when.
12. As someone tracking household expenses, I want a single Monthly Equivalent total at the
    top of the page, so that I can answer "what do my subscriptions cost me" without doing
    arithmetic across four different interval units.
13. As someone tracking household expenses, I want the Monthly Equivalent converted into my
    Preferred Currency, so that Rules billed in different currencies can be summed at all.
14. As someone tracking household expenses, I want the Monthly Equivalent to hide rather than
    show a wrong number when Exchange Rates are unavailable, so that I'm never misled — the
    same way converted totals already behave elsewhere.
15. As someone tracking household expenses, I want disabled Rules excluded from the Monthly
    Equivalent, so that the figure reflects what I'm actually paying.
16. As someone tracking household expenses, I want each Rule to mint real Expenses, so that my
    Daily View, List View, Calendar View and Exports are complete without manual entry.
17. As someone tracking household expenses, I want exactly one charge materialized ahead of
    today, so that I can see what's coming in Upcoming without my ledger filling with
    speculative rows.
18. As someone tracking household expenses, I want the Pending Charge to appear in the
    Ledger's Upcoming section, so that the forward visibility I already have a place for
    finally has something in it.
19. As someone tracking household expenses, I want a Pending Charge to become ordinary history
    when its date passes, so that there is no state to manage and nothing to confirm.
20. As someone tracking household expenses, I want editing a Rule to rewrite its Pending
    Charge, so that a price rise is corrected in one place before it ever becomes history.
21. As someone tracking household expenses, I want editing a Rule to leave past Expenses
    untouched, so that a change today can never rewrite what I already recorded, reported or
    exported.
22. As someone tracking household expenses, I want a monthly Rule anchored on the 31st to
    charge on the 28th in February and the **31st** again in March, so that its dates match
    the statement I'm reconciling against.
23. As someone tracking household expenses, I want charge dates never computed by adding to
    the previously clamped date, so that one February can't walk the Rule backwards through
    the calendar forever.
24. As someone tracking household expenses, I want missed charges backfilled on their true
    dates when the app was down, so that an outage doesn't punch a hole in my record.
25. As someone tracking household expenses, I want to be told how many charges Catch-up added,
    so that Expenses never appear silently behind my back.
26. As someone tracking household expenses, I want to disable a Rule, so that I can pause a
    subscription without losing its configuration.
27. As someone tracking household expenses, I want disabling a Rule to remove its Pending
    Charge, so that Upcoming doesn't advertise a charge that isn't going to happen.
28. As someone tracking household expenses, I want re-enabling a Rule to resume from today
    rather than backfilling the paused stretch, so that "disabled" means disabled and not
    merely deferred.
29. As someone tracking household expenses, I want past Expenses from a Rule I disabled to
    remain, so that pausing something doesn't erase the months I did pay for.
30. As someone tracking household expenses, I want to set an optional end date on a Rule, so
    that a fixed-term installment stops minting Expenses once it's paid off.
31. As someone tracking household expenses, I want a Rule to disable itself when its next
    charge would fall past its end date, so that I don't have to remember to switch it off.
32. As someone tracking household expenses, I want to leave the end date blank, so that
    open-ended subscriptions behave as before.
33. As someone tracking household expenses, I want to delete a Rule entirely, so that I can
    remove something I never should have added.
34. As someone tracking household expenses, I want deleting a Rule to keep the Expenses it
    already created, so that removing a schedule doesn't destroy my spending history.
35. As someone tracking household expenses, I want deleting a Rule to remove its Pending
    Charge, so that no orphaned future Expense is left behind.
36. As someone tracking household expenses, I want the List View's total to stop counting
    charges that haven't happened yet, so that "total spent" means money actually spent.
37. As someone tracking household expenses, I want the List View's Category breakdown bounded
    the same way, so that the summary and the total agree with each other.
38. As someone tracking household expenses, I want charge dates evaluated in my configured
    Timezone, so that a charge due today doesn't appear a day early or late because of where
    the server is.
39. As someone tracking household expenses, I want generation to survive a restart without
    duplicating anything, so that rebooting the container is never risky.
40. As someone tracking household expenses, I want a Rule's Expenses to be indistinguishable
    from hand-entered ones in every existing view, so that nothing I already rely on has to
    learn a new concept.
41. As someone tracking household expenses, I want to edit a Pending Charge directly as an
    ordinary Expense, so that a one-off variation doesn't require changing the Rule.
42. As someone tracking household expenses, I want to reach the Recurring page from the app's
    navigation, so that I can find it without knowing its URL.

## Implementation Decisions

### The domain

- A new **Recurring Rule** aggregate is added to the domain, alongside Expense, User and
  Preferences. The UI calls the page "Recurring"; the type is a Rule because not everything it
  models is a subscription (rent and installments are Rules too).
- A **Pending Charge** is not a distinct type. It is an ordinary Expense that happens to be
  future-dated and linked to a Rule. There is no pending/confirmed state machine anywhere in
  the system — this was considered and explicitly rejected, because it would require every
  existing sum in the app to learn which Expenses "count".
- A Rule carries: name, amount, currency, category, payer (optional), interval unit, interval
  count, anchor day, next charge date, end date (optional), and enabled.
- The Rule's name **is** the Description of the Expenses it mints. No separate description
  field exists. Templated per-charge descriptions were rejected: descriptions are grouped and
  ranked by frequency to build the Description Suggestion pills, so unique per-charge strings
  would each rank once and degrade that feature.
- Currency and Payer default from Preferences, matching the add form's existing behaviour.

### Scheduling

- Interval unit is one of months, weeks or days, with a count of at least 1. Weeks and days are
  plain multiples of 24 hours; only months need calendar reasoning.
- **Anchor day is stored separately from the next charge date.** Each monthly occurrence is
  computed as `min(anchor day, days in that target month)`. A Rule anchored on the 31st runs
  Jan 31 → Feb 28 → Mar 31 → Apr 30 → May 31. February never poisons the series.
- `next_charge_date` is persisted so that "which Rules are due" stays a cheap indexed query,
  but it is **never authoritative** — it is always recomputed from the anchor, never by adding
  an interval to its own previous value. That distinction is the entire defence against date
  drift and is the single most important invariant in this effort.
- When the newly computed next charge date would fall after the Rule's end date, the Rule
  disables itself instead of scheduling.

### Generation

- Generation is owned by the Service layer and takes **the current date, in the user's
  Timezone, as an explicit argument**. It must never read the clock internally. This is a
  testability requirement first and a correctness requirement second: charge dates are
  Timezone-sensitive, and the Timezone lives in Preferences.
- Exactly one charge is materialized ahead of the supplied date, per enabled Rule.
- **Catch-up**: when the supplied date is far enough ahead that several charges came due,
  every one of them is minted on its own true date, in order, and the Rule advances past all
  of them. The count of minted Expenses is returned so the caller can surface it as a Flash.
- Catch-up applies only to elapsed real time, never to a period during which the Rule was
  disabled. Disabling stops the clock; re-enabling sets the next charge to the first anchor
  occurrence on or after today.
- Generation is driven by a single background goroutine: once at startup, then on a ticker,
  cancelled by context on shutdown — the same construction as the exchange-rate fetcher. The
  startup run is what closes gaps. Because only one goroutine ever generates, concurrent
  double-generation is impossible by construction rather than by locking. Lazy
  generate-on-page-render was rejected: it would make GET requests mutate the database and
  reintroduce the races the single-goroutine design eliminates.

### Persistence

- A `recurring_rules` table is added, plus a nullable recurring-rule reference on `expenses`,
  applied through the existing additive `pragma_table_info` migration pattern.
- A **uniqueness constraint on (rule reference, date)** makes double-generation structurally
  impossible, not merely unlikely. This is safe precisely because the minimum interval is one
  day, so a Rule can never legitimately owe two charges on the same date.
- **Deleting a Rule** follows the existing user-deletion precedent exactly: one transaction
  that clears the rule reference on the Rule's past Expenses, deletes its Pending Charge, and
  drops the Rule. Past Expenses survive as ordinary unlinked Expenses.
- **Editing a Rule** rewrites its Pending Charge — every field, not only the amount — and
  recomputes the pending date if the interval or anchor changed. Expenses whose date has
  passed are never touched by any Rule operation.

### The Monthly Equivalent

- Each enabled Rule is normalized to a per-month amount: every n months divides by n; every n
  weeks multiplies by 365.25 / 7 / 12 / n; every n days multiplies by 365.25 / 12 / n. The
  figure is presented as approximate, because day-based intervals genuinely do not divide into
  months.
- Each normalized amount is converted into the Preferred Currency using the existing
  conversion path, then summed. When rates are unavailable the figure is hidden rather than
  shown wrong, reusing the same convention that governs converted totals elsewhere.
- Disabled Rules are excluded.

### Existing behaviour that changes

- The **List View's total and Category breakdown become bounded to today**, excluding
  future-dated Expenses. Without this, every enabled Rule would permanently overstate the
  List View headline by one charge. This is a deliberate, spec'd change to an existing page
  rather than a silent side effect, and it also fixes the same latent distortion for
  hand-entered future-dated Expenses.

### Presentation

- The Recurring page is server-rendered: a list plus ordinary CRUD forms, following the shape
  of the existing user-management pages. **No Island is introduced** — ADR 0001's bar for
  hydration is not met by a form-and-list page, and the Export effort already established that
  a form plus a link needs no Island.
- The page is reachable from the app's existing navigation.

## Testing Decisions

A good test here observes behaviour at a public interface and says nothing about how the
result was reached. Concretely: a test asserts *which Expenses exist after generation runs*,
never how the scheduler looped, what intermediate dates it considered, or which helper it
called. Date logic is the substance of this feature, so tests must be able to drive time
freely — that is why the current date is an argument rather than a clock read.

**Primary seam (new) — the Service layer's recurring API, with the current date supplied by
the caller.** Nearly the whole design is observable here, and it is the highest seam that can
observe generation at all, since generation is triggered by a goroutine rather than by HTTP:

- one charge materialized ahead, and no more, per enabled Rule
- the anchor series Jan 31 → Feb 28 → **Mar 31** → Apr 30, which is the assertion that proves
  dates are recomputed from the anchor rather than accumulated
- Catch-up minting every missed charge on its true date and returning the count
- disable removing the Pending Charge; re-enable resuming from today with no backfill
- end date causing self-disable instead of scheduling
- editing a Rule rewriting the Pending Charge and never touching past Expenses
- the Monthly Equivalent's normalization, currency conversion, exclusion of disabled Rules,
  and its hidden-not-wrong behaviour when rates are missing

Prior art: the existing service tests, which already inject a rate-fetcher double and drive
conversion behaviour through it.

**Existing seam — HTML handler tests** for the Recurring page rendering, its CRUD forms and
redirects, and the List View bounding change. Prior art: the existing HTML handler and export
handler tests, which drive real requests through the router against a mock repository.

**Existing seam — repository tests over in-memory SQLite** for the migration, the uniqueness
constraint that blocks double-generation, and the transactional Rule deletion that preserves
past Expenses while removing the Pending Charge. These are guarantees enforced by SQL, which a
mock repository cannot demonstrate; the existing expense and rate repository tests are the
prior art and already migrate a fresh in-memory database per test.

No frontend tests are added, because no Island is introduced.

## Out of Scope

- **A pending/confirmed review step.** Charges become real Expenses immediately; there is no
  approval queue. Rejected during grilling as duplicating state that every existing sum would
  then have to interpret.
- **Rules owning their whole history.** A Rule can never rewrite or delete an Expense whose
  date has passed, so editing a price never alters a month you already exported.
- **Materializing a long horizon.** No 30- or 60-day forecast window; exactly one charge ahead.
- **Tracking actual charged amounts against expected ones.** A Rule states what it expects to
  charge; reconciling that against what a bank actually billed is a different feature.
- **Variable or rule-driven amounts** (percentage rises, indexed amounts, per-occurrence
  overrides). An amount changes when a human edits the Rule.
- **Reminders or notifications** of any kind — no email, no push, no alerting on an upcoming
  charge. The Ledger's Upcoming section is the entire notification surface.
- **Importing Rules** from bank statements or existing Expenses, including any "we noticed
  this looks recurring" detection.
- **The `/api/summary` mixed-currency defect** — it sums raw amounts across currencies with no
  conversion and no date bound. Real, and worth fixing, but independent of this effort and not
  caused by it.
- **Migrating money from float to integer minor units** — a pre-existing modelling choice this
  effort inherits and does not relitigate, exactly as the Export effort declined to.
- **Multi-currency Monthly Equivalent breakdowns** — one total in the Preferred Currency, not a
  per-currency subtotal table.

## Further Notes

The strongest argument for this feature is not convenience but *completeness*: an expense
tracker whose totals silently omit rent and subscriptions is answering a different question
than the one being asked of it. That framing is what drove the generator decision in the first
question of the grilling, and it is worth re-reading before anyone proposes softening it into
a reference list.

Two decisions carry nearly all the risk of getting this wrong, and both are cheap to protect
with tests written first:

1. **Recomputing dates from the anchor rather than accumulating them.** The failure is silent
   and slow — a Rule drifts a few days earlier per year and nobody notices until it no longer
   matches a statement. The Jan 31 → Feb 28 → Mar 31 assertion is the canary.
2. **Catch-up firing for outages but not for deliberate pauses.** These two rules meet at
   re-enable, and the wrong answer there dumps months of invented Expenses into the ledger the
   moment someone un-pauses something they paused in the spring.

The pleasant structural consequence of the design is how little it disturbs: because a Pending
Charge is just a future-dated Expense, the Ledger, the Calendar View and Export all display
and include Rules' output without a line of change. The only existing behaviour this effort
touches at all is the List View's total — and it touches that to make it more correct than it
is today.
