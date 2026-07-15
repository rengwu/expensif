---
type: grilling
blocked_by: [01, 03, 06]
---

# Test strategy

## Question

Where the seams are tested, so the impl session knows what to write and when. Waits on the
tickets that define the testable surfaces: the filtering seam (01), the row/column shape
(03), and the encoding contract (06).

- **The filtering seam (01).** Service-level tests for each filter and their combination —
  and the sharp edge that an *unset* filter means *all*, not *none*, so a blank form
  exports everything rather than nothing.
- **Row generation (03).** Assert the concrete column set, order, and formatting against a
  fixed fixture — including a zero-expense result yielding a header-only file.
- **The encoding contract (06).** A hostile `Description` (comma + quote + newline)
  round-trips through a real CSV reader unchanged; the BOM/delimiter/line-ending rules hold.
  Test through `encoding/csv`'s reader, not by eyeballing bytes.
- **The route (04).** A bad/reversed range is a 400, not a silent empty file; the
  `Content-Disposition` filename matches the convention. Follow the daily-timeline
  precedent of testing the handler in-package (`server_test.go`).
- **Tests-first?** State which tests land red before implementation, echoing the
  daily-timeline strategy where the first gap-fill test led the code.

Pin the seams, not the assertions on rendered HTML — the page (05) is prototyped and
eyeballed against its `.approved` asset, not asserted.

## Answer

**Four test seams, each owning one concern; logic pushed to fast/pure surfaces so the
one HTTP test stays thin — the daily-timeline strategy (ticket 10) applied to a CSV
byte stream instead of an HTML fragment.** Two design refinements fell out of asking
"where does this get tested," and both preserve the premises they touch (neither ticket
is undermined):

- **01 refined, not broken.** 01 said "the handler converts." Conversion + `[]string`
  building are *factored into a pure package function* — `buildRecords(rows, resolve,
  pref) [][]string` — rather than left inline in the handler. Still the web layer, still
  outside the query seam (01's actual premise: `ExpensesMatching` returns raw rows and
  stays a pure read). What changed is only *inline code → named testable function*.
- **02 refined, not broken.** 02 said backfill "lives in the handler layer." The
  fetch-missing/upsert orchestration is *factored into a discrete service method* —
  `EnsureRatesForRange(ctx, from, to)` — that the handler calls before converting. 02's
  mechanism (Timeseries fetch, upsert, hard-fail, nearest-on-or-before repo method) is
  untouched; only the orchestration's home moves, so it can be mock-counted.

### The seam map

| Seam | Test surface | What it asserts | Tickets |
|---|---|---|---|
| **Service** | `ExpensesMatching` over `expenseRepoStub` | each filter, filter combination, and the sharp **unset = all** case (a zero-value `ExpenseFilter` returns every row, so a blank form exports everything) | 01 |
| **Service** | `EnsureRatesForRange`, `mockRateClient.Timeseries` **counting calls** | fetches only the dates the store lacks; a 2nd export over the same range makes **zero** calls (cache); a `Timeseries` error → the method returns an error | 02 |
| **Pure** | `buildRecords(rows, resolve, pref)`, fake resolver | per-date convert; **KWD → 3 decimals**, **JPY → 0** (`FormatFloat(v,'f',-1,64)`); unconvertible currency → **blank** `amount_converted` + `rate_date` but present `currency_converted`; payer-less → blank `paid_by`; header is record 0; **zero rows → header-only** | 02, 03 |
| **Repository** | `RatesOnOrBefore` over **in-memory SQLite** | a **weekend expense resolves to the prior trading day** (Sat → Fri). This is a **net-new Layer-3 seam** — the first repository test in the codebase (the 2026-06-02 testing-strategy doc lists in-memory SQLite repo tests as planned-but-unbuilt; this builds the first) | 02 |
| **httptest** | `GET /export.csv` via `httptest.NewRecorder` against the export handler (no renderer — `export.csv` has no template) | **400** on unparseable / inverted range; **503** when `EnsureRatesForRange` errors; **200** header-only on zero matches; `Content-Disposition` filename for both the ranged (`expensif-{from}_{to}.csv`) and open-ended (`expensif-expenses-{today}.csv`) cases; **one** end-to-end round-trip of 06's hostile field (`He said "hi", then left` + embedded newline) parsed back through `csv.Reader` — proving BOM strip, CRLF terminators, and quote-doubling on the *actual bytes*, which is the only place they exist | 04, 06 |

### Why the split falls this way

- **The pure seam can stay pure only because the rate lookup is injected.** 02's
  nearest-on-or-before is a *SQL* method (`RatesOnOrBefore`), which a pure function
  can't run. So `buildRecords` takes an injected `resolve(date) → (rates, rateDate)`;
  production backs it with the repo, tests pass a canned fake. The weekend→Friday rule
  therefore can't be a `buildRecords` test — it's proven once at the repository seam
  where the SQL lives. (Two alternatives were weighed and rejected: doing the walk-back
  in Go inside `buildRecords` — dodges the SQLite seam but moves logic out of 02's repo
  method; and pre-resolving rows in the handler — splits 02's store-lookup across two
  homes.)
- **The encoding contract is only real on the written bytes.** `buildRecords` emits
  `[][]string` records — it does *not* apply the BOM, CRLF, or `csv.Writer` escaping
  (those are 04's streaming writer). So 06's contract is verified exactly once, in the
  httptest, against the real response body. `buildRecords`'s job is only to produce a
  record that *contains* the comma/quote/newline; the writer's job is to escape it.
- **The 503 is asserted at two levels, deliberately** — the service test proves
  `EnsureRatesForRange` returns an error when rates can't be had; the httptest proves the
  handler maps that error to 503. Same shape as daily-timeline's `ErrInvalidRange` →
  400 (logic at the service, status at the handler).

### Tests-first, and the first red

Tests are written **before** the implementation, throughout — tickets 01–06 already
designed every interface, so (as in daily-timeline) an implementation-first order has
nothing to discover.

The leading red test is **`TestBuildRecords_ColumnsFormattingAndBlanks`** (in the web
package): one fixed fixture — a KWD row (3 decimals), a JPY row (0), an
unconvertible-currency row, and a payer-less row — plus a fake resolver, asserting the
exact `[][]string` with the header as record 0. It **cannot compile until `buildRecords`
and its signature exist** — that failure is the red — and it pins the export's central
value (expenses → correct rows) in one shot, mirroring daily-timeline's
`TestDailyGroupsInRange_GapFillsEmptyDays`.

### Mock / stub extensions the impl session will need

Facts the `-impl` map depends on, all extending scaffolding that already exists:

- `RateFetcher` (interface, `service.go:25`) grows **`Timeseries(ctx, base, start,
  end)`**; `mockRateClient` (`service_test.go:13`) grows a **counting** `Timeseries`
  double so the fetch-only-missing / cache cases are call-count assertions.
- `expenseRepoStub` (`service_test.go:485`, already carries `create` + `listInRange`)
  grows a **`matching`** field for `ExpensesMatching`, mirroring the existing pattern —
  no second stub invented.
- The rate repository grows **`RatesOnOrBefore`**, and its test is the codebase's first
  **in-memory SQLite** repository test (spin up `:memory:`, migrate, insert Friday's
  rate, assert Saturday resolves to it).

### What is NOT tested

- **The Export page HTML (05)** — prototyped and eyeballed against its `.approved`
  asset, never asserted, exactly as the ticket instructs.
- **Chrome, class names, copy** anywhere. The httptest asserts CSV *bytes* (the
  machine-readable contract) and status/headers, never markup — the narrowed, not
  broken, version of the 2026-06-02 doc's "don't assert on template HTML" rule, the same
  narrowing daily-timeline made.
- **The rate client's real HTTP call** to Frankfurter — a thin wrapper, mocked at the
  `RateFetcher` seam (the doc's "What NOT to Test" §, still true).
- **Timezone/DST** — irrelevant here; that was daily-timeline's concern. Export's range
  is user-supplied query params, not an anchored "today."
