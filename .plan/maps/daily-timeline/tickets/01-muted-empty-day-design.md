---
type: prototype
blocked_by: []
---

# Muted empty-day card design

## Question

What does a day with no expenses look like, and what is its add affordance?

`templates/daily.html:44` already holds a first draft — a full card with an italic
grey line and a primary "Add one" button. It was written for the `?date=` single-day
path, where it is the only thing on the page. It has never been seen in a stack of
twenty.

Make the cheap concrete artifact and react to it. The tension to resolve: a day card
today is a bordered white panel with a header, a `data-table`, and a footer total. An
empty day has none of those. If it keeps the same chrome it is visually loud —
twenty of them drown the days that matter. If it sheds the chrome it stops reading as
a day at all.

Decide, against something you can actually look at:

- Full card, thin row, or bare date line?
- Is the add affordance a button, a hover-revealed `+`, or the whole row being a
  click target to `/expenses/new?date=`?
- Does the affordance survive on mobile, where hover does not exist?
- Does a non-empty day keep its own footer "+ Add expense" button
  (`daily.html:41`), or does that move/go?

Prototype at least two treatments in the template and view them against a seeded run
of empty and non-empty days. The output is the chosen markup, linked as an asset.

Constraint from the map: no islands. This must be static markup and CSS.

## Answer

**Variant C — the ledger line — wins.** An empty day is not a card. It is a single row:
a fixed-width date gutter (`tabular-nums`, so dates align down the column), a muted
"no expenses" label, and a **persistent circular `+` button** on the right.

Chosen against variants A (muted card, same chrome as a real day) and B (thin row,
whole row clickable) rendered live at `/?variant=` on a seeded 30-day window.

Why C:

- **The add affordance is always visible.** B's cue was hover-only; the daily view is
  mobile-first (`mobile-pad-start`, `sm:` breakpoints throughout) and hover does not
  exist there. C's `+` is a real tap target at all widths.
- **A is too loud.** With a 30-day window, **22 of 30 days were empty** in the seeded
  data — 73%. Twenty-two dashed cards drown the eight days that carry information.
  Empty days are the background of this page, not its content.
- The `+` is the affordance, not the row. The row is inert; only the button navigates
  to `/expenses/new?date=`.

### Consequence — the card must go

C abandons the card metaphor for empty days. Leaving days-with-expenses as cards
leaves two visual species in one list. The user therefore extended the design to the
**day entry itself**: the whole timeline becomes a ledger, no cards.

That is a new decision, tracked as
[Redesign the day entry as a ledger row](./08-day-entry-ledger-redesign.md), and it
may remove the `data-table` island from this page entirely.

### Assets

Prototype: `internal/web/handlers_html_prototype.go`,
`templates/partials/empty-day-prototype.html` (throwaway — folded in and deleted when
ticket 08 lands). Ran on a scratch DB, seeded with 12 expenses across 8 days,
including a six-day empty run.
