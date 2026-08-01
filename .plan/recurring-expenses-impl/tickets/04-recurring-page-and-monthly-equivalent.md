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
