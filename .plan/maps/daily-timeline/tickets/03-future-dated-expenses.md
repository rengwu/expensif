---
type: grilling
blocked_by: []
---

# What happens to expenses dated after today

## Question

Anchoring the timeline at today means a future-dated expense has nowhere to render.
Is that acceptable, and if not, what gives?

This is not hypothetical. `HandleCalendar` deliberately extends its range to
`today.AddDate(1, 0, 0)` (`internal/web/handlers_html.go:151`) — a full year forward.
Nobody pads a calendar with a year of empty future months by accident; the app
expects future-dated expenses to exist. Confirm that reading before deciding.

Today's daily view has the bug in a quieter form: `DailyGroups` sorts by date
descending and takes the last 100 expenses, so a future-dated expense _does_ appear,
at the top, above today. Once today becomes the anchor, it silently vanishes.

Resolve:

- Can an expense actually be created with a future date? Check `HandleAdd`
  (`handlers_html.go:292`) and the form for validation. **Look this up, do not ask.**
- If yes: does the daily view show future days when future expenses exist (a ragged
  top edge), pin them into a "Scheduled" section above today, or keep them out and
  accept that the calendar is where you see them?
- If no future expenses are creatable, is the calendar's forward year dead code, and
  does that change the answer to anything else?

The answer constrains the query shape (whether the window's end is `today` or
`max(today, latest expense)`), so it blocks the `DailyGroups` rework.

## Answer

Future-dated expenses render in an **"Upcoming" section above the timeline**: grouped
by day, in the same ledger style, **with no gap-filling** — only days that actually
carry an expense. Below it, a divider, then the 30-day timeline anchored at today.

Then the timeline proper is unchanged: window end is **today**, never later.

### Why not the alternatives

- **Window end = latest expense.** Uniform, one rule — but gap-filling makes it
  unusable. A single expense dated 2031 renders ~1,600 muted empty rows above today.
  The property that makes the past readable makes the future absurd.
- **Hide them.** Would silently _remove_ behaviour that exists today (see below), and
  strand the expense: unreachable from the daily view, and from the calendar too if
  it is more than a year out. Visible only in the flat list.

### Facts established (probed on a scratch DB, today = 2026-07-09)

- **Future dates are creatable.** `POST /api/expenses` with `"date":"2026-07-10"` and
  `"date":"2031-01-01"` both returned `201`. `validateExpenseInput`
  (`internal/service/service.go:47-66`) does not validate the date at all — it only
  defaults an empty date to today. `templates/form.html:46` has no `max=` attribute.
- **They already render in the daily view, above today.** `DailyGroups` sorts
  date-descending with no ceiling. Observed order: `banana`, `Jan 1, Wednesday`
  (2031), `Jul 10, Friday`, `Jul 9, Thursday`. So anchoring at today is a
  **regression** unless Upcoming exists. This is the fact that decided the ticket.
- **The calendar is not a home for them.** Its range ends at `today + 1 year`
  (`handlers_html.go:151`), so the 2031 expense renders in **zero** calendar cells.
  The forward year is not evidence of a considered feature — nothing about dates is
  considered anywhere.
- **`{"date":"banana"}` returns `201`.** It renders as a day header reading `banana`,
  sorted above everything, because `b` > `2` under string comparison.

### Consequence — dates get validated on write

Because "future" means `date > today`, an unparseable date sorts into Upcoming and
lands at the very top of the page. The user chose to **fix this at the source**:
validate dates in `validateExpenseInput` rather than defend on read. Tracked as
[Validate expense dates on the write path](./09-validate-expense-dates.md).

**Accepted residual risk:** the daily view will _not_ parse defensively. Rows written
before the validation lands — in any already-deployed database — will still sort into
Upcoming as garbage day headers. The local dev database is empty; deployed ones may
not be. A one-line skip-on-parse-failure in the grouping loop would close this, and
was explicitly declined in favour of the source fix alone.

### Constraint handed to ticket 04

The window's end is `today`, not `max(today, latest expense)`. Upcoming is a **second,
separate query** — future expenses, grouped, ungapped — not an extension of the window.
