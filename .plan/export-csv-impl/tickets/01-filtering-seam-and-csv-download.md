---
type: task
blocked_by: []
---

# The filtering seam and the streaming CSV download

## Question

Deliver the spec's **filtering seam** (01), **column set / row formatting** (03), **download
route** (04), and **encoding contract** (06) as one tracer bullet: a real, correctly-encoded
CSV downloadable by URL. Conversion is deferred — this ticket ships the ten-column file with
`amount_converted`/`rate_date` **blank**, so the encoding, route, filtering, and record-building
are proven in isolation from the rate machinery (ticket 02 supplies the real resolver).

Build:

- **The seam.** `ExpenseFilter{Start, End, Category, PayerID}` (zero-values = "all") driving a
  single deep `ExpensesMatching(ctx, ExpenseFilter) ([]domain.Expense, error)` at the repository
  with a service passthrough. The SQL is the existing range query with two optional predicates
  appended when set; payer/category filtering is pure SQL over the existing `LEFT JOIN users`.
  `ListExpensesInRange(ctx, start, end)` becomes a one-line delegate — daily-timeline's callers
  stay untouched while the duplicated SELECT collapses into `ExpensesMatching`. Introduce the
  **one shared filter→`WHERE` helper** here; tickets 03/04 reuse it so the preview count equals
  the download by construction.
- **The pure record builder.** `buildRecords(rows, resolve, pref) [][]string` — ten columns in
  fixed order (`id,date,category,description,paid_by,amount,currency,amount_converted,
  currency_converted,rate_date`), header as record 0, money via `FormatFloat(v,'f',-1,64)`,
  absence as an empty cell (`paid_by` blank when no payer; `amount_converted`+`rate_date` blank
  when `resolve` yields no rate; `currency_converted` present on every row). `resolve(date) →
  (rates, rateDate)` is **injected** — this ticket passes a no-op stub that always yields "no
  rate", so converted columns come out blank.
- **The download route.** `GET /export.csv?from&to&category&payer`, params mapping 1:1 onto
  `ExpenseFilter`, absent/empty = all. **Streams** at constant memory: `w.Write(utf8BOM)` →
  `csv.NewWriter(w)` (`UseCRLF = true`, comma delimiter) → header record → data rows → `Flush()`.
  Pre-stream error order: unparseable range → **400** (existing `httpDateRangeError`), inverted
  range → **400**, zero matches → **header-only 200**. `Content-Type: text/csv; charset=utf-8`;
  `Content-Disposition` filename `expensif-{from}_{to}.csv`, or `expensif-expenses-{today}.csv`
  when a bound is omitted. (The 503 rate-failure branch is ticket 02's to add.)

Tests come **first**, red, per the spec's Testing Decisions:

- **`TestBuildRecords_ColumnsFormattingAndBlanks`** (the leading red — cannot compile until
  `buildRecords`'s signature exists): one fixture — a KWD row (3 decimals), a JPY row (0), an
  unconvertible-currency row, a payer-less row — plus a fake resolver, asserting the exact
  `[][]string` with the header as record 0.
- **Service:** `ExpensesMatching` over `expenseRepoStub` (grow its existing pattern with a
  `matching` field) — each filter, filter combinations, and the sharp **unset = all** case.
- **httptest:** `GET /export.csv` via `httptest.NewRecorder` — 400 on unparseable and inverted
  range; header-only 200 on zero matches; `Content-Disposition` filename for both ranged and
  open-ended cases; a bare `GET /export.csv` exports everything; and **one** end-to-end round-trip
  of the hostile field (`He said "hi", then left` + embedded newline) parsed back through
  `csv.Reader` — proving BOM strip, CRLF terminators, and quote-doubling on the actual bytes.

Done when: the named tests are green, `ListExpensesInRange` is a delegate with daily-timeline's
callers unbroken, and a browser hitting `/export.csv?from=…&to=…` downloads a real, correctly
escaped CSV whose converted columns are (for now) blank.

## Answer

Delivered as specified; the tests landed first and red
(`TestBuildRecords_ColumnsFormattingAndBlanks` could not compile until `buildRecords`'s
signature existed), then the implementation turned them green.

- `domain.ExpenseFilter{Start, End, Category, PayerID}` (zero-values = "all") drives one deep
  `ExpensesMatching(ctx, ExpenseFilter)` at the repository through a single `filterWhere` helper —
  the shared filter→`WHERE` mapping tickets 03/04 reuse — with a service passthrough.
  `ListExpensesInRange` is now a one-line delegate, so daily-timeline's callers stay untouched
  while the duplicated SELECT collapses into `ExpensesMatching`.
- A pure `buildRecords(rows, resolve, pref)` emits the ten columns (header as record 0, money via
  `FormatFloat(v,'f',-1,64)`, absence as a blank cell, `currency_converted` on every row) with the
  rate lookup **injected**. This ticket passes a no-op resolver, so `amount_converted`/`rate_date`
  ship blank; ticket 02 supplies the real one. Conversion's cross-rate was factored into a pure
  `service.CrossRate` so the builder reuses the views' exact math.
- `GET /export.csv` streams BOM → CRLF `csv.Writer` → header → rows, with every status
  (400 unparseable/inverted, header-only 200 on zero matches) decided pre-stream; the 503 rate
  branch is ticket 02's.
- Verified live: a real download carried the BOM, CRLF, and a quote-doubled hostile field
  round-trip.

- **For ticket 02:** the resolver seam is `resolve(date) → (rates, rateDate)`; wire the real
  historical lookup behind it, and slot the 503 rate-failure branch into the pre-stream error
  order (step 3, after the range 400s).
