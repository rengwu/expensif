---
type: grilling
blocked_by: [02, 03]
undermined_by: [06]
---

# Re-shape DailyGroups around dates, not expenses

## Question

What is the new signature and body of the service call behind the daily view?

`DailyGroups(ctx, limit int)` (`internal/service/service.go:143`) is expense-indexed:
list N expenses, bucket into a map keyed by date, sort descending. Days without
expenses cannot exist in a map built by iterating expenses. The gap-filling loop has
to come from somewhere that knows about *dates*.

`HandleCalendar` already writes this loop —
`for d := first; !d.After(last); d = d.AddDate(0, 0, 1)` — but it writes it *in the
handler*, reading from a `dayTotals` map it built itself. So the codebase has a
working answer that lives at the wrong layer.

Resolve, applying `codebase-design` (this is a module-depth question):

- Signature: `DailyGroups(ctx, end string, days int)`? `DailyGroupsInRange(ctx,
  start, end string)`? Something that takes the window from ticket 02 directly?
- Where does the gap-filling loop live — service, or handler like the calendar does?
  If service, should `HandleCalendar` be refactored to share it, or is that a
  different effort? (Be honest: shared-helper pressure is real, but so is coupling
  two views that are diverging.)
- Does `DailyGroup` (`internal/domain/models.go:34`) need an `IsEmpty` or `IsToday`
  field, or is `len(Expenses) == 0` enough for the template?
- Currency conversion currently happens in the handler after the fact
  (`handlers_html.go:271-285`), looping over groups. Empty groups have nothing to
  convert. Does that loop need guarding, or does it fall out clean?
- Timezone: "today" comes from `nowInTZ(prefs.Timezone)` but `Expense.Date` is a bare
  `2006-01-02` string. Where is the window's end computed, and in whose zone?

## Answer

> **Undermined by [ticket 06](./06-day-card-chrome-drift.md).** The *rationale* below for
> "`Expenses` is never nil" — that it marshals to `"expenses": []` and ticket 07's island
> `.map`s over it — is void: there is no JSON DTO, the endpoint serves an HTML fragment,
> and `{{range}}` iterates nil happily. The invariant itself still stands, on the new
> rationale recorded in [ticket 10](./10-test-strategy.md), and is asserted there.
> Everything else in this answer is unaffected.

`DailyGroups(ctx, limit)` is replaced by **two service calls**, both returning
`[]domain.DailyGroup` sorted newest-first:

```go
// Every day in [start, end] appears, empty days included.
func (s *Service) DailyGroupsInRange(ctx context.Context, start, end string) ([]domain.DailyGroup, error)

// Expenses dated after `after`, grouped by day. Not gap-filled.
func (s *Service) UpcomingGroups(ctx context.Context, after string) ([]domain.DailyGroup, error)
```

**Range, not `(end string, days int)`.** It matches `ListExpensesInRange`, and ticket
02's island pages backwards by fetching arbitrary older *ranges*, so a day-count
parameter would be converted back to a range at every call site.

**The gap-filling walk lives in the service.** Two callers need gap-filled groups —
`HandleDaily` and ticket 02's JSON endpoint — so the seam is real rather than
hypothetical. Today's expense-bucketing body is not deleted: it survives as
`UpcomingGroups`, which [ticket 03](./03-future-dated-expenses.md) requires to be
grouped but *not* gap-filled.

**`HandleCalendar` is not refactored onto it.** The shared material is a three-line
date walk. Everything else diverges: the calendar converts currency *before*
bucketing (the daily view converts *after* grouping), it emits `CalendarCell` with
totals, counts and heat quintiles rather than expense lists, and its range runs a year
back to a year forward. Consuming `DailyGroupsInRange` would make it load 730 days of
expense lists it immediately discards. By the deletion test a shared `eachDay()` helper
is a pass-through — delete it and three lines reappear in two places.

**No new fields on `DailyGroup`.** Emptiness stays derivable from `len(Expenses) == 0`;
an `IsEmpty` field would give one fact two homes. Instead the service holds an
invariant: **`Expenses` is never nil** — a gap-filled empty day carries
`[]domain.Expense{}`, so it marshals to `"expenses": []` rather than `null` and ticket
07's island can `.map` over it with no null guard.

`IsToday` is **deferred**, not rejected. It is fog ("Today's row"), and it costs this
signature nothing later: the handler already computes today, so when that patch
graduates it can pass `Today` in page data and the template compares
`{{if eq .Date $.Today}}`. Adding the field now would force `today` into the service as
a third date parameter to render nothing.

**The service stays timezone-free**, and this resolves the timezone fog outright. The
handler computes `today := nowInTZ(prefs.Timezone)` — it already does — formats it to
`2006-01-02`, and passes bare date strings across the seam. Inside, the walk iterates
values from `time.Parse("2006-01-02", …)`, which are UTC, so **no window edge can
straddle a DST transition**: the timezone is consulted exactly once, to name today, and
never again. Ticket 03's "future" comparison is that same single naming.

**The currency loop needs no guard.** Verified by reading `handlers_html.go:269-286`:
for an empty group the inner loop runs zero times, `convTotal` stays `0`, and
`ConvertedTotal` is assigned `0`. It falls out clean.

**No new SQL.** `ListExpensesInRange` (`internal/repository/sqlite.go:69`) already takes
an unbounded date range. `UpcomingGroups` reads `[after+1d, <far future>]` through the
same call.

**Order.** Both calls sort descending, matching the page: Upcoming sits above the
timeline, so its furthest-future day is at the top and its last row is the day nearest
today, flowing into today's row below.

**The old signature had no defender** — `DailyGroups` had exactly one caller
(`handlers_html.go:264`), so the 100-expense limit dies with it.

### What this leaves

The test strategy fog is now specifiable and graduates to
[Test strategy for the date-indexed timeline](./10-test-strategy.md). The timezone fog
is answered above and is struck rather than graduated.
