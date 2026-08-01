---
type: task
blocked_by: [04, 05, 06]
claimed_by: sd4c2f970daae
claimed_at: 2026-08-01T19:27:48Z
---

# Docs and architecture catch up with what shipped

## Question

The map's last ticket. Make the documentation describe what was actually built, then leave the
repo in a state where a fresh agent gets an accurate picture without reading this map.

Do:

- **`CONTEXT.md`** gains the vocabulary this effort introduced: **Recurring Rule**, **Pending
  Charge**, **Anchor Day**, **Catch-up**, and **Monthly Equivalent**. Follow the file's existing
  conventions — a plain user-facing definition, an `_Avoid_` line where a tempting wrong synonym
  exists — and place them near the views they relate to. Two are worth stating carefully because
  they are the effort's load-bearing ideas: a **Pending Charge** is an ordinary future-dated
  Expense and *not* a distinct type or state, and **Catch-up** covers time the app was absent
  and never a stretch a human deliberately paused. Related terms can ride inside an entry rather
  than earning their own, the way **Payer** carries `paidBy`.

- **Assess whether an ADR is warranted, and be willing to conclude it isn't.** This effort adds
  no Island and introduces no new architectural pattern — the generation goroutine deliberately
  mirrors the existing rate fetcher rather than inventing anything. The likely right outcome is
  no new ADR, possibly a note on an existing one. Write a new ADR only if something genuinely
  architectural was decided that a future reader would otherwise re-litigate; say plainly in the
  answer which way you went and why.

- **`AGENTS.md`**'s `.plan/` inventory was updated when this map was charted, so it should
  already name this effort — verify it still describes it accurately, and check nothing else in
  that file drifted. Do not restate how far along the map is or how many tickets are resolved —
  that is derived from the tickets, and a count written down is true for a week and wrong
  forever.

- **Sweep for drift.** Look for comments, templates or ticket-era TODOs that describe shipped
  work as pending ("until generation ships", "ticket 02 supplies…"), and rewrite them in the
  present tense. Check for dead code, unused template partials, and anything the spec described
  that did not survive contact with implementation.

- **Flag rather than fix** anything you find that contradicts the spec or looks like a decision
  quietly deviated from during implementation. This map has no planning tickets to mark
  undermined, so a contradiction comes back to the human — name it in the answer.

Docs-only work still runs the checks: `go vet ./...` and `go test ./...` must be green, and if a
template was touched, load the affected pages to prove they still parse — the Go server caches
templates at startup, so restart before checking.

**Done when:** `CONTEXT.md` carries the new vocabulary, the ADR question is decided either way
with reasoning, `AGENTS.md` describes this effort without stating progress, the drift sweep is
done, and anything contradicting the spec is flagged for the human rather than silently
corrected.

## Answer

**The docs describe what shipped, and the ADR question came back "no new ADR" — the effort really
did introduce no new architectural pattern.** Everything in the diff is markdown plus one Go
comment, so the suite stayed green untouched: `go vet ./...` clean, `go test ./...` green
(repository, service, web), `gofmt` clean. No template changed, so no page needed re-parsing and
the UI checks were not run; `git status` shows no `ui/` change.

### CONTEXT.md — five terms, placed with the views, plus one correction

The five land as a block after **Export Preview**, where the page/view cluster ends, following the
file's conventions: bold term, a plain user-facing definition on a single unwrapped line, and an
`_Avoid_` line. The Recurring page itself rides *inside* the **Recurring Rule** entry rather than
earning a sixth term, the way **Payer** carries `paidBy`.

The two load-bearing ones are stated as the ticket asked. **Pending Charge** says outright that it
is an ordinary future-dated Expense and **not** a distinct type or state — "nothing in the system
asks whether an Expense is pending, there is no approval step" — and its `_Avoid_` list names the
four synonyms (pending transaction, scheduled expense, draft, provisional) that would each smuggle
back the state machine the spec rejected. **Catch-up** says it covers time the *app* was absent and
never a stretch a human deliberately paused, with the disable/re-enable rule spelled out beside it.
**Anchor Day** carries the Jan 31 → Feb 28 → **Mar 31** series and the never-accumulated rule.

A **Recurring Rule vs. Expense** note joins the Cross-Context section: a Rule states what it
*expects* to charge, the Expenses it mints are the record of what happened, and a Rule owns only the
future. That is the distinction the whole ownership model rests on and the section's existing shape
(Date vs. Time, User vs. Payer) is exactly where it belongs.

**One existing entry was corrected, because this effort made it false.** **List View** read "A page
showing all expenses in a single table" — ticket 06 bounded rows, total and breakdown to today, so
"all" is no longer true. It now states the bound and where future-dated expenses remain visible.
This is drift the sweep was for, not a new decision.

### The ADR — assessed, and the answer is no new ADR

Nothing genuinely architectural was decided. The candidates and why each fails the bar:

- **No island on the Recurring page** — that is ADR 0001 *applied*, not amended. The existing
  decision already says most of the UI needs no client-side state.
- **The generation goroutine** — deliberately the exchange-rate fetcher's shape, copied rather than
  improved on. Prior art in the same codebase is the opposite of a new pattern.
- **The `web.Notice` seam** — ~50 lines in one package, with its rejected alternatives already
  written into `notice.go`'s doc comment. A future reader questioning it reads the code and finds
  the reasoning there.

Measured against the one ADR this repo has — a foundational frontend choice whose reversal "would
require rebuilding routing, state management, and data loading on the client" — none of these are
close. An ADR per effort would dilute the directory into a changelog.

**What it got instead: a short note appended to ADR 0001**, which the ticket sanctioned. It records
the Recurring page as the clearest measure of where the hydration bar sits — a whole feature surface
(list, create/edit forms, pause/resume, delete, a computed figure) with zero client-side state and
one inline `confirm()` — so a future reader who assumes a page that interactive must hydrate
something reads that as the bar being high rather than as an oversight. A second paragraph names the
goroutine and the Notice as things this effort did *not* change architecturally, and points at where
their reasoning lives, so the absence of an ADR is itself recorded rather than looking like a gap. It
sits under its own `## Note (recurring-expenses effort)` heading rather than extending the
daily-timeline/export-csv one, because that note tracks a different axis (which islands are vanilla
TS) and this one is about earning no island at all.

### AGENTS.md — the inventory was already right; one Layout line was not

The `.plan/` inventory names `.plan/recurring-expenses/` + `.plan/recurring-expenses-impl/`
accurately, and the sentence about an effort being planned on a `<slug>` map "or, where a grilling
session settles every decision outright, a `<slug>/spec.md` with no map at all" describes this
effort exactly — verified against the directory, which has a `spec.md` and no `map.md`. No progress
or ticket count is stated anywhere, and none was added. Nothing else in the file had drifted: the
commands table, the skills tables and the `.pocock-skills/` description all still hold, and the
handoffs paragraph is still true (dated files and the `session-*.md` archive both exist).

**One edit, and it is a judgement call worth naming.** The Layout line described `internal/web/` as
"HTTP handlers and templates"; it now also holds a background goroutine that writes to the database
without a request, which a fresh agent looking for "where does generation live" would never guess
from that description. It now reads "HTTP handlers and templates, plus the background goroutine that
generates recurring charges". The export-csv docs ticket set a precedent of *not* touching Layout —
but its reason was "Export adds handlers/templates within `internal/web`, not a new package", and
that reason genuinely does not cover a goroutine. If a human reads the precedent as absolute, this
is a one-hunk revert and nothing depends on it.

### The drift sweep — one comment, and otherwise clean

Grepped the Go, template and TS sources for ticket references, `TODO`/`FIXME`, and future-tense
phrasing ("until … ships", "will be", "is expected to", "a later ticket"):

- **Fixed:** `internal/service/recurring.go`'s `normalizeRule` doc said the payer is not defaulted
  "and the Recurring form **is expected to** do the same" — written in ticket 01 when no form
  existed. It now says the form does the same, in `HandleRecurringNew`.
- **Left alone:** `templates/export.html`'s "built against … the ticket 05 markup" is provenance
  that is still true, which the export docs ticket already ruled on.
- **No dead code.** Every repository method the effort added has non-test callers
  (`MintCharge`, `RewritePendingCharge`, `DeletePendingCharge`, `SumMatching`, and the five rule
  methods). Every template partial is referenced, including the new `partials/currency-select.html`
  (expense form and rule form). No orphaned templates, no stray island registration — `git status`
  is clean and `ui/` was never touched by this effort.
- **`Service.NextChargeDate` is exported but called only by tests** (production paths use the
  unexported `nextCharge`). Deliberately kept: it is the public scheduling seam ticket 01 was asked
  for and the spec's Testing Decisions name the service's recurring API as the primary seam — it is
  the anchor-series canary's entry point, not dead code.
- **Everything the spec named is covered by tests.** Checked each bullet of the spec's Testing
  Decisions against the test names across `internal/service`, `internal/repository` and
  `internal/web`: one-ahead materialization, the anchor series, Catch-up's count, disable/re-enable,
  the end date, edit-rewrites-pending, and all four Monthly Equivalent behaviours are each present,
  as are the SQL-level migration, uniqueness and transactional-delete tests.
- **Pre-existing, unrelated, not touched:** `go.mod` lists `github.com/dustin/go-humanize` as
  `// indirect` while `internal/web/renderer.go` imports it directly; `go mod tidy` would fix it.
  It dates from the MVP commit, long before this effort, and editing `go.mod` is outside a docs
  ticket's remit.

### Nothing contradicts the spec — and three doubts a human still owns

I looked specifically for decisions quietly deviated from, and found none. Two places where
implementation reads differently from the spec's letter are both *recorded* deviations, argued in
their tickets' answers rather than slipped in, so they are not flags:

1. The spec says "Currency and Payer default from Preferences"; the *service* defaults only
   currency, and the *handler* pre-selects the payer the way `HandleAdd` does. User-facing behaviour
   matches the spec exactly. Raised and settled on tickets 01 and 04.
2. User story 11 wants each Rule's next charge date; a **paused** Rule shows "No charges while
   paused" instead, because its stored date is a stopped clock. Argued in ticket 04.

**What a human should still rule on** — these are open doubts scattered across tickets 04–06, and
since this is the map's last ticket they are gathered here rather than left three answers deep:

1. **The Flash fires for every mint, not only for Catch-up** (ticket 05, doubt 1). Wording is "N
   recurring charges added", covering backfilled charges and the newly materialized Pending Charge
   alike. User story 25 asks to be told "how many charges **Catch-up** added". Steady state is quiet
   — one message per charge cycle per rule — but if that is too noisy, the fix is to have generation
   report the two counts separately and flash only the backfilled one. **I wrote the CONTEXT.md
   Catch-up entry to describe what actually ships** ("the ones it backfilled and the newly
   materialized Pending Charge alike") rather than the narrower story, so the glossary does not
   quietly paper over this.
2. **A newly created Rule has no Pending Charge until the next tick** (≤1 hour) (ticket 05, doubt 2;
   ticket 04 lived with it, and the create form says so out loud). Closing the gap means either
   generating from a handler — which breaks the one-goroutine property — or a nudge channel to the
   generator. A decision, not a liberty.
3. **The List View's empty state can read "No expenses yet." when every Expense is future-dated**
   (ticket 06, doubt 1) — realistically only a fresh install whose first Rule has just minted. The
   clarifier line sits above it.

**Deliberately left out:** no handoff was written (that is the `handoff` skill's job, user-invoked,
and not in this ticket's Done-when); no `.plan/expensif/` project spec was updated, since that is an
earlier effort's planning memory rather than live documentation; and `go mod tidy` was not run.
