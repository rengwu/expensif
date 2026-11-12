---
type: task
blocked_by: [03]
claimed_by: s5e91e5a8c71b
claimed_at: 2026-08-01T18:12:36Z
---

# The Recurring page and the Monthly Equivalent

## Question

Give the effort a face. Everything the page drives already exists behind the service by now, so
this ticket is presentation plus one calculation.

Build:

- **`GET /recurring`** — a server-rendered list of every Rule showing name, next charge amount
  and date, interval, and enabled state, ordered by next charge date. Disabled Rules are
  visually distinct and sort after the enabled ones. Follow the shape of the existing
  user-management pages; they are the closest prior art in the codebase for list-plus-forms.

- **The CRUD forms** — new, edit, delete, and an enable/disable toggle — driving the service
  operations from [What a rule owns](./03-what-a-rule-owns.md). Reuse the existing form
  conventions: shared partials where they fit, flash messages for success and failure, and form
  values preserved on a validation error the way the expense form already preserves them.

- **The Monthly Equivalent**, one figure at the top of the page. Each enabled Rule normalizes to
  a per-month amount — every n months divides by n, every n weeks multiplies by 365.25 / 7 / 12 / n,
  every n days multiplies by 365.25 / 12 / n — then converts into the Preferred Currency through
  the existing conversion path, and the results sum. Present it as approximate; day-based
  intervals genuinely do not divide into months, and the `≈` is honest rather than decorative.
  Disabled Rules are excluded.

  **When rates are unavailable the figure hides rather than showing a wrong number**, reusing
  the same convention that already governs converted totals elsewhere in the app. A Rule already
  in the Preferred Currency needs no conversion and must still count when rates are missing —
  decide deliberately whether a partially-convertible set shows a partial total or hides
  entirely, and say which in the answer.

- **A navigation entry**, so the page is reachable without knowing its URL.

**No Island.** This is a list and some forms; ADR 0001's bar for hydration is not met, and the
Export effort already established that a form plus a link needs no Island. If this ticket finds
itself wanting one, that is a decision to raise with the human, not a liberty to take.

Tested at the existing HTML handler seam — real requests through the router against a mock
repository, the way the existing HTML handler and export handler tests work. Assert status,
redirects, and that the page renders the Rules it was given; assert the Monthly Equivalent's
value and its hidden-when-rates-are-missing behaviour. Do not assert markup structure.

**Done when:** Rules can be created, edited, disabled, re-enabled and deleted entirely through
the page; the Monthly Equivalent is correct across mixed intervals and currencies and hides
when it cannot be trusted; the page is reachable from the navigation; and no TypeScript changed.

## Answer

The effort has a face: `GET /recurring`, server-rendered, **no Island** — no `data-island`
container, no entry script, no TypeScript touched at all.

- **The list.** Every Rule, soonest charge first (the repository's own order), with disabled
  ones sorted after every enabled one by a stable sort in the handler. A paused Rule is muted,
  badged *Paused*, and — taking ticket 02's warning seriously — **shows no next-charge date at
  all**: its stored date is a stopped clock and may be the one that overran its end date. It
  reads "No charges while paused" instead. Each row carries name, amount in its own currency,
  interval in words ("Every 2 weeks"), category, payer and end date.
- **The forms.** `templates/recurring.html` + `recurring_form.html`, following the
  `users.html`/`user_form.html` shape, with one form serving new and edit. Create, edit,
  pause/resume and delete all drive ticket 03's service operations, each supplied `today` in
  the Preferences Timezone from `basePageData`. Delete confirms first and says that expenses
  already created are kept. Values typed into a rejected form come back with the error, as the
  expense form does.
- **The Anchor Day is carried by the edit form**, visible for a monthly Rule and hidden for a
  week/day one. This matters more than it looks: `normalizeRule` re-derives a missing anchor
  from the stored charge date, so a form that dropped the field would silently re-anchor a
  31st Rule to the 28th the first time February clamped it — the exact drift this effort
  exists to prevent. The create form asks only "when does it first charge?", and the anchor
  comes from that date, as ticket 01 intended.
- **The Monthly Equivalent.** `Service.MonthlyEquivalent(ctx, rules, target)` — months divide
  by n, weeks and days scale by the mean 30.44-day month — converting through
  `GetRatesForConversion`/`ConvertWithRates`, summing, excluding disabled Rules, rendered as
  `≈ $257.41 / month`. It takes the rules rather than fetching them, so the figure is provably
  about the Rules on screen.

**The partial-total decision, made deliberately: a set with even one unconvertible Rule hides
entirely.** A Rule already in the Preferred Currency needs no rates and still counts, so an
account billed only in its own currency always gets a figure — but dropping one foreign Rule
from a single opaque number understates committed spending with nothing on screen to say so,
which is precisely the wrong number the spec says to avoid. (`ConvertExpensesTotal` skips
unconvertible rows instead; it totals rows the reader can see and count, and this does not.)

**Done-when, clause by clause.** `TestRecurringPage_CreateEditPauseResumeDelete` drives
create → edit → pause → resume → delete entirely through the page's own forms, reading the
store back after each; `TestRecurringPage_MonthlyEquivalentAcrossIntervalsAndCurrencies` pins
the figure across two currencies and two intervals with a paused Rule excluded, and
`..._HidesWhenARuleCannotBeConverted` / `..._SurvivesMissingRates` pin both halves of the
hidden-not-wrong rule; `TestRecurringPage_ReachableFromTheNavigation` asserts the entry from
the Ledger, since it lives in the shared layout (desktop dropdown and mobile-nav props alike);
and `git status` shows no `ui/` change, so the TypeScript checks were not run. The handler
tests go through the real mux, so routing and path values are exercised too. Verified in the
running app against a scratch DB: rules created through the page were minted by the generator
on the next startup and the Flash said so.

**Three judgement calls a reader should know about.**

1. **Success flashes cross the redirect as a whitelisted code** (`/recurring?flash=created`),
   mapped to a fixed message server-side — no URL text ever reaches the page. The repo had no
   post-redirect success flash to copy (the user pages redirect silently), and the ticket asked
   for flashes on success as well as failure. Failures follow the existing convention exactly:
   the form re-renders with the error, and pause/delete — which have no form — re-render the
   list with it.
2. **The currency `<select>` became `partials/currency-select.html`**, drawn by both the
   expense form and the rule form, rather than a second copy of seventeen options.
3. **`NewServer` now builds its mux in a `routes()` function**, so the tests drive the real
   routing table instead of a hand-rolled copy of it.

**Deliberately left out:** the List View's bounding (06) and the docs/glossary updates (07).
Generation is not called from any handler — a Rule created here has no Pending Charge until
the goroutine's next pass, which ticket 05 settled and the form says out loud ("A new rule's
first charge appears in the ledger within the hour"). Nothing in the spec was found wrong, so
nothing was raised.
