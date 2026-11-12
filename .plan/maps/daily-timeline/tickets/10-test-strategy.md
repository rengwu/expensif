---
type: grilling
blocked_by: [04]
---

# Test strategy for the date-indexed timeline

## Question

What is tested, at which seam, and what is the first test written?

Graduated from the map's fog once [Re-shape DailyGroups around dates](./04-date-indexed-daily-groups.md)
fixed the query shape. It could not be specified before that: the seam under test did
not exist.

The daily view has **zero test coverage today** — nothing in `internal/web/*_test.go`
or `internal/service/service_test.go` touches `daily` or `DailyGroups`. Ticket 04
deletes `DailyGroups(ctx, limit)` outright, so there is nothing to characterize; the
tests are written against the new interface, not the old one.

Read `.plan/expensif/testing-strategy.md` before deciding anything.

Resolve, one at a time:

- **Which seam.** `DailyGroupsInRange` is a service call over a repository interface
  that `service_test.go:328` already stubs (`expenseRepoStub.ListExpensesInRange`).
  Is the service the test surface, or does `HandleDaily` get exercised through
  `mock_repo_test.go` as well? Ticket 04's answer says the interface is the test
  surface — does that settle it, or does the handler's currency loop want its own test?
- **The gap-filling cases.** Empty window, window with expenses only on the first and
  last day, a single day, `start > end`. What is the contract when `start > end` — an
  error, or an empty slice?
- **The `Expenses` is never nil invariant.** Ticket 04 makes this a promise the island
  depends on. Is it asserted once, or on every gap-filled day in every test?
- **The DST/timezone boundary.** Ticket 04 argues no window edge can straddle a DST
  transition, because the walk runs on UTC-parsed dates and the timezone is consulted
  once to name today. Is that argument tested, and if so where — the walk is
  timezone-free, so the only thing left to test lives in the handler.
- **`UpcomingGroups`.** Ungapped, descending, dated strictly after today. Does the
  boundary (an expense dated exactly today) have a test?
- **Where the first test goes**, and whether it lands before or with the implementation.

## Answer

Two test surfaces, and the second one exists only because [ticket 06](./06-day-card-chrome-drift.md)
moved the island's contract into markup.

### The strategy doc is wrong on two points, and both were checked

`.plan/expensif/testing-strategy.md` is a dated discussion (2026-06-02) and is **not
edited** — but this effort does not obey it, and here is why:

- It says HTML handler tests are **blocked** on extracting a `TemplateRenderer`
  interface, because `render()` reads templates from disk. They are not.
  `internal/web/server_test.go:45` already builds a real renderer in-package —
  `NewRenderer("../../templates", false, nil)` — and hands it to `NewHTMLHandler`. The
  prescribed refactor buys nothing here and is **not done**.
- It says **never assert on template HTML output** ("if you change a `div` to a `span`,
  tests break"). That rule was written when every table on the daily view was a React
  island fed JSON. After ticket 06 the older-days endpoint *serves HTML*, and the
  cursor (`data-next-*`) and the foot's four states (`data-state`) are produced by the
  template and by nothing else. The rule is **narrowed, not broken**: chrome, classes
  and copy stay untested; the machine-readable contract gets covered, because a typo
  there kills pagination silently.

### Seam 1 — the service, for all logic

`DailyGroupsInRange` and `UpcomingGroups` against the repository interface, which
`service_test.go:328` already stubs. `expenseRepoStub` (`service_test.go:313`) grows a
`listInRange func(start, end string) ([]domain.Expense, error)` field, mirroring its
existing `create` field — no second stub is invented.

The handler's currency loop gets **no test of its own**. Ticket 04 verified by reading
`handlers_html.go:269-286` that an empty group converts to `0` with no guard; there is
no branch to cover.

### Seam 2 — the fragment, for the contract only

`GET /daily/older` through the real renderer (`httptest`, the `server_test.go` shape),
asserting **only** on `data-next-*` and `data-state`. Never on markup, class names, or
copy. If a test would break when a `div` becomes a `span`, it is the wrong test.

### `start > end` is an error, not an empty slice

The service validates its own range. Unparseable endpoints reuse ticket 09's
`ErrInvalidDate`; an inverted range returns a new `ErrInvalidRange`. `HandleDaily` and
`/daily/older` both map these to **400**, matching the write path's precedent.

An empty slice was rejected on a concrete failure: `/daily/older?start=&end=` takes its
range from user-controlled query params, so an inverted range that returned zero days
would reach the island as an empty fragment — **indistinguishable from reaching the
earliest expense**. Infinite scroll would stop silently. Garbage in, terminal state out.

Tests: the service returns the error; the handler returns 400.

### `Expenses` is never nil — survives, on a new rationale

**Ticket 04's justification for this invariant is void.** It said non-nil "marshals to
`"expenses": []` rather than `null` and ticket 07's island can `.map` over it with no
null guard." There is no JSON — ticket 06 replaced the DTO with an HTML fragment, and
`{{range .Expenses}}` iterates a nil slice happily. Nothing in the code today would
notice a nil.

The invariant stands anyway: a nil-vs-empty distinction that carries no meaning is a
latent bug (the next `!= nil` check, the next JSON endpoint — and ticket 07's island
decision has already flip-flopped once). It is asserted in **one** dedicated test that
names it, not as a line in every table case; an invariant asserted everywhere reads as
an accident asserted everywhere.

[Ticket 04](./04-date-indexed-daily-groups.md) is marked `undermined_by: [06]` for this.

### The timezone test needs a clock seam; the DST argument needs nothing

`nowInTZ` (`handlers_html.go:49`) calls `time.Now()` directly, so **no test can control
what "today" is** — and "anchor at today" is the assumption the whole map rests on.

The implementation adds `now func() time.Time` to `HTMLHandler`, defaulted to `time.Now`
in `NewHTMLHandler`. One test freezes it at `2026-07-11T23:00Z` with
`tz=Pacific/Auckland` and asserts the window ends **2026-07-12**, not the 11th. Without
it, a server in UTC serving a user in NZ ends the timeline yesterday and drops today's
row entirely, and no test can see it.

Ticket 04's DST claim — the walk parses to UTC, so no window edge straddles a transition
— is **not tested**. It is a property of `time.Parse`, not of our code; a test for it
would be a test of the standard library. The service walk is timezone-free by
construction and has nothing to assert.

### The today/upcoming boundary is one test, asserted from both sides

One test seeds an expense dated **exactly today** and one dated **tomorrow**, then
asserts both calls: today's is in `DailyGroupsInRange`'s last row and **absent** from
`UpcomingGroups`; tomorrow's is the reverse.

The bug being hunted is not a missing expense — it is an expense landing in *both*
sections, rendered twice on one page, with both day totals looking correct. Two separate
per-call tests can both pass while that happens: the partition is the invariant, so the
partition is what gets asserted.

### The gap-fill cases

Empty window (no expenses in range — every day is an empty group, none of them nil), a
window with expenses only on the first and last day, a single-day window (`start ==
end`, one group), and `start > end` (`ErrInvalidRange`, above).

### The first test, and it lands red

**`TestDailyGroupsInRange_GapFillsEmptyDays`** in `internal/service/service_test.go`: a
five-day range with expenses on days 1 and 5, asserting five groups newest-first, days
2–4 empty, `Expenses` non-nil throughout. It **cannot compile until
`DailyGroupsInRange` exists** — that failure is the red. It pins gap-filling, ordering
and the never-nil invariant in one shot, which is the entire point of the effort.

Tests are written **before** the implementation, not alongside it. Ticket 04 already
designed the interface, so there is nothing for an implementation-first order to
discover.

**No test code is written by this ticket** — wayfinder plans, it does not do. This
answer names the tests; the spec carries them; the implementation session writes them.
