---
type: prototype
blocked_by: []
assets: [../assets/day-entry-ledger.html.approved]
---

# Redesign the day entry as a ledger row

## Question

What does a day *with* expenses look like, once the card is gone?

[Muted empty-day card design](./01-muted-empty-day-design.md) chose the ledger line
for empty days. That choice is not local: an empty day that is a bare row sitting
between two bordered white cards reads as a rendering bug, not a design. The card has
to go from the whole timeline, or come back for empty days. The user chose the former.

Resolve, against a live seeded stack:

- **Layout.** Is the day a section heading with expense rows beneath it? A date rail
  down the left with rows to the right? A flat ledger where the day is just the first
  row's gutter label?
- **Where does the day total go**, now that there's no card footer? And the day's
  "+ Add expense" button (`daily.html:41`) — does it survive, or does the empty-day
  `+` become the only add affordance, reachable only on days with no expenses? (That
  would be a bug: you could not add a second expense to a day from the timeline.)
- **Edit and delete.** The card gets these from `DataTable`'s `actions` column
  (`daily.html:23-25`). A ledger row has no actions column. Do rows link to
  `/expenses/edit/{id}`, with delete demoted to the edit page? Or does each row carry
  a visible delete? Mobile has no hover — the same constraint that killed variant B.
- **Density.** Amounts must right-align on `tabular-nums` to be scannable. Category
  and `paidByName` compete for the same horizontal space on a phone.

## Consequence to price before choosing

A ledger row is not a table. If the day entry stops using `DataTable`, this page loses
its only island — `daily.html` is the sole reason `data-table` is registered here
(`handlers_html.go:288`). That would mean:

- Expenses become **server-rendered HTML**. The daily view would work without
  JavaScript for the first time (today it shows a `<noscript>` message instead of any
  expense — see ticket 02's findings).
- [The infinite-scroll island's contract](./07-scroll-island-contract.md) assumed the
  island renders `DataTable` as a child of each appended day-card. If there is no
  `DataTable`, the island renders plain ledger rows instead — simpler, and the
  no-JS question in that ticket changes shape entirely.
- [Contain the day-card chrome drift](./06-day-card-chrome-drift.md) grows: the TSX
  side must now reimplement expense rows too, not just day chrome.

Do not treat that as a reason to keep the card. It may be a reason to *prefer* the
ledger. Price it, then choose.

## Answer

**Variant F — the date rail — wins**, then iterated live with the user over eight
rounds. The card is gone from the whole timeline. Approved markup is captured at
[`../assets/day-entry-ledger.html.approved`](../assets/day-entry-ledger.html.approved).

### The layout

Five flex columns, identical on both day kinds:

    [ date w-28 ][ rail w-px ][ expenses flex-1 self-start ][ rail w-px ][ total w-24 ]

Three invariants hold it together. They are load-bearing — each was arrived at by
fixing a visible defect, and breaking one reintroduces it:

1. **Rails are their own stretching columns**, not `border-l` on a neighbour. Required
   because the expenses column is `self-start` (see 3); a border on a non-stretching
   column stops short and the line breaks. Days carry no vertical padding *between*
   them, so consecutive rails touch and read as two unbroken lines.
2. **Vertical padding lives on the row, never on the column.** The date's `py-1.5`
   matches an expense row's `py-1.5`, so date, first expense, and total share a
   baseline. Padding on the column instead pushes rows 6px below the date.
3. **Everything is a 28px row** (16px `leading-4` text + `py-1.5`). `leading-4` is
   pinned explicitly, because `text-xs` and the `text-[10px]` category chip otherwise
   resolve to different line boxes and the rhythm drifts.

Each day is wrapped in a div carrying `pt-2 pb-2` on all three content columns.

### The details

- **Date** reads `Jul 8 · Wed` (`Format("Jan 2 · Mon")`). The app's `formatDate`
  ("Jul 8, Wednesday") wraps at `w-28` and breaks the grid.
- **Day total** sits in its own rightmost `w-24` column, right-aligned `tabular-nums`,
  `text-sm font-bold` — outweighing the `text-xs` expense amounts.
- **Empty day** is one 28px row: "no expenses" left, `+` right, the *whole row* is the
  link. Merged from an earlier two-row version, which doubled empty days to 56px —
  fatal when 22 of 30 days are empty (ticket 01).
- **Spending day** gets a dedicated full-width `+` row at the foot of its expenses
  column, `justify-end` so its glyph aligns with the empty days' `+` down the page.
- **Empty days reserve the total column's width** with a spacer div, so the rails
  never shift horizontally between day kinds.
- **Month transitions** (Jul 1 → Jun 30) get a `border-gray-200` divider matching the
  rails; all other day dividers are `border-gray-50`. Computed by comparing the
  `YYYY-MM` prefix of adjacent dates — a template cannot see the previous item from
  inside `range`, so this is a Go helper over the group slice, and the loop is
  `{{range $i, $g := .DailyGroups}}`.
- `divide-y` on the wrapper was **removed**: `.divide-gray-50 > * + *` outranks a
  plain `border-gray-200` class, so the month divider would have been silently
  overridden. Borders now sit on each day.

### Edit and delete

Rows link to `/expenses/edit/{id}`. **Delete is demoted to the edit page** — a ledger
row has no actions column, and an always-visible `✕` (prototyped as variant E) made
the row unreadable on a phone. This is a real behaviour change from the card, which
got Edit and Delete free from `DataTable`. Deleting now costs one extra tap.

### Consequence — the daily view loses its only island

Confirmed on the running prototype: the ledger renders **0 `data-table-root` nodes**,
against 8 on the unchanged control page. `daily.html` is the sole reason `data-table`
is registered (`handlers_html.go:288`).

Expenses become server-rendered HTML. **The daily view will work without JavaScript
for the first time** — today it renders a `<noscript>` message instead of any expense.
This invalidates assumptions in tickets 06 and 07; both updated.

### Two things this design does NOT settle

- **Formatting.** Totals render `RM1223.10` — `printf "%.2f"`, no thousands
  separator, carried over from the card (`daily.html:38`). Pre-existing; not fixed.
- **Where the gap-filling comes from.** The prototype used a throwaway
  `prototypeFillGaps` walking back 30 days from today. The real query is ticket 04.
