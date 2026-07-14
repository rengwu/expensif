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
  **per-expense-date conversion over a persisted historical rate store.** On export, missing
  dates in the range are **lazily backfilled** from Frankfurter's time-series (one call,
  upserted, cached) via a new `Timeseries` client method; if they can't be obtained the
  export **hard-fails** (503, no file) rather than shipping latest-rate guesses. Rows convert
  at the nearest stored rate on-or-before their date (weekends → prior trading day), shown in
  a per-row `rate_date` column; a currency the provider doesn't cover blanks converted +
  rate_date without failing. *Re-grilled: an earlier per-date-with-silent-fallback resolution
  was reopened and replaced — the fallback was inaccurate.* **Brings historical-rate fetching
  into scope**; adds a 503 case to ticket 04 and a per-row type to 03.

- [The column set and row formatting](./tickets/03-column-set-and-formatting.md) — ten
  columns in fixed order (`id,date,category,description,paid_by,amount,currency,
  amount_converted,currency_converted,rate_date`), one `snake_case` header row, `created_at`
  omitted. Amounts via `FormatFloat(v,'f',-1,64)` — exact shortest decimal, correct for
  0/2/3-decimal currencies (`%.2f` rejected as lossy). Absence is an **empty cell**:
  converted+rate_date blank when unconvertible, paid_by blank when no payer. Rows are built
  as `[]string` records in the handler (no rate-date field on `domain.Expense`); ticket 06
  owns escaping and the header write.

- [The download route and delivery](./tickets/04-download-route-and-delivery.md) — two
  routes: `GET /export` (the page, a `method=GET` form) and `GET /export.csv?from&to&category&payer`
  (the download), filters mapping 1:1 onto `ExpenseFilter`, empty param = all. The CSV
  **streams** to the response at constant memory; validation + rate backfill run first and
  own every status — **400** on unparseable/inverted range (reusing `httpDateRangeError`),
  **503** when ticket 02's backfill can't get rates, all **pre-stream**. Zero matches →
  header-only 200, not an error. `Content-Disposition` filename encodes the range
  (`expensif-{from}_{to}.csv`). Buffering weighed and rejected. Resolves the "streaming vs
  buffering" fog.

## Not yet specified

<!-- in-scope fog too dim to ticket until the route and encoding land -->

- **Locale and the decimal separator.** A CSV opened in a spreadsheet configured for a
  comma decimal separator mis-parses `12.50`. Whether the spec cares, and whether that
  interacts with the delimiter choice, only sharpens once encoding is decided.
  <clears-with: 06>

## Out of scope

- **Formats other than CSV** — Excel/`.xlsx`, JSON, PDF. The destination is CSV; another
  format redraws it as a fresh effort.
- **Scheduled or recurring exports**, and **emailing/uploading** the file anywhere. The
  page produces a download on demand, nothing more.
