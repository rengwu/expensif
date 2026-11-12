---
type: grilling
blocked_by: [01, 04]
---

# Contain the day-card chrome drift

## Question

[Window size and how older days load](./02-window-size-and-pagination.md) put the first
30-day window in `daily.html` (Go template) and every window below it in a React
island. Day-card chrome — the date header, `humanDate` subtitle, footer total, add
affordance, and the muted empty-day treatment from ticket 01 — is therefore written
twice, in two languages, and must render identically at the seam. A user scrolling
past day 30 must not see the cards change.

This is an accepted cost, not a mistake. The job is to contain it, not undo it.

Note what is *not* at risk: the tables. `templates/partials/data-table.html` emits no
table markup at all, only a `<div data-table-root>` and JSON props — the React
`DataTable` renders every table on this page already. Only the chrome duplicates.

Resolve:

- Is the duplication load-bearing enough to warrant a test that renders one
  `DailyGroup` through both paths and asserts the same fields appear? Or is a visual
  check at the seam enough?
- Does currency conversion produce the same string on both sides? The Go path formats
  with `printf "%.2f"` and `$.CurrencySymbol` (`daily.html:37`); the React path would
  format in TSX. Rounding and symbol placement are easy to diverge on.
- `formatDate` / `humanDate` are Go template funcs (`renderer.go:77,99`) taking a
  timezone. What renders the date in the island — a JS reimplementation, or does the
  JSON endpoint ship pre-formatted display strings alongside the raw date?
- Cheaper alternative worth pricing before accepting: does the endpoint return
  **display-ready** `DailyGroup`s (formatted date, formatted total, formatted label)
  so the TSX card is a dumb renderer and all formatting stays in Go?

---

## Update — after ticket 08 (the ledger)

The card is gone. What duplicates across Go template and TSX is now the **ledger**:
five columns, two rails, the 28px row unit, the month-break divider, and the expense
rows themselves — which the card never duplicated, because `DataTable` rendered them.
The drift surface **grew**, not shrank.

Two facts that reframe this ticket:

- **The repo already accepts this exact trade.** `templates/partials/button.html:1-3`
  opens with *"keep in sync with `ui/src/components/Button.tsx` — both must use
  identical Tailwind class mappings for variant + size."* Go-template/TSX duplication
  is a documented convention here, not a novel hazard.
- The ledger's alignment rests on three invariants (see ticket 08's answer) that are
  invisible in a screenshot and easy to break in a reimplementation — `self-start` on
  the expenses column, padding on rows not columns, explicit `leading-4`. A TSX
  reimplementation that "looks right" in isolation will drift at the seam.

That strengthens the case for the cheaper option this ticket already floated: have the
JSON endpoint return **display-ready** groups (formatted date, formatted total,
formatted amounts) so the TSX side is a dumb renderer and all formatting logic stays
in Go. Only the class strings duplicate then, exactly as `button.html` does it.

---

## Update — after ticket 07 (the island's contract)

**That cheaper option is no longer this ticket's to price. It was taken.**
[Ticket 07](./07-scroll-island-contract.md) specifies `GET /api/daily` as returning a
display-ready web DTO: `humanDate`, and a formatted, already-converted `total`. Go's
`humanDate` and `printf "%.2f"` are the only formatters of any date or number on the page.

Two of this ticket's four bullets are therefore answered — the endpoint *does* return
display-ready groups, and the island reimplements neither `formatDate`/`humanDate` nor the
currency formatting. Rounding and symbol placement can no longer diverge at the seam.

What is left is narrower than when this was written:

- **The class strings still duplicate**, and they carry the ledger's three alignment
  invariants (`self-start` on the expenses column, padding on rows not columns, explicit
  `leading-4`) — the ones ticket 08 flagged as invisible in a screenshot. How do
  `daily.html` and the TSX ledger stay in sync? `button.html`'s answer is a comment.
- **Does the both-paths test still earn its keep?** Ticket 07 removed the formatting drift
  such a test would most naturally have caught, which weakens the case for it. Ticket 10
  owns test strategy — decide here whether this seam warrants more than a visual check, and
  hand 10 the requirement rather than the test.
- The island renders **expense rows**, which the card never duplicated because `DataTable`
  drew them. That surface is still yours.

---

## Answer

**There is no drift to contain. The duplication is deleted rather than managed.**

The endpoint serves **rendered HTML from the same Go partial that renders the first
window**, and the island appends it. The ledger has exactly one implementation, in one
language. This ticket's four bullets — class-string sync, the currency-formatting seam,
`humanDate` in JS, the display-ready payload — are not answered. They stop existing.

### What unlocked this

[Ticket 02](./02-window-size-and-pagination.md) rejected HTML fragments for exactly one
stated reason: `data-table.tsx` calls `init()` at module load and `querySelectorAll`s the
document, so `[data-table-root]` nodes appended later would never mount.

**That reason was entirely about `data-table`, and [ticket 08](./08-day-entry-ledger-redesign.md)
removed `data-table` from this page.** An appended ledger day contains no islands at all:
every row in the approved markup is a plain `<a>`, and delete was demoted to the edit page.
There is nothing to mount, so there is no reason to ship JSON and re-render it in React.

This is the same move ticket 07 made for the island itself — a decision resting on a
premise ticket 08 destroyed — applied to the clause immediately below it in ticket 02's
answer, which nobody had re-read against 08.

### The endpoint

`GET /daily/older?start=YYYY-MM-DD&end=YYYY-MM-DD` in `handlers_html.go`, **not**
`handlers_api.go`. It returns `text/html`, not the `apiResponse{Data, Error}` envelope: it
is a template partial served over HTTP, and it has no consumer but this island.

The cursor semantics from ticket 07 survive unchanged, carried as attributes rather than
JSON. The fragment's wrapper div holds `data-next-start` / `data-next-end`, **absent** when
the earliest expense has been reached. The island reads them and never does date
arithmetic — the property ticket 07 was protecting, preserved by different means.

### The foot

`daily.html` renders all four foot states once — sentinel, loading, error-with-retry,
terminal marker — and the island only sets `data-state` on the wrapper; CSS reveals one.

**Zero Tailwind classes and zero markup in TypeScript.** Had the island built its own
spinner and terminal marker, those strings would carry the ledger's 28px rhythm and
`leading-4` pinning, and this ticket's drift surface would have crept back in through the
foot. The terminal marker's text is server-known anyway; the error state, which by
definition cannot be fetched, has to exist client-side regardless of payload format.

### Consequences

- **The island is no longer React.** It is a sentinel, a fetch, an `insertAdjacentHTML`,
  and a cursor — roughly thirty lines of vanilla TypeScript. It still needs a Vite entry
  for bundling. `AGENTS.md` describes the architecture as "React Islands"; this is the
  first island that is not one, and that sentence will want revisiting when the spec lands.
- **Tailwind is a non-issue, verified.** `base.html:9` loads the Play CDN
  (`cdn.tailwindcss.com`), which observes DOM mutations and styles inserted nodes. There
  are no build-time content globs to configure.
- **Month breaks need no cross-window state.** Ticket 08 specified a Go helper comparing
  adjacent groups, which would have been awkward at a window seam. The timeline is
  gap-filled and contiguous, so the day below `D` is always `D-1`, and a month break is
  exactly `D.Day == 1`. It is a pure function of the date.
- **For [ticket 10](./10-test-strategy.md):** the both-paths test this ticket was going to
  demand is now unbuildable and unnecessary — there is one path. What replaces it is a
  much cheaper assertion: that `/daily/older` and `HandleDaily` invoke the same partial.

### What this undermines

Recorded rather than quietly re-decided:

- [Ticket 02](./02-window-size-and-pagination.md) — its "the island fetches JSON and renders
  appended day-cards in React" clause, and its rejection of HTML fragments. The 30-day
  rolling window, the island itself, and termination at the earliest expense all stand.
- [Ticket 07](./07-scroll-island-contract.md) — the display-ready DTO, dropping
  `convertedTotal`, and `mountIsland`/`createRoot` are void. The server-issued cursor, the
  sentinel with one fetch in flight, manual retry on error, and the refusal of a
  `<noscript>` fallback all stand, and are now cheaper.
