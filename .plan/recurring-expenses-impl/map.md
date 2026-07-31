# Recurring Expenses — implementation

## Destination

The recurring-expenses spec ([`.plan/recurring-expenses/spec.md`](../recurring-expenses/spec.md))
implemented end to end: a Recurring page lists every Recurring Rule with a Monthly Equivalent
total, each enabled Rule mints real Expenses on its schedule — one charge materialized ahead,
visible in the Ledger's Upcoming — a background goroutine generates and backfills them without
duplication, and the List View's total no longer counts charges that haven't happened. All the
spec's named tests pass, and the architecture docs reflect what shipped.

## Notes

**This map carries execution.** Every ticket is a `task` that delivers working code, not a
decision — all decisions were made in a grilling session and synthesized into the
[spec](../recurring-expenses/spec.md), which is the single source of truth here. There was no
planning map for this effort; the interview did that map's job. Do not re-litigate a decision;
if implementation exposes one as wrong or missing, **raise it rather than quietly deviating** —
this map has no planning ticket to mark undermined, so it comes back to the human.

Every session: read the spec in full, then this map, then your ticket. Use `CONTEXT.md`'s
vocabulary — this effort adds **Recurring Rule**, **Pending Charge**, **Anchor Day**,
**Catch-up** and **Monthly Equivalent** to it, and the final ticket is where they land. Respect
ADR 0001: this effort introduces **no Island**, and a ticket that finds itself wanting one has
found a decision to raise, not a liberty to take. The `review-code` skill (in `.pocock-skills/`)
reviews the diff before commit; the `run` skill drives the real app.

**The central invariant, worth reading twice:** charge dates are recomputed from a stored anchor
day, never accumulated from the previous date. Jan 31 → Feb 28 → **Mar 31**, not Mar 28. The
failure this prevents is silent and slow, and no test but the anchor series will catch it.

Per-ticket checks: `go test ./...`, `go vet ./...`. No TypeScript is expected to change; run
`cd ui && npx tsc --noEmit` and `cd ui && npx vitest run` only if it does. Tests land **before**
implementation where the spec's Testing Decisions say so — the leading red is the anchor series
in [The rule, its store, and the anchor arithmetic](./tickets/01-rule-store-and-anchor-arithmetic.md).

Time is an argument, never a clock read. Generation takes the current date, in the user's
Timezone, from its caller. A ticket that reaches for `time.Now()` inside the service has made
the whole effort untestable — the only place that reads the real clock is the goroutine in
[Live generation](./tickets/05-live-generation-goroutine-and-flash.md).

Environment: `make dev` runs air (:8080) + Vite (:8081); the Go server caches templates at
startup, so restart after template edits. **The user's dev database (`~/.expensif/expenses.db`)
must not be seeded or generated into** — a bug in generation writes real rows to it. Work
against a scratch DB via `DATA_DIR=<tmp> PORT=8090 go run ./cmd/server`, which also gives you a
database whose clock you can reason about. The wayfinder linter for this map:
`cd ../wayfinder && go run ./cmd/wayfinder-maps lint ../expensif/.plan/recurring-expenses-impl`.

## Decisions so far

<!-- one line per resolved ticket: gist + link. Empty until the first ticket resolves. -->

- **The Rule is stored and the anchor arithmetic is right.** `domain.RecurringRule` +
  `recurring_rules` + a nullable `recurring_rule_id` on expenses, with a *partial* unique index
  on `(recurring_rule_id, date)`; the schedule is `Service.NextChargeDate(rule, after)`, which
  takes the month from the date it advances from and the day from the stored anchor, so Jan 31 →
  Feb 28 → **Mar 31**. Verified red against a drifting implementation first. Three things later
  tickets inherit: the rule API takes a `domain.RecurringRule` value rather than flat arguments;
  the payer is **not** defaulted in the service, so ticket 04's form handler must pre-select it
  the way `HandleAdd` does; and `Enabled` is never defaulted, so that form must send it.
  — [01](./tickets/01-rule-store-and-anchor-arithmetic.md)
- **Rules mint expenses.** `Service.GenerateCharges(ctx, today)` walks each enabled rule forward
  from its stored date, minting every charge on its own true date until it lays down the one
  after `today` — so catch-up is not a separate mode — and returns how many Expenses it created.
  Idempotence is derived from the data, not from a flag: `ExpenseRepository.MintCharge` inserts
  `ON CONFLICT … DO NOTHING` against ticket 01's partial unique index, so a repeated run creates
  nothing and never overwrites an edited charge. Three things later tickets inherit: a rule's
  `NextChargeDate` *is* its Pending Charge's date (not the one after it); the returned count
  includes the pending charge as well as the backfilled ones, so ticket 05's Flash wording must
  fit that; and a self-disabled rule keeps the computed date that overran its end date, so
  ticket 04 should not render a next-charge date for a disabled rule unquestioned. The Ledger
  and the CSV export were proven byte-identical with and without the rule reference — no
  existing view changed.
  — [02](./tickets/02-generation-catch-up-and-end-date.md)
- **The future belongs to the Rule; today and the past belong to the ledger.**
  `Service.UpdateRule(ctx, rule, today)` rewrites the one strictly future Pending Charge in
  full and recomputes its date from the prior occurrence when the schedule changes;
  `SetRuleEnabled(ctx, id, enabled, today)` removes that future on pause and resumes at the
  first anchored occurrence on or after today without Catch-up. A direct ordinary-Expense date
  edit moves the Rule's `NextChargeDate` too, so generation and later Rule operations keep
  owning the same charge. `DeleteRule(ctx, id, today)` is one SQLite transaction: delete the
  strictly future linked Expense, clear the reference from everything that survives, then drop
  the Rule. Ticket 04 must supply `today` in the configured Timezone to edit/toggle/delete; a
  charge dated exactly today is ledger history and must not be rewritten or removed.
  — [03](./tickets/03-what-a-rule-owns.md)
- **Generation is live, and a goroutine can raise a Flash.** `web.Generator` runs
  `GenerateCharges` once at startup and then **hourly**, on the context that already stops the
  rate refresher, dating charges in the Preferences Timezone read fresh every pass. It is the
  app's only caller, so double-generation stays impossible by construction; a failing run is
  logged and the next tick retries. The thin seam is settled as **`web.Notice`**: an in-memory,
  mutex-guarded count the generator adds to and the next *whole page* consumes exactly once
  through the existing Flash plumbing — fragments never consume it, and a Flash the page already
  set (an error the human caused) outranks it and leaves the notice for the page after. Two
  things later tickets inherit: the Flash fires for *every* mint, worded "N recurring charges
  added", so a quieter Catch-up-only message means splitting the count in the service; and a rule
  created on the Recurring page has no Pending Charge until the next tick (≤1h), which ticket 04
  must live with rather than generating from a handler.
  — [05](./tickets/05-live-generation-goroutine-and-flash.md)

## Not yet specified

<!-- Empty. Every decision is settled in the spec; this map only executes it. Nothing is
foggy — a ticket that exposes a genuinely new question raises it with the human, it does not
open fog here. The one seam the spec left thin is named inside its own ticket rather than
here: how a Flash raised by a background goroutine reaches the next rendered page. -->

## Out of scope

<!-- Inherited from the spec's Out of Scope; these never graduate into tickets on this map. -->

- **A pending/confirmed review step** — charges become real Expenses immediately; no approval
  queue, and no existing sum has to learn which Expenses count.
- **Rules owning their whole history** — a Rule can never rewrite or delete an Expense whose
  date has passed.
- **A long materialization horizon** — exactly one charge ahead, never a 30- or 60-day forecast.
- **Reconciling expected against actual charged amounts** — a different feature entirely.
- **Variable or rule-driven amounts** (percentage rises, indexed amounts, per-occurrence
  overrides). An amount changes when a human edits the Rule.
- **Reminders or notifications** of any kind. The Ledger's Upcoming section is the whole
  notification surface.
- **Detecting recurring patterns** in existing Expenses or imported statements.
- **The `/api/summary` mixed-currency defect** — it sums raw amounts across currencies with no
  conversion and no date bound. Real and worth fixing, but independent of this effort and not
  caused by it.
- **Migrating money from `float64` to integer minor units** — a pre-existing modelling choice
  this effort inherits, exactly as the Export effort declined to relitigate it.
