---
type: grilling
blocked_by: [01, 04]
undermined_by: [06]
---

# The infinite-scroll island's contract

## Question

Specify the new island end to end. It is the app's first pagination island and the
first island to fetch anything — every existing island (`data-table`,
`category-pills`, `description-pills`, `mobile-nav`) is handed its props as JSON at
mount and never talks to the server.

Resolve:

- **Endpoint.** Path and shape. `internal/web/handlers_api.go` exists; does this live
  there as `GET /api/daily?before=YYYY-MM-DD&days=30`, returning `[]DailyGroup`? Does
  it reuse the same service call `HandleDaily` uses (it should — see ticket 04)?
- **Trigger.** `IntersectionObserver` on a foot sentinel, or an explicit button? The
  decision said "infinite scroll", but a button is a legitimate degenerate case worth
  re-confirming once the cost is concrete.
- **States.** Loading, error, and the terminal marker ("No expenses before Mar 4,
  2024" — see ticket 02's termination rule). What happens on a failed fetch: retry,
  silent stop, or a visible error with a retry affordance?
- **How does the island know where to start and when to stop?** It needs the oldest
  date currently on screen and the earliest expense date. Both are server-known at
  first render — pass them as island props via `data-props`, like `hydrate.tsx:12`
  reads, rather than making the client discover them.
- **Mounting.** `hydrate.tsx` finds a single container by `[data-island="name"]`;
  `data-table.tsx` uses its own `querySelectorAll` loop and does not use
  `hydrateIsland` at all. Which pattern does this island follow, and does it need to
  mount the `DataTable` component directly as a child rather than via `data-table`'s
  document-wide scan? (It does — a scan cannot see nodes appended later.)
- **No-JS.** With the first window server-rendered, a JS-less user sees 30 days and no
  way back. Acceptable, or does a `<noscript>` link to a plain paginated fallback earn
  its keep? Note the page's tables already require JS, so a JS-less user sees 30 muted
  cards and nothing else.

---

## Update — after ticket 08 (the ledger)

**The premise of this ticket has changed.** It was written assuming the island renders
`DataTable` as a child of each appended day-card. There is no `DataTable` on the daily
view any more — verified on the prototype: 0 `data-table-root` nodes, against 8 on the
control page. The island would render plain ledger rows. Simpler.

The no-JS question is no longer a footnote. With expenses server-rendered, the daily
view works without JavaScript **for the first time** — a JS-less user gets a fully
functional 30-day timeline, not a `<noscript>` message. Infinite scroll would be the
only thing on the page that needs JS.

So re-open the mechanism, honestly:

- Ticket 02 chose an infinite-scroll island over a plain `?days=` link. One argument
  against the plain link was that this page already requires JS, so a no-JS fallback
  bought nothing. **That argument is now false.**
- A plain "Load older" link would leave the entire daily view JS-free and delete the
  island, the endpoint, and ticket 06's drift problem in one move.
- The user chose the island deliberately and may still want it. But the decision was
  made under a fact that no longer holds, and it deserves one honest re-ask before
  anyone writes the endpoint.

If the island survives: it renders ledger rows, needs the oldest on-screen date and
the earliest expense date as props, and the mount question below still stands.

---

## Answer

**Undermined by [Contain the day-card chrome drift](./06-day-card-chrome-drift.md).** This
ticket specified a JSON payload without re-reading ticket 02's rejection of HTML fragments
against ticket 08, which had voided that rejection's only stated reason. Ticket 06 found it.

**Void:** the display-ready DTO, dropping `convertedTotal`, and `mountIsland`/`createRoot`.
The endpoint serves HTML from the same Go partial as the first window, there is no DTO, and
the island is not React. `GET /api/daily` is now `GET /daily/older`, in `handlers_html.go`.

**Standing, and cheaper:** the island survives its re-ask; pagination is a server-issued
cursor (carried as `data-next-*` attributes rather than a JSON field); range parameters, not
`?before=&days=`; sentinel-triggered with one fetch in flight; terminal marker when the
cursor is absent; manual retry on error, never an armed observer; and no `<noscript>`
fallback. Read the sections below with that substitution in mind.

**The island survives the honest re-ask.** The mechanism was put to the user again with
the falsified premise stated plainly and the plain-link alternative priced — including
that it would delete the island, the endpoint, and ticket 06 in one move. The user chose
infinite scroll again. [Ticket 02](./02-window-size-and-pagination.md)'s decision therefore
no longer rests on "the page already needs JS"; it rests on a deliberate preference for
scroll ergonomics, taken with the cost visible. Ticket 02's `undermined_by` is discharged.

Everything below follows from that, and from the standing rule this ticket kept reaching
for: **logic and formatting stay in Go; the island renders strings and holds a cursor.**

### The endpoint

`GET /api/daily?start=YYYY-MM-DD&end=YYYY-MM-DD`, in `handlers_api.go`, behind the
existing `apiResponse{Data, Error}` envelope. It calls the same
`DailyGroupsInRange(ctx, start, end)` that `HandleDaily` calls — ticket 04 already named
this endpoint as its second caller.

**A range, not `?before=&days=30`.** Ticket 04 rejected `(end, days)` for the service
because the island pages by arbitrary ranges and a day-count would be converted back to a
range at every call site. The same argument binds the endpoint.

### The payload is display-ready

`Data` is an object, not a bare array:

```json
{"data": {
  "groups": [{"date": "2026-06-11", "humanDate": "Thu 11 Jun",
              "total": "$42.00", "expenses": [...]}],
  "next": {"start": "2026-04-13", "end": "2026-05-12"}
}}
```

A **web-layer DTO**, not `domain.DailyGroup` — ticket 04 forbids new fields on the domain
type, and `humanDate`/`total` are display fields. Go's `humanDate` and `printf "%.2f"`
stay the only formatters of any date or number on the page, so the TSX cannot round or
place a currency symbol differently from the template at the 30-day seam. Only Tailwind
class strings duplicate, which is the trade `templates/partials/button.html:1-3` already
documents as convention.

`convertedTotal` **does not appear**. Currency conversion runs in the handler before
marshalling, so `total` is already converted and symbolised; shipping both would put one
fact in two homes. `date` survives alongside `humanDate` because it is an identifier, not
a display string.

Ticket 04's **`Expenses` is never nil** invariant carries into the DTO: an empty day
marshals `"expenses": []`, and the island `.map`s with no null guard.

### Pagination is a server-issued cursor

Each response carries `next` — the range to fetch after this one — or `null` when the
earliest expense has been reached. The first cursor is server-rendered into `data-props`.

**The island does no date arithmetic and owns no stop condition.** Walking back a window,
clamping the final short window to the earliest expense, and terminating all collapse into
one rule, computed in `internal/web` from `GetEarliestExpenseDate`, with one place to test
it. Ticket 07 originally proposed passing `oldest` and `earliest` as props and letting TSX
subtract days and compare; that was rejected as the same duplication the payload decision
just removed, one layer down.

This absorbs the empty-database case for free: `GetEarliestExpenseDate` returns `""` when
`MIN(date)` is NULL (`sqlite.go:241`), the first cursor is `null`, and the island never
fetches. (The *page's* zero-expense empty state remains fog and is not decided here.)

### Trigger and states

`IntersectionObserver` on a foot sentinel, **one fetch in flight at a time**.

- **ok** → append groups, re-arm the sentinel.
- **`next` is null** → render the terminal marker, disconnect the observer.
- **error** → disconnect the observer, replace the foot with an explicit **Retry** button.

The observer is not left armed through a failure. A user who keeps scrolling against a
down server would otherwise emit a burst of failing requests with no way to stop short of
leaving the page. Auto-retry is refused; the retry is the user's.

There is no "empty response" terminal condition. `DailyGroupsInRange` is gap-filled and
always returns one group per day in the range, so a window is never empty — only `next`
can be null.

### Mounting

The container is **server-empty**: sentinel, rows, marker and button are all client-created.
`hydrateRoot` is therefore the wrong call, and both `lib/hydrate.tsx` and
`entries/data-table.tsx` use it today.

Add **`mountIsland(name, Component)`** to `ui/src/lib/hydrate.tsx`, beside `hydrateIsland`:
same `[data-island]` lookup, same `data-props` JSON, but `createRoot` instead of
`hydrateRoot`. It names the distinction the codebase has been fudging — hydrate a container
the server filled, mount one it left empty. New Vite entry `dailyScroll` →
`src/entries/daily-scroll.tsx`, a fifth input in `ui/vite.config.ts`.

**Discovered, and deliberately not ticketed:** `data-table.tsx` calls `hydrateRoot` on a
container `data-table.html` leaves empty — a hydration mismatch React silently recovers
from by client-rendering. It is a real bug, `mountIsland` is its fix, and it is *outside
this map's destination*: ticket 08 removes `data-table` from the daily view entirely, so
no daily-timeline work depends on it. Fix it as separate work.

### No-JS

**No `<noscript>` fallback.** A JS-less user gets a fully functional 30-day ledger, and
reaches any day in the past year through `calendar.html`'s plain `href`s
(`/?date=…&from=calendar`). Only history older than a year needs JS — and the calendar
cannot reach that either, since `HandleCalendar` renders a fixed year-back range and does
not page.

The deciding cost was structural, not effort: a `<noscript>` "load older" link needs
`HandleDaily` to render an arbitrary `?before=` window — **a third branch, added while
[ticket 05](./05-converge-handledaily-branches.md) is mid-flight on whether the existing
two should converge.** This ticket declines to hand 05 a harder problem, and declines to
build most of the plain-link option that was just rejected, for a user who cannot reach
year-old history by any other route anyway.

### What this hands the open tickets

- [Ticket 06](./06-day-card-chrome-drift.md): its "cheaper alternative" is now **decided** —
  the endpoint returns display-ready groups. 06 no longer prices that option; it decides
  only how the class strings stay in sync, and whether the both-paths test earns its keep.
- [Ticket 10](./10-test-strategy.md): the cursor rule is the one new server-side unit with
  a sharp contract — final-window clamping, and `next == null` at the earliest expense and
  on an empty database.
- [Ticket 05](./05-converge-handledaily-branches.md): unchanged. Still two branches.
