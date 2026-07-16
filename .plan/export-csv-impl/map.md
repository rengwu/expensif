# Export to CSV — implementation

## Destination

The export-csv spec ([`.plan/export-csv/spec.md`](../export-csv/spec.md)) implemented end
to end: an Export page filters the account's expenses by date range, category, and payer and
streams a spreadsheet-ready CSV — each row carrying the entered amount *and* an amount
converted at its own-date historical rate — with a live preview of count and sample rows. All
the spec's named tests pass, and the architecture docs reflect what shipped.

## Notes

**This map carries execution.** Every ticket is a `task` that delivers working code, not a
decision — all decisions were made on the
[export-csv planning map](../export-csv/map.md) and synthesized into the
[spec](../export-csv/spec.md), which is the single source of truth here. Do not re-litigate a
decision; if implementation exposes one as wrong, mark the *planning* ticket undermined and
raise it, rather than quietly deviating.

Every session: read the spec in full, then this map, then your ticket, then the approved
asset(s) the ticket names ([`export-page.html.approved`](../export-csv/assets/export-page.html.approved)
for the page/preview tickets). Use `CONTEXT.md`'s vocabulary. The `review-code` skill (in
`.pocock-skills/`) reviews the diff before commit; the `verify`/`run` skills drive the real
page — download an actual file and open it, since the encoding contract (BOM, CRLF, quoting)
and the live preview are only real on the wire.

Per-ticket checks: `go test ./...`, `go vet ./...`, `cd ui && npx tsc --noEmit` (and
`cd ui && npx vitest run` when TS changes). Tests land **before** implementation where the
spec's Testing Decisions say so — the leading red is
`TestBuildRecords_ColumnsFormattingAndBlanks` (ticket 01).

Environment: `make dev` runs air (:8080) + Vite (:8081); the Go server caches templates at
startup, so restart after template edits. The user's dev database (`~/.expensif/expenses.db`)
must not be seeded — prototype against a scratch DB via `DATA_DIR=<tmp> PORT=8090 go run
./cmd/server`. Ticket 02 hits Frankfurter's time-series endpoint on a cold range; mock it at
the `RateFetcher` seam in tests, and don't hammer the live API when eyeballing — a warmed
scratch store needs no calls. The wayfinder linter for this map:
`cd ../wayfinder && go run ./cmd/wayfinder-maps lint ../expensif/.plan/export-csv-impl`.

## Decisions so far

<!-- one line per resolved ticket: gist + link. Empty until the first ticket resolves. -->

- [The filtering seam and the streaming CSV download](./tickets/01-filtering-seam-and-csv-download.md) —
  `domain.ExpenseFilter` (zero-values = "all") drives one deep `ExpensesMatching` at the
  repository through a single `filterWhere` helper (the shared filter→`WHERE` mapping 03/04/09
  reuse), with a service passthrough; `ListExpensesInRange` is now a one-line delegate, daily's
  callers untouched. A pure `buildRecords(rows, resolve, pref)` emits the ten columns (header as
  record 0, money via `FormatFloat(v,'f',-1,64)`, absence as a blank cell, `currency_converted`
  on every row) with the rate lookup **injected** — this ticket passes a no-op resolver, so
  `amount_converted`/`rate_date` ship blank; ticket 02 supplies the real one. `GET /export.csv`
  streams BOM → CRLF `csv.Writer` → header → rows, with every status (400 unparseable/inverted,
  header-only 200 on zero matches) decided pre-stream; the 503 rate branch is 02's. Conversion's
  cross-rate factored into a pure `service.CrossRate` so the builder reuses the views' exact math.
  Verified live: a real download carried the BOM, CRLF, and a quote-doubled hostile field
  round-trip. Leading red was `TestBuildRecords_ColumnsFormattingAndBlanks`, as designed.

- [Per-date historical conversion and rate backfill](./tickets/02-per-date-conversion-and-backfill.md) —
  `rate.Client.Timeseries` fetches Frankfurter's `/{start}..{end}` once (date→target→rate, base at
  1.0); added to `RateFetcher`. `Service.EnsureRatesForRange` backfills only when `rangeCovered`
  (a two-pointer walk over business days, covered when a stored rate sits within a 7-day
  `rateLookback` on-or-before each weekday) reports a gap — so a weekend/holiday reads covered and a
  second export makes zero calls; the fetch widens 7 days before `from` so a leading weekend resolves
  to its prior trading day. `RatesOnOrBefore` (repo) returns the nearest stored set + matched date —
  the codebase's first repository test proves Sat→Fri over in-memory SQLite (`db.Migrate` exported);
  `RateDatesInRange` feeds the coverage probe. The handler slots the backfill at pre-stream step 3
  (error → **503**, no file), then a memoised `resolve` backed by `RatesOnOrBefore` + `CrossRate`
  fills `amount_converted`/`rate_date`; unconvertible currencies blank without failing. Two
  spec-consistent refinements: backfill is driven by the converting rows' own date-span (not the
  `0001..9999` bare range), and skipped entirely when nothing needs converting. Verified on the wire:
  BOM/CRLF, Sat→Fri `rate_date`, unconvertible `ZZZ` blank at 200.

## Not yet specified

<!-- Empty. Every decision is settled in the spec; this map only executes it. Nothing is
foggy — a ticket that exposes a genuinely new question sends it back to the planning map,
it does not open fog here. -->

## Out of scope

<!-- Inherited from the spec's Out of Scope; these never graduate into tickets on this map. -->

- **Formats other than CSV** (Excel/`.xlsx`, JSON, PDF) — a fresh effort each.
- **Scheduled/recurring exports** and **emailing/uploading** the file — the page produces a
  download on demand, nothing more.
- **Wiring the List View (`/expenses`) to the server-side `ExpensesMatching` seam** — the
  seam is built general enough to serve it later, but no code lands for a caller that doesn't
  yet exist.
- **Multi-select category/payer** — the seam is single-value equality.
- **Comma-decimal-locale accommodation** — one canonical US/ISO file; other-locale users
  import via their spreadsheet's text-import dialog (documented, not built).
- **Migrating money from `float64` to integer minor units** — a pre-existing modelling choice
  this effort does not relitigate.
