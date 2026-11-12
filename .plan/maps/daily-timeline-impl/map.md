# Daily Timeline — implementation

## Destination

The daily-timeline spec ([`.plan/daily-timeline/spec.md`](../daily-timeline/spec.md))
implemented end to end: the Daily View is date-indexed per the spec, all named tests
pass, and the architecture docs reflect what shipped.

## Notes

**This map carries execution.** Every ticket is a `task` that delivers working code,
not a decision — all decisions were made on the
[daily-timeline planning map](../daily-timeline/map.md) and synthesized into the spec,
which is the single source of truth here. Do not re-litigate a decision; if
implementation exposes one as wrong, mark the *planning* ticket undermined and raise
it, rather than quietly deviating.

Every session: read the spec in full, then this map, then your ticket, then the
approved asset(s) the ticket names. Use `CONTEXT.md`'s vocabulary. The `review-code`
skill (in `.pocock-skills/`) reviews the diff before commit; the `verify`/`run` skills
drive the real page — the ledger's three alignment invariants are invisible to tests
by design, so eyeball the rendered page against the `.approved` assets.

Per-ticket checks: `go test ./...`, `go vet ./...`, `cd ui && npx tsc --noEmit`
(and `cd ui && npx vitest run` when TS changes). Tests land **before** implementation
where the spec says so.

Environment: `make dev` runs air (:8080) + Vite (:8081); the Go server caches
templates at startup, so restart after template edits. The user's dev database
(`~/.expensif/expenses.db`) must not be seeded — prototype against a scratch DB via
`DATA_DIR=<tmp> PORT=8090 go run ./cmd/server`. The wayfinder linter for this map:
`cd ../wayfinder && go run ./cmd/wayfinder-maps lint ../expensif/.plan/daily-timeline-impl`.

## Decisions so far

<!-- one line per resolved ticket: gist + link -->

- [The date-indexed service, wired end to end](./tickets/01-date-indexed-service.md) —
  `DailyGroupsInRange` + `UpcomingGroups` shipped tests-first and wired into
  `HandleDaily` with the clock seam and the `NoExpensesEver` flag; the old card
  template renders the gap-filled window until the ledger lands. The handler's
  400-mapping exists but is only reachable via `/daily/older` (ticket 03's test).

- [The ledger](./tickets/02-the-ledger.md) — the whole visual layer shipped from the
  three assets: one `day-entry` partial for both day kinds and both views (today and
  the future derived from the day's date, never threaded), the overflow row placed
  *above* the days it summarises (the asset's sketch order lost to the prose), month
  breaks via a `monthBreak` helper, `data-day` as the partial's machine-readable
  marker, and `data-table` gone — the daily view is JS-free. Ticket 04's failed-save
  gap closed: `return` rides the edit form round-trip. Verified against the assets on
  scratch DBs, including empty, dormant, and future-heavy accounts.

- [The older-days endpoint and the scroll island](./tickets/03-older-days-endpoint-and-island.md) —
  the scroll's whole rulebook is one server-side function (`olderWindow`), so the island
  (0.69 kB, no React) owns no stop condition and does no date maths; a new `day-rows`
  partial gives page and fragment one implementation, `monthBreak` becoming a rule about a
  *pair* of days so the seam divider is correct in an appended window. The foot's states
  hide by plain CSS, not Tailwind's `data-` variants — **the Play CDN is JavaScript**, so
  with JS off the foot had been printing all four states at once. An over-long range is a
  400. Verified live in a browser: the walk, the terminal, Retry, and a JS-less first window.

- [The edit page's danger zone](./tickets/04-edit-page-danger-zone.md) — the approved
  markup shipped verbatim as a sibling form; `PageData.ReturnTo` carries `?return=`
  through `HandleEdit` to the hidden field, and `localPath` gates it at the redirect —
  **one gate, on the only line that can send a browser off-origin**, rather than
  validating at both ends. Seven-case table test; verified live against a hostile
  `?return=`. Leaves one recorded gap for the ledger ticket, which is what starts
  *sending* `?return=`: the path does not survive a failed save.

- [Architecture docs catch up with what shipped](./tickets/05-architecture-docs.md) —
  ADR 0001 gains a note (not a new ADR) that the scroll island is the first non-React
  island and why that doesn't reverse the decision; `AGENTS.md`'s "React Islands" line
  softened to "mostly React, plus one vanilla-TS island"; `CONTEXT.md` redefines the
  Daily View as date-indexed and adds **Ledger**, **Rail**, **Window**, **Upcoming**
  (and widens **Island** off "React component"). The leftover sweep found nothing —
  earlier tickets already deleted the prototypes, dropped the daily data-table, and
  replaced the dead `{{if .DailyGroups}}` branch. Docs only; suite green untouched.

## Not yet specified

<!-- Empty by construction: the planning map cleared all fog before this map was charted. -->

## Out of scope

Everything the spec's **Out of Scope** section rules out — collapsing empty-day runs,
repairing garbage date rows, the data-table hydration bug, number formatting, the
Calendar View handler refactor, any List View change, the unreachable back-link. One
home: [the spec](../daily-timeline/spec.md); not restated here.
