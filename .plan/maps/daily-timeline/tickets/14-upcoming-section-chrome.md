---
type: prototype
blocked_by: [03, 08]
assets: [../assets/upcoming-continuation.html.approved]
---

# The Upcoming section's chrome

## Question

What does the Upcoming section *look* like?

[What happens to expenses dated after today](./03-future-dated-expenses.md) fixed its
**content**: future-dated expenses render in an "Upcoming" section above the timeline,
grouped by day, **not** gap-filled (no walk into next year), ordered so the furthest-out
day sits at the top and the day nearest today sits last, flowing into today's row below.
It fixed none of its chrome. `UpcomingGroups(ctx, after)` already exists as a decision
([ticket 04](./04-date-indexed-daily-groups.md)); nothing renders it.

The raw material is [ticket 08](./08-day-entry-ledger-redesign.md)'s ledger and its
approved markup ([`assets/day-entry-ledger.html.approved`](../assets/day-entry-ledger.html.approved)).
The constraint that shapes everything: **the common case is empty.** Most users have no
future-dated expenses, so whatever this section is, its usual state is nothing at all.

This is a prototype ticket: draw it against the approved ledger and react.

Resolve, one at a time:

- **The empty case first, because it is the normal case.** Does the section vanish
  entirely (no heading, no divider, no reserved space), or does it persist as a
  zero-height affordance? Vanishing is almost certainly right — but say it, because the
  timeline's top edge then changes shape depending on data, and
  [ticket 12](./12-todays-row.md) is deciding whether "top row" means "today".
- **Heading and divider.** Ticket 08's ledger has exactly one horizontal rule in its
  vocabulary — the month-break divider. Does "Upcoming" reuse it, or does a section
  boundary need a heavier mark than a month boundary? Two competing dividers a few rows
  apart is a real risk when an upcoming expense sits in next month.
- **Do upcoming days carry a `+` row?** Ticket 01's empty ledger line always shows `+`.
  But Upcoming is *ungapped* — an empty future day does not exist in it, so there is no
  empty row to carry a `+`, and there is no affordance to add an expense on a specific
  future day from this section. Is that a gap, or correct (that is what the calendar and
  `?date=` are for)?
- **Does it collapse when long?** A user who logs a year of rent has twelve upcoming
  days pushing today's row below the fold — inverting the page's whole purpose. A cap, a
  "show more", or nothing?
- **The two rails.** Ticket 08's rails are described as *unbroken*. Does Upcoming share
  them (one continuous rail through the section boundary into the timeline), or is it a
  separate rail system? This is the decision that makes Upcoming feel like part of the
  ledger or a box sitting on top of it.

## Answer

**There is no Upcoming section.** The ledger simply continues past today into the future,
ungapped. No heading, no divider, no panel, no card, no tinted background. Approved markup:
[`assets/upcoming-continuation.html.approved`](../assets/upcoming-continuation.html.approved).

Three variants were built on the real daily view behind `?variant=`, against ticket 08's
approved ledger and real data, then thrown away:

- **A** — Upcoming as a labelled *region* of one ledger: a gutter label and a
  `border-gray-200` boundary. **Rejected**, and the prototype is what killed it: that
  boundary is the same weight as the ledger's month divider, so an upcoming expense in
  next month (seeded: August rent) puts two grey lines within a couple of rows of each
  other. The collision the ticket predicted is real and visible.
- **B** — Upcoming as a detached tinted panel with its own rails, heading, day count and
  total. **Rejected**: it makes the future a separate *object*, and the page is one ledger.
- **C** — no section at all. **Wins.**

### What marks a future day — exactly two things

1. **The date is tinted** (`text-blue-500`) rather than `font-semibold text-gray-900`.
2. **Today's row is distinguished** — and that is the *only* thing separating "hasn't
   happened yet" from "already spent".

### This constrains ticket 12, and that is now written into it

With no section chrome, today's row **is** the boundary. So
[Today's row](./12-todays-row.md) no longer decides *whether* today is visually
distinguished — this ticket requires it. Ticket 12's question narrows to **how**. Its body
has been updated to say so; the alternative (letting the future tint carry the boundary
alone) was put to the user and rejected, because on a page with no future expenses — the
common case — the tint never appears, so the boundary would exist only on the pages that
don't need it.

### The + row stays on future days

**Overridden from the prototype**, which omitted it. A future day is an ordinary day, and
C's whole premise is that the ledger just continues — so the add affordance continues too.

The argument *against* (raised and rejected): Upcoming is ungapped, so a `+` appears on
August 1 only because rent already sits there, while August 2 cannot be added to at all —
an affordance that exists as an accident of what is already logged. The user took the
consistency of the row over that objection. Adding to an arbitrary future day remains the
calendar's and `?date=`'s job; both already reach every day in ±1 year.

### The overflow row — Upcoming is capped at 3 days

Upcoming sits **above** today, so a long one pushes the page's anchor off screen. This was
not hypothetical: with a seeded year of rent (eight future payments to March 2027), the
uncapped variant put **fourteen expense rows before today** and today below the fold.

So: render at most the **three upcoming days nearest today**. The rest collapse into a
single 28px ledger row — `later │ 7 more upcoming days → │ $9,800` — placed *above* the
days it summarises, because further-future is further up. It links to the calendar.

It is a **link, not a disclosure.** Expanding in place needs JavaScript, and the daily view
has none but the scroll island ([ticket 06](./06-day-card-chrome-drift.md)).

Rejected: a **horizon cap** (only show the next 14 days). It looks tidiest, but future
expenses would vanish from the daily view with nothing hinting they exist — that changes
[ticket 03](./03-future-dated-expenses.md)'s *content* decision, not its chrome, and would
have marked 03 undermined. Nothing is hidden without a trace; the overflow row states the
count and the total.

### The empty case falls out for free

No future expenses — the common case — means no future days render, no overflow row
renders, and **nothing at all sits above today**. There is no chrome to hide because there
is no chrome. This is the strongest argument for C: A and B both had to answer "what does
the heading do when it's empty", and C never has to ask.

### Rails and dividers

Unchanged from ticket 08, because future days *are* ledger days: the two rails run
unbroken from the furthest-future day to the oldest day in the window, day dividers stay
`border-gray-50`, and the `border-gray-200` month divider keeps its meaning — it is now the
only heavy horizontal line on the page.
