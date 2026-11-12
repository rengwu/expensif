---
type: task
blocked_by: [01, 04]
assets:
  [
    ../../daily-timeline/assets/day-entry-ledger.html.approved,
    ../../daily-timeline/assets/todays-row.html.approved,
    ../../daily-timeline/assets/upcoming-continuation.html.approved,
  ]
---

# The ledger

## Question

Deliver the spec's whole visual layer, from the three approved assets: the day-entry
partial (five columns, two unbroken rails, 28px rows, the three alignment
invariants), muted empty-day rows, today's gray-900 dot and non-receding empty row
(`{{if eq .Date $.Today}}` against page data), the future continuation with tinted
dates and the capped-at-3 overflow row linking to the Calendar View, month-break
dividers via the adjacent-groups Go helper, and the `NoExpensesEver` template branch
swapping in the verbatim "No expenses yet." block.

This **drops the data-table island from the Daily View** — expense rows become
server-rendered links to the edit page, carrying the `?return=` the danger zone
(previous ticket) already honours. The `?date=` view renders populated days through
the same partial verbatim and keeps its fuller empty state. Delete any prototype
leftovers this replaces.

Blocked by the danger zone as well as the service: landing the ledger first would
remove the daily view's row delete before its demoted home exists.

Tests: only the cheap same-partial assertion (timeline, `?date=` — the fragment
endpoint joins it next ticket). The chrome itself is deliberately untested — verify
by eyeballing the rendered page against the assets on a seeded scratch DB, including
the empty-account, dormant-account, and future-heavy cases.

Done when: the daily view is card-free, JS-free, and matches the approved markup;
`?date=` and the empty states behave per spec; checks green.

## Answer

Shipped from the three assets. The day partial lives at
`templates/partials/day-entry.html` — one define handling both day kinds, so the
rails cannot drift between them — and derives today and the future from the day's
date (`eq`/`gt` against `today` in its dict), never from a threaded flag, so
`?date=<today>` shows the dot for free. `daily.html` is rewritten around it: the
timeline ranges the merged groups, the `?date=` branch draws a populated day through
the partial verbatim and keeps the fuller "No expenses for this day" card when empty,
and the `NoExpensesEver` swap is unchanged. `HandleDaily` no longer registers
`data-table`; the page's only scripts are the nav.

Calls this ticket made, recorded:

- **The overflow row sits above the three shown upcoming days.** The approved
  asset's sketch *order* puts it between them and today, but its own comment, ticket
  14's answer, the map, and the spec all say "above the days it summarises (further-
  future is further up)" — the prose won. Cap mechanics live in the handler, after
  the conversion loop, as `PageData.HiddenUpcoming`/`HiddenUpcomingTotal`
  (`handlers_html.go`); the slice is newest-first so the shown three are its tail.
- **`data-day="YYYY-MM-DD"` on the partial's root** — not in the assets. It is the
  machine-readable marker for the spec's one same-partial test
  (`TestHandleDaily_TimelineAndSingleDayShareDayEntryPartial`, landed red first) and
  the natural hook for ticket 03's fragment assertions.
- **Month breaks**: `monthBreak` template func over the group slice
  (`renderer.go`), comparing adjacent `YYYY-MM` prefixes defensively — a
  pre-validation garbage date shorter than seven bytes must not panic the page.
  Probed with a hand-inserted `date='banana'` row: no panic, but the row **vanishes
  from the daily view** (lexically outside both range queries) rather than "sorting
  above the timeline" as the spec's out-of-scope note predicted from the old
  rendering. Still visible in the List View; accepted garbage either way.
- **Ticket 04's recorded gap is closed**: the edit form now posts `return` back
  (hidden field in the shared `form` partial, keyed off `ReturnTo` so `new.html` is
  untouched), and `HandleUpdate`'s error branch restores it — a delete after a failed
  save no longer falls back to `/`. `HandleDaily` sets `ReturnTo` to its own
  request URI on both branches, so every ledger row's edit link carries it.

Verified live on scratch DBs (`DATA_DIR=<tmp>`), screenshots against the assets:
future-heavy account (overflow `later │ 3 more upcoming days → │ $2460.00`, three
tinted dates, dot on today, month divider above Jun 30, rails unbroken, baselines
level); empty account (page-swap intact); dormant account (real timeline, today's
empty row non-receding: "no expenses yet", dark `+`); `?date=` populated / empty /
today / future-tinted. Probes: hostile `?return=` still falls to `/`; failed save
keeps the return; garbage date renders 200.

In passing: the tz→`*time.Location` lookup, triplicated across renderer funcs once
`shortDate` joined, extracted to `tzLocation`; pre-existing `gofmt` drift fixed in
`mock_repo_test.go` and `service_test.go`. `go test ./...`, `go vet ./...`,
`tsc --noEmit` green; no TypeScript changed.
