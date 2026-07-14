# Export to CSV — a new Export page

## Destination

A plan-only spec at [`.plan/export-csv/spec.md`](./spec.md) describing an **Export
page** whose one job is to download the account's expenses as a CSV. Detailed enough
that one fresh session (or a follow-on `export-csv-impl` map) can build it end to end.
Execution is *not* in this map — decisions only, per the wayfinder default.

Three forks were settled while charting and are premises here, not open tickets:

- **Scope.** The page filters by **date range plus category and payer** before export
  — not a bare "export everything" button.
- **Currency.** Each row carries **both** the original `Amount`+`Currency` *and* a
  `ConvertedAmount` in the preference currency. CSV is cheap on columns; hiding either
  figure was rejected.
- **Destination shape.** Plan-only. Code lands later under a separate `-impl` map, the
  way `daily-timeline` → `daily-timeline-impl` ran.

## Notes

Domain: Go server-rendered templates + React Islands (partial hydration), SQLite,
single binary. Read before deciding anything:

- `internal/domain/models.go:5` — the `Expense` row. `Amount`+`Currency` are the entered
  figures; `ConvertedAmount` and `PaidByName` are **computed at render time**, not stored.
  So an exporter must run the same conversion/naming the views do, not read columns off SQL.
- `internal/service/service.go` — the query seams that already exist: `ListExpenses(limit)`,
  `ExpensesInRange(start, end)` (`:99`), and the currency machinery
  `GetRatesForConversion` (`:299`) / `ConvertExpensesTotal` (`:330`). **No new SQL is
  likely needed for the date range**; category/payer filtering is the net-new part.
- `internal/web/handlers_html.go` — `HandleList` (the `/expenses` view) ships **raw rows
  to a `data-table` React island and filters entirely client-side**. There is therefore
  *no* server-side filter concept to reuse — ticket 01 is inventing one.
- `internal/web/server.go:21` — routes are plain `mux.HandleFunc` lines. Adding
  `GET /export` and a download route sits alongside `/calendar`, `/expenses`.

Skills every session should consult: `codebase-design` before adding the filtering seam
to the service (ticket 01); `prototype` for the page UI (ticket 05); `domain-modeling`
whenever a term firms up — add **Export** to `CONTEXT.md` when it does.

Standing preference: server-render unless a decision *forces* an island. An Export page
is a form plus a download link — it should need no island. Ticket 07 confirms that
against ADR 0001 rather than assuming it.

## Decisions so far

<!-- one line per resolved ticket: gist + link. -->

- [The server-side filtering seam](./tickets/01-filtering-seam.md) — one `ExpenseFilter`
  value (range + optional category + payer, zero-values = "all") drives a single deep
  `ExpensesMatching(ctx, filter)` query at the repository; SQL does the payer/category
  filtering via the join that already exists. Seam returns **raw** rows — the handler
  converts, as every view does — so ticket 02's rate questions stay out of the query.
  `ListExpensesInRange` becomes a one-line delegate, collapsing the duplication without
  touching daily-timeline's callers. Export is the only filtering caller; the List view is
  untouched, out of scope.

- [The rate date and conversion basis](./tickets/02-rate-date-and-conversion-basis.md) —
  **per-expense-date** conversion via a new `RatesOnOrBefore(base, date)` (nearest stored
  rate on-or-before the row's date), falling back to the latest set when nothing precedes.
  The effective rate date becomes a **per-row `rate_date` column** (the basis varies, so it
  must); an unconvertible currency leaves converted **and** rate_date blank, never `0`.
  Chosen over the single-latest-set basis with the fallback's honesty cost accepted and
  made visible by the column. No historical-*fetch* needed, so no new ticket; ticket 01
  stands (it deferred the rate choice here).

## Not yet specified

<!-- in-scope fog too dim to ticket until the route and encoding land -->

- **Streaming vs buffering a large export.** Whether a multi-thousand-row account needs
  a streamed response (and a service that yields rows) or a buffered file is fine. Hangs
  on the route's shape and the row-generation seam. <clears-with: 04>
- **Locale and the decimal separator.** A CSV opened in a spreadsheet configured for a
  comma decimal separator mis-parses `12.50`. Whether the spec cares, and whether that
  interacts with the delimiter choice, only sharpens once encoding is decided.
  <clears-with: 06>

## Out of scope

- **Formats other than CSV** — Excel/`.xlsx`, JSON, PDF. The destination is CSV; another
  format redraws it as a fresh effort.
- **Scheduled or recurring exports**, and **emailing/uploading** the file anywhere. The
  page produces a download on demand, nothing more.
