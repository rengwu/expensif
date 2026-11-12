---
type: task
blocked_by: [02]
---

# The older-days endpoint and the scroll island

## Question

Deliver the spec's **Window and pagination** decisions: `GET /daily/older?start=&end=`
in the HTML handler layer, serving a fragment rendered by the same day-entry partial
as the first window, carrying the server-issued cursor as `data-next-start`/`-end`
(absent at the earliest expense — the terminal rule, final-window clamping included,
lives server-side). The four foot states server-rendered in the daily template and
toggled by `data-state`. The island itself: ~30 lines of vanilla TypeScript — sentinel
`IntersectionObserver`, one fetch in flight, append via `insertAdjacentHTML`, manual
Retry with the observer disconnected on error — plus its Vite entry. Zero Tailwind and
zero markup in TS.

Tests per the spec's seam 2: the fragment through the real renderer via `httptest`,
asserting only `data-next-*` and `data-state`; bad or inverted range → 400. Verify the
scroll live on a seeded scratch DB: append at the window seam (rails unbroken, month
dividers correct), terminal at the earliest expense, Retry on a killed server, and the
first-window terminal for a future-only account.

Done when: scrolling walks back to the earliest expense and stops, errors offer
Retry, and a JS-less load still shows a fully working first window.

## Answer

Shipped as specified. `GET /daily/older?start=&end=` (`handlers_html.go`,
`HandleDailyOlder`) serves `text/html` through a new `Renderer.RenderFragment` — a
page without the page, no base layout — and the island (`ui/src/entries/daily-scroll.ts`,
its own Vite entry, **0.69 kB built, no React chunk**) appends it. Verified live in a
real browser against seeded scratch DBs: two windows fetched, 74 days on the page, the
walk stopping on the earliest expense; Retry recovering from a killed endpoint; a
future-only account never fetching; a JS-less load still rendering all 30 days with
every add and edit link live.

**The scroll's whole rulebook is one function.** `olderWindow(start, earliest)` returns
the next cursor — `[start-30, start-1]`, clamped up to the earliest expense — or *no*
cursor, which **is** the terminal. The island therefore has no stop condition, does no
date arithmetic, and cannot disagree with the server about where history ends: it reads
`data-next-start`/`-end` off the fragment it just appended and re-arms only if they are
there. The same function issues the page's first cursor, so first window and Nth window
are the same rule.

**One ledger, one implementation, including the seam.** A new `day-rows` partial owns
the *sequence* of days and the dividers between them; the timeline and the fragment both
draw through it, so an appended window is byte-for-byte the markup the page would have
rendered. The month divider is a property of a *pair* of days, so `monthBreak` now takes
`(prev, cur)` rather than `(slice, index)`, and a fragment is told the day above its
first row (`PrevDate` = `end + 1 day`, derived — nothing crosses the wire). Screenshots
confirm the rails run unbroken through the seam and the month divider lands correctly
*inside* an appended window.

Three things the spec did not name, each recorded rather than smuggled:

1. **The foot's states hide via a plain CSS rule, not Tailwind's `data-` variants** —
   because the **Tailwind Play CDN is itself JavaScript**. With JS off it never runs,
   `hidden` styles nothing, and the foot printed all four states at once, including
   *"no earlier expenses"* on a page whose scroll had never run. Caught by the JS-less
   check, which is the only reason it was caught. The spec's "the island only sets
   `data-state` and CSS reveals one" holds exactly; only the CSS mechanism changed, and
   zero Tailwind and zero markup still live in TypeScript. (Corollary worth knowing:
   ticket 08's "the daily view works without JS" means *functional* — content, links,
   forms — not *styled*. Nothing in this app is styled without JS.)
2. **A range longer than one window is a 400.** Cursors are server-issued and never span
   more than 30 days, so a longer range is hand-crafted — and the gap-fill would answer
   it with a row per day for as far back as it asked (`?start=0001-01-01` is a 700k-row
   response). This enforces the window contract rather than adding to it.
3. **`ScriptTag` no longer hardcodes `.tsx`** (an entry may name its own extension), and
   a fragment's rows carry `?return=/` rather than `?return=/daily/older?...`, which
   would have stranded a deleting user on a bare HTML fragment.

**For ticket 05 (architecture docs):** the first non-React island has now landed, so
`AGENTS.md`'s and ADR 0001's "React Islands" is no longer universally true — and the
`.ts` island rides a `.tsx`-shaped asset pipeline, which is worth one line.
