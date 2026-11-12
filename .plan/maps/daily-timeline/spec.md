# Spec — Daily View: a date-indexed timeline

Synthesized from the wayfinder map at [`map.md`](./map.md) and its fourteen resolved
tickets. Where this spec and a ticket disagree, the ticket's **Answer** section is the
record of why; where tickets disagree with each other, the later ticket governs (06
supersedes parts of 02 and 07; 10 re-grounds an invariant from 04; 14 constrains 12).

## Problem Statement

The Daily View is expense-indexed: it takes the last 100 expenses and buckets them by
date, so a day only exists on the page if an expense happens to sit on it. A user
scanning their week sees an unbroken stack of spending with no sense of time passing —
a quiet Tuesday simply isn't there. There is no way to see *that* a day was empty, no
affordance to add an expense to a specific past day from the timeline, and the
100-expense cap means the visible history is an arbitrary number of days. Meanwhile
future-dated expenses (which the app accepts) float above today with no explanation,
and an unparseable date like `"banana"` renders as a day header at the top of the page.

## Solution

The Daily View becomes **date-indexed**: every day in a rolling 30-day window ending at
today appears, whether or not it carries an expense. The page is a single continuous
**ledger** — no cards — where an empty day is one muted 28px row with an always-visible
`+`, and a day with expenses lists them between two unbroken vertical rails with a
right-aligned day total. Today is marked with a solid dot in the date gutter. The
ledger continues past today into the future, ungapped, with future dates tinted blue
and capped at the three days nearest today (the rest collapse into one overflow row
linking to the Calendar View). Older 30-day windows load via infinite scroll — the
page's only JavaScript — fetching server-rendered HTML fragments until the earliest
expense is reached. Expense dates are validated on the write path so garbage can no
longer enter. A brand-new user still sees the existing "No expenses yet." empty state;
everyone else sees the timeline.

## User Stories

1. As a user, I want every day in the recent past to appear on my Daily View, so that I can see which days I spent nothing as clearly as which days I spent something.
2. As a user, I want empty days rendered as muted single rows rather than full cards, so that the days that carry expenses stand out against them instead of drowning among them.
3. As a user, I want an always-visible `+` on every empty day, so that I can add a forgotten expense to that exact date in one tap.
4. As a mobile user, I want every add and edit affordance to be a real tap target rather than hover-revealed, so that the page works on a phone.
5. As a user, I want the whole empty-day row to be the add link, so that I don't have to hit a small button precisely.
6. As a user, I want a day with expenses to show each expense with its amount, category, description, and payer, so that I can scan a day at a glance.
7. As a user, I want a bold right-aligned total on every spending day, so that day subtotals line up in a column I can run my eye down.
8. As a user, I want a `+` row at the foot of a spending day's expenses, so that I can add a second expense to a day that already has one.
9. As a user, I want each expense row to link to its edit page, so that fixing a fat-fingered amount is one click away.
10. As a user, I want month transitions marked with a heavier divider, so that I can orient myself while scrolling through weeks of rows.
11. As a user, I want today's row marked with a dot beside its date, so that I can find my anchor instantly on a page where the future may sit above it.
12. As a user, I want today's empty row to read "no expenses yet" in darker ink than other empty days, so that the row I came to fill invites me instead of receding.
13. As a user, I want my future-dated expenses (scheduled rent, booked travel) to stay visible above today, so that anchoring the timeline at today doesn't hide money I've committed.
14. As a user, I want future days rendered as ordinary ledger rows with tinted dates and no section chrome, so that the page stays one ledger rather than a stack of boxes.
15. As a user with many future expenses, I want only the three upcoming days nearest today shown, with the rest collapsed into one summary row stating their count and total, so that today is never pushed below the fold.
16. As a user, I want that overflow row to link to the Calendar View, so that nothing about my future spending is hidden without a trace.
17. As a user, I want to scroll past day 30 and have older days load automatically, so that reaching last quarter doesn't require a pagination UI.
18. As a user, I want the infinite scroll to stop with a terminal marker at my earliest expense, so that I am not scrolled into an infinite empty pre-history.
19. As a user on a flaky connection, I want a failed load to show an explicit Retry button rather than silently retrying or silently stopping, so that I stay in control and don't emit a burst of failing requests.
20. As a user without JavaScript, I want the first 30-day window fully server-rendered and functional, so that I can read and add expenses with only infinite scroll unavailable.
21. As a user clicking a day cell on the Calendar View, I want a single-day detail view of that date — any date within a year either way — so that I can inspect days the timeline window can't reach.
22. As a user landing on an empty day via the calendar, I want a full "No expenses for this day" state rather than a muted background row, so that the page doesn't read as a broken link.
23. As a user viewing today via the calendar, I want the same today-dot the timeline shows, so that the two views never disagree about what today looks like.
24. As a user, I want to delete an expense from its edit page behind a confirm step, so that a destructive action costs deliberate clicks instead of sitting on every row.
25. As a user deleting an expense, I want to be returned to the page I came from — timeline, single-day view, or List View — so that deleting doesn't strand me on the wrong page with my filter gone.
26. As a user cleaning up test or duplicate data, I want the List View to keep its one-click row delete, so that bulk cleanup doesn't become five page journeys.
27. As a user (or API client) submitting an expense, I want an unparseable date rejected with a 400, so that garbage can never become a day header at the top of my timeline.
28. As a user, I want an empty date to keep defaulting to today, so that quick entry stays quick.
29. As a new user with no expenses ever, I want the "No expenses yet." empty state instead of thirty muted rows, so that my first sight of the app is an invitation, not a void.
30. As a dormant user whose only expenses are months old, I want the real timeline (not the empty state), so that scrolling reaches my own data.
31. As a user in a non-UTC timezone, I want "today" computed in my preferred timezone, so that the timeline doesn't end on yesterday's date while I'm awake.
32. As a user with expenses in several currencies, I want day totals converted to my Preferred Currency exactly as they are elsewhere in the app, so that totals are comparable.

## Implementation Decisions

### Service layer

- `DailyGroups(ctx, limit)` is deleted (it had exactly one caller, so the 100-expense
  cap dies with it) and replaced by two service calls, both returning
  `[]domain.DailyGroup` sorted newest-first. From the grilling in ticket 04:

  ```go
  // Every day in [start, end] appears, empty days included.
  DailyGroupsInRange(ctx context.Context, start, end string) ([]domain.DailyGroup, error)

  // Expenses dated strictly after `after`, grouped by day. Not gap-filled.
  UpcomingGroups(ctx context.Context, after string) ([]domain.DailyGroup, error)
  ```

- **Range, not `(end, days)`** — pagination fetches arbitrary older ranges, so a
  day-count would be converted back to a range at every call site.
- **The gap-filling walk lives in the service**, not the handler. The Calendar View's
  handler keeps its own three-line walk: it converts currency before bucketing, emits
  heat-quintile cells rather than expense lists, and spans two years — sharing would
  make it load 730 days of expense lists it discards.
- **No new fields on `DailyGroup`.** Emptiness is `len(Expenses) == 0`; no `IsEmpty`,
  no `IsToday`. **`Expenses` is never nil** — a gap-filled empty day carries an empty
  slice. The original JSON-marshalling rationale is void (there is no JSON); the
  invariant stands because a meaningless nil-vs-empty distinction is a latent bug.
- **The service is timezone-free.** The handler consults the Preferences timezone
  exactly once, to name today as a `YYYY-MM-DD` string, and passes bare date strings
  across the seam. The walk inside iterates UTC-parsed dates, so no window edge can
  straddle a DST transition.
- **The service validates its own range**: unparseable endpoints return
  `ErrInvalidDate` (reusing ticket 09's error); `start > end` returns a new
  `ErrInvalidRange`. Both map to **400 Bad Request**. An empty slice was rejected
  because an inverted range from user-controlled query params would reach the scroll
  island as an empty fragment, indistinguishable from the terminal state.
- **No new SQL.** The existing range-listing repository call already takes an
  unbounded date range; `UpcomingGroups` reads through the same call.
- The handler's currency-conversion loop needs no guard for empty groups — verified:
  the inner loop runs zero times and the Converted Amount lands at 0.

### Window and pagination

- The window is a **rolling 30 days ending at today**: `[today-29, today]`. Always 30
  rows, today always present. Month-alignment was rejected (a one-day-tall first
  window on the 1st). The window's end is **today, never later** — future expenses are
  a second, separate, ungapped query, not an extension of the window.
- The first window is **server-rendered** in the daily template. Older windows arrive
  via an **infinite-scroll island** — a deliberate user choice, re-confirmed after the
  page became otherwise JS-free, resting on scroll ergonomics rather than sunk JS cost.
- **The island is ~30 lines of vanilla TypeScript, not React**: a sentinel, a fetch,
  an `insertAdjacentHTML`, a cursor. It gets its own Vite entry for bundling. It is
  the app's first non-React island (see Further Notes re the architecture docs).
- **The endpoint is `GET /daily/older?start=YYYY-MM-DD&end=YYYY-MM-DD`**, served by
  the HTML handler layer, returning `text/html` — **not** the JSON API envelope. The
  fragment is rendered by **the same Go partial that renders the first window**, so
  the ledger has exactly one implementation and there is no Go/TSX drift to contain.
- **Pagination is a server-issued cursor.** The fragment's wrapper carries
  `data-next-start` / `data-next-end` attributes; they are **absent** when the
  earliest expense has been reached. The first cursor is server-rendered into the
  page. The island does no date arithmetic and owns no stop condition — window
  walking, clamping the final short window to the earliest expense, and termination
  are one rule computed server-side. An empty database yields no first cursor and the
  island never fetches.
- **Trigger and states**: `IntersectionObserver` on a foot sentinel, one fetch in
  flight at a time. Success appends and re-arms; a missing cursor renders the terminal
  marker and disconnects the observer; an error disconnects the observer and shows a
  manual **Retry** button — never an armed observer through a failure, never
  auto-retry.
- **All four foot states (sentinel, loading, error-with-retry, terminal) are
  server-rendered** in the daily template; the island only sets `data-state` on the
  wrapper and CSS reveals one. **Zero Tailwind classes and zero markup live in
  TypeScript.**
- **No `<noscript>` fallback.** A JS-less user gets a fully functional 30-day ledger
  and reaches any day in ±1 year through the Calendar View's plain links. A fallback
  would have grown the daily handler a third branch.
- Tailwind is served via the Play CDN, which observes DOM mutations — appended
  fragments are styled with no build-time content globs to configure (verified).

### The ledger (day entry)

Approved markup: [`assets/day-entry-ledger.html.approved`](./assets/day-entry-ledger.html.approved).

- Five flex columns, identical for empty and spending days:
  `[ date w-28 ][ rail w-px ][ expenses flex-1 self-start ][ rail w-px ][ total w-24 ]`.
- Three load-bearing alignment invariants, each arrived at by fixing a visible defect:
  1. **Rails are their own stretching columns**, never `border-l` on a neighbour —
     otherwise the line breaks where the expenses column is `self-start`. Days carry
     no vertical padding between them, so consecutive rails touch and read as two
     unbroken lines.
  2. **Vertical padding lives on the row, never on the column**, so date, first
     expense, and total share a baseline.
  3. **Everything is a 28px row** (16px text with `leading-4` pinned explicitly plus
     `py-1.5`), or the mixed text sizes drift the rhythm.
- The date gutter reads `Jul 8 · Wed` in `tabular-nums` (the app's longer date format
  wraps at this width). The day total is `text-sm font-bold`, right-aligned,
  outweighing the `text-xs` expense amounts.
- An **empty day is one 28px row**: muted italic "no expenses" left, `+` right, the
  whole row a link to the add form pre-filled with that date. Empty days reserve the
  total column's width with a spacer so the rails never shift between day kinds.
- A **spending day** gets a dedicated full-width `+` row at the foot of its expenses
  column, `justify-end` so its glyph aligns with the empty days' `+` down the page.
- **Month transitions** get a heavier divider (matching rail gray); all other day
  dividers are near-invisible. Computed by a Go helper comparing adjacent groups'
  `YYYY-MM` (a template can't see the previous range item). No cross-window state is
  needed: the timeline is gap-filled and contiguous, so within and across windows the
  break falls exactly on the 1st of a month. `divide-y` must not be used on the
  wrapper — its child selector outranks the month divider's class; borders sit on
  each day.
- **This drops the data-table island from the Daily View** — its only island — so
  expenses become server-rendered HTML and the page works without JavaScript for the
  first time. Rows link to the edit page; delete is demoted there (see below).

### The future continuation ("Upcoming")

Approved markup: [`assets/upcoming-continuation.html.approved`](./assets/upcoming-continuation.html.approved).

- **There is no Upcoming section.** The ledger continues past today into the future,
  ungapped — no heading, no divider, no panel, no tinted background. A labelled region
  died on the prototype: its boundary was the same weight as the month divider and the
  two collided when an upcoming expense sat in next month.
- A future day is marked by exactly one thing: its **date is tinted `text-blue-500`**
  instead of the normal semibold dark. Rails, dividers, and the `+` row are unchanged
  — a future day is an ordinary day (the `+` row's presence-by-accident on ungapped
  days was raised and consciously accepted; adding to an arbitrary future day remains
  the Calendar View's and the single-day view's job).
- **Capped at the 3 upcoming days nearest today.** Further-future days collapse into
  one 28px overflow row — `later │ N more upcoming days → │ <total>` — placed above
  the days it summarises, **linking to the Calendar View** (a link, not a disclosure:
  expanding in place would need JS the page doesn't have). A horizon cap was rejected
  because it would hide future expenses without a trace.
- The empty case (no future expenses — the common case) renders nothing at all above
  today. No chrome exists, so none has to be hidden.

### Today's row

Approved markup: [`assets/todays-row.html.approved`](./assets/todays-row.html.approved).

- With no section chrome, today's row is the **only** boundary between "hasn't
  happened yet" and "already spent", so it **must** be distinguished (ticket 14's
  constraint; letting the future tint carry the boundary alone was rejected because
  the tint never appears on pages with no future expenses).
- The mark is a **solid gray-900 dot in the date gutter, beside a date that stays**.
  Replacing the date with "Today" (loses the date, breaks `tabular-nums`) and
  thickening the rails (reads as a seam in the unbroken lines) both died on the
  prototype. Gray-900 deliberately, never blue — blue is spent on future dates
  directly above.
- **Today's empty row does not recede**: same 28px structure and whole-row add target,
  but "no expenses **yet**" in `text-gray-500` (not muted italic gray-300) with a
  `text-gray-900` `+`. A full call-to-action banner was rejected as shouting.
- The dot is a **property of the day, not of the timeline**: the shared day partial
  compares the group's date against `Today` from page data
  (`{{if eq .Date $.Today}}`), which base page data already supplies on every page.
  So the single-day `?date=` view shows the dot for today too, nothing is threaded,
  and the two views cannot drift. No `IsToday` field anywhere.

### The single-day view (`?date=`)

- **The daily handler's two branches do not converge.** `?date=` is a single-day
  detail view for any date in ±1 year (every Calendar View cell links to it),
  including future zero-spend days and days before the earliest expense — dates the
  timeline's contract can never render. It keeps its own range query and hand-built
  one-element group, and does **not** call `DailyGroupsInRange`. The shared
  currency-conversion loop stays shared.
- What converges is **rendering**: a populated day draws through the ledger's
  day-entry partial **verbatim** — no `solo` flag, no rail-suppressing conditional
  (that conditional is the chrome-drift shape just deleted). Short rails and a mostly
  empty total column are accepted; the page title and back-link supply context.
- An **empty** `?date=` day shows the fuller "No expenses for this day" state, *not*
  the muted ledger line. Two zero-state designs on purpose: on the timeline an empty
  day is background; under `?date=` it is the entire answer to the click, and muting
  it reads as a broken link. So the muted line has exactly one consumer (the
  timeline); the populated day-entry partial has two (timeline and `?date=`), plus
  the fragment endpoint as a third invoker.
- The branch's unreachable `else` back-link (only firing for hand-typed URLs) is noted
  and left alone.

### The zero-expense-ever empty state

- The existing full-page "No expenses yet." + **Add one** swap **survives, verbatim**,
  re-conditioned on `NoExpensesEver := earliest expense date == ""` — "ever", not "in
  this window". This is free: the handler already fetches the earliest expense date
  for the scroll cursor, and the repository returns `""` on an empty table.
- The old `{{if .DailyGroups}}` condition is **dead** (a new account now has thirty
  gap-filled groups) and is replaced by the flag. The branch lives **in the
  template**; the handler's control flow stays at two branches, honouring the earlier
  refusal to grow a third. Accepted cost: a brand-new account walks thirty empty
  groups the template discards.
- **Dormant accounts** (data exists, all older than the window) get the real timeline
  and reach their data by scrolling. Accounts whose only expense is future-dated get
  the timeline plus the future continuation — and their earliest date sits after the
  window start, so the scroll terminal fires on the first window and the island never
  fetches, which is correct.
- The add link carries no date; empty dates default to today on the write path.

### Delete and the edit page

Approved markup: [`assets/edit-delete-danger-zone.html.approved`](./assets/edit-delete-danger-zone.html.approved).

- Delete is **built on the edit page as a "Danger zone" at the foot**: a **sibling**
  form posting to the existing delete route (the page is already one form; nesting is
  invalid HTML), one click plus a JS `confirm()` naming the expense ID. The zero-JS
  variants lost because their premise was false — the edit page already mounts two
  islands (Category Suggestion and Description Suggestion pills) and has always
  required JS; the JS-free property belongs to the Daily View alone.
- Accepted cost, recorded: the confirm dialog states only the ID, not the amount or
  description.
- **The return path is a server-issued `?return=`**: the ledger row's Edit link
  carries where it came from, the edit handler passes it through, the danger-zone
  form submits it as a hidden field, and the delete handler validates it as a local
  path (must begin with `/`, not `//`, no scheme) with a `/` fallback. No client
  state, no Referer.
- **The List View is now the intentional bulk-delete surface.** It keeps its
  data-table and row-level one-click Delete. **Constraint: any future effort that
  drops the data-table from the List View must replace that affordance first.**

### Date validation (already implemented — ticket 09)

- A non-empty date must parse as `YYYY-MM-DD`, else `ErrInvalidDate` → 400, on both
  create and update, API and form. Future dates remain valid (the future continuation
  depends on them). Empty dates default to today. Existing garbage rows are left
  untouched; the read path does **not** parse defensively (explicitly declined).

### The clock seam

- The daily handler gains an injectable `now func() time.Time`, defaulted to
  `time.Now`, so "anchor at today" is testable across a timezone boundary. "Today" is
  computed in the Preferences timezone, once.

## Testing Decisions

**What makes a good test here**: assert external behaviour at a public seam, never
implementation details. For HTML, that means the machine-readable contract only —
`data-next-*` and `data-state` — never chrome, class names, or copy. If a test would
break when a `div` becomes a `span`, it is the wrong test. (This narrows, without
breaking, the older project testing-strategy doc's "never assert on template HTML"
rule, which predates the endpoint that serves HTML as its contract; that doc's
renderer-extraction "blocker" was also found not to exist — handler tests already
build the real renderer in-package.)

**Two seams:**

1. **The service, for all logic.** `DailyGroupsInRange` and `UpcomingGroups` against
   the repository interface, using the existing expense-repo stub pattern (the stub
   grows a range-listing function field mirroring its existing ones; no second stub).
   Prior art: the existing service tests.
2. **The fragment, for the island's contract only.** `GET /daily/older` through the
   real renderer via `httptest`, asserting only the cursor attributes and the foot's
   `data-state`. Prior art: the existing in-package server test shape.

**Named cases:**

- **First test, landing red before any implementation**:
  `TestDailyGroupsInRange_GapFillsEmptyDays` — a five-day range with expenses on days
  1 and 5, asserting five groups newest-first, days 2–4 empty, `Expenses` non-nil
  throughout. It cannot compile until the new call exists; that failure is the red.
- Gap-fill cases: empty window (every day an empty, non-nil group), expenses only on
  first and last day, single-day window (`start == end`), and `start > end` →
  `ErrInvalidRange` from the service, 400 from both the page handler and the fragment
  endpoint.
- **The today/upcoming partition, asserted from both sides in one test**: seed one
  expense dated exactly today and one dated tomorrow; assert today's is the window's
  last row and absent from `UpcomingGroups`, and the reverse for tomorrow's. The bug
  hunted is an expense rendering in both places, which two separate per-call tests
  would both pass.
- **The never-nil invariant is asserted once**, in one dedicated test that names it —
  not as a line in every case.
- **The clock test**: freeze the injected clock at `2026-07-11T23:00Z` with timezone
  `Pacific/Auckland` and assert the window ends `2026-07-12`, not the 11th.
- **One cheap same-partial assertion**: the timeline, the fragment endpoint, and the
  `?date=` view all invoke the same day-entry partial (this replaced the unbuildable
  "render through both implementations" test — there is only one implementation).

**Deliberately not tested**: the handler's currency loop (no branch to cover — an
empty group falls through to zero); the DST claim (a property of `time.Parse`, i.e.
of the standard library); the ledger's visual chrome.

Tests are written **before** the implementation — the interface is already designed,
so there is nothing for an implementation-first order to discover.

## Out of Scope

- **Collapsing long runs of empty days** into an expandable row. Ruled out by the
  user while scoping; it stays out because the user ruled it out (its original
  no-islands rationale no longer holds), and it does not graduate — wanting it means
  a fresh effort.
- **Repairing existing rows with unparseable dates** in deployed databases, and any
  defensive parsing on the read path. A garbage row written before validation landed
  will still sort above the timeline; accepted in ticket 03.
- **The data-table hydration bug** (it hydrates a container the server left empty; a
  `mountIsland` helper is its fix). Real, discovered during this effort, and outside
  it — no daily-timeline work depends on the data-table any more. Fix as separate work.
- **Number formatting.** Totals render without thousands separators (`RM1223.10`),
  carried over from the card. Pre-existing; not fixed here.
- **Refactoring the Calendar View's handler** onto the new service calls.
- **Any change to the List View** — its data-table and row-level delete are now
  load-bearing (the bulk-delete constraint above).
- **The unreachable back-link `else`** in the single-day branch — noted, untouched.

## Further Notes

- **Approved markup assets** are the visual source of truth for the implementation:
  [`day-entry-ledger.html.approved`](./assets/day-entry-ledger.html.approved),
  [`upcoming-continuation.html.approved`](./assets/upcoming-continuation.html.approved),
  [`todays-row.html.approved`](./assets/todays-row.html.approved),
  [`edit-delete-danger-zone.html.approved`](./assets/edit-delete-danger-zone.html.approved).
- **The architecture docs want a one-line update**: `AGENTS.md` and ADR 0001 describe
  the frontend as "React Islands"; the scroll island is the first island that is not
  React (vanilla TS, HTML-fetching). This does not reverse ADR 0001 — Go still owns
  routing, data, and the document — but the "React owns the islands" sentence is no
  longer universally true. Ticket 06 flagged this for when the spec lands; the
  implementation session should touch it up (an ADR amendment note, not a new ADR).
- **`CONTEXT.md` gains no new terms from this spec's decisions yet**, but "Daily
  View"'s definition ("groups expenses by date, newest first") will need updating to
  mention date-indexing once implemented, and **Ledger**, **Upcoming**, and
  **Window** are candidates for the glossary when the implementation firms them up.
- **Implementation order hint** (from ticket dependencies, not a mandate): the service
  rework and its red-first tests, then the ledger template + the `?date=` and
  empty-state template branches, then the fragment endpoint + island, then the edit
  page danger zone. Date validation (ticket 09) is already merged.
- The prototype throwaways (a prototype handler and template) are folded in and
  deleted when the ledger lands, per ticket 01.
