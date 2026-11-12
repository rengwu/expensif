# Export to CSV — spec

Synthesized from the `export-csv` wayfinder map (`map.md` + tickets 01–09), all of which
are resolved. This is a plan-only spec; code lands under a separate `export-csv-impl` map,
the way `daily-timeline` → `daily-timeline-impl` ran. Uses `CONTEXT.md` vocabulary
throughout and respects ADR 0001 (React Islands).

## Problem Statement

I keep my expenses in Expensif, but I have no way to get them *out*. When I need to file a
reimbursement, reconcile against a bank statement, or hand figures to an accountant, I have
nothing to give them — the data only lives inside the app's Daily, Calendar, and List
views. I want a spreadsheet-ready file of my expenses, narrowed to the ones I care about
(a date range, maybe a single category or a single payer), with each amount shown both as I
entered it *and* converted into my preferred currency — and the converted figure has to be
honest, using the exchange rate that was actually in effect on each expense's date, not
today's rate applied retroactively.

## Solution

A new **Export** page (`GET /export`), reached from the `⋮` overflow menu, whose one job is
to download the account's expenses as a CSV. The page is a single-column, guided two-step
form: pick a date range (presets — *This month / This year / All time* — or a custom pair of
date inputs) and optionally narrow to one category and/or one payer. As the filters change,
a **live preview** shows the exact row count and the first ten sample rows (entered amounts,
not converted), so the size and contents are visible before committing. Submitting the form
navigates to `GET /export.csv`, which streams a UTF-8 CSV the browser saves.

Each row carries **both** the entered `amount`+`currency` and an `amount_converted` in the
preferred currency, converted at the exchange rate in effect on that expense's own date. The
first time a range is exported, any missing historical rates are fetched once from
Frankfurter's time-series endpoint and persisted, so later exports of the same range need no
network call. If the rates for the range genuinely can't be obtained, the export **hard-fails
with no file** rather than shipping inaccurate numbers.

## User Stories

1. As an account holder, I want an Export page whose only job is to download my expenses as
   a CSV, so that I can get my data out of the app.
2. As an account holder, I want to reach Export from the `⋮` overflow menu, so that this
   less-frequent action doesn't clutter the top nav (Daily / Calendar).
3. As an account holder, I want to filter the export by a date range, so that I export only
   the period I care about (a reimbursement month, a tax year).
4. As an account holder, I want date-range presets (*This month*, *This year*, *All time*),
   so that the common ranges are one click, not two manual date entries.
5. As an account holder, I want a *Custom…* toggle revealing two date inputs, so that I can
   export an arbitrary range the presets don't cover.
6. As an account holder, I want to narrow the export to a single category, so that I can pull
   just "Travel" or "Groceries" for a report.
7. As an account holder, I want the category dropdown populated from the categories I have
   actually used, so that I'm not typing free-text or picking a category with no expenses.
8. As an account holder, I want to narrow the export to a single payer, so that I can pull
   just the expenses one person paid.
9. As an account holder, I want a blank/"All" default on category and payer, so that leaving
   them untouched exports *everything* in the range, not nothing.
10. As an account holder, I want a live row count as I change filters, so that I know how big
    the file will be before I download it.
11. As an account holder, I want to see the first several sample rows in the preview, so that
    I can confirm I've got the right expenses before committing to a download.
12. As an account holder, I want the preview count to match exactly what the download
    contains, so that the reassurance isn't a lie.
13. As an account holder, I want the preview to stay responsive as I type (not fire a request
    per keystroke), so that editing filters feels smooth.
14. As an account holder editing a date field, I want a transiently-inverted range to show a
    gentle "Check the dates" hint rather than a hard error, so that mid-edit states aren't
    hostile.
15. As an account holder, I want each CSV row to show the amount exactly as I entered it
    (original amount + currency), so that the file is faithful to what I recorded.
16. As an account holder, I want each row to *also* show the amount converted into my
    preferred currency, so that I can total a mixed-currency export in one column.
17. As an account holder, I want each row converted at the exchange rate in effect on that
    expense's own date, so that historical figures are accurate rather than restated at
    today's rate.
18. As an account holder, I want a per-row `rate_date` column showing which rate date was
    used, so that the file is self-documenting and reproducible.
19. As an account holder exporting across a weekend or holiday, I want those expenses
    converted at the nearest prior trading day's rate, so that non-trading-day rows still get
    an honest, adjacent-day figure.
20. As an account holder, I want the export to refuse (no file, a clear error) if the
    historical rates for my range can't be obtained, so that I never receive a file of
    inaccurate today's-rate guesses.
21. As an account holder with an expense in a currency the rate provider doesn't cover, I want
    that row's converted amount and rate date left blank (not zero), so that a spreadsheet
    `SUM` skips it instead of being silently corrupted — the original amount still carries the
    truth.
22. As an account holder, I want the CSV to include a stable `id` column, so that I can
    round-trip or reconcile rows against the app.
23. As an account holder, I want a `paid_by` column, so that a payer-filtered (or mixed)
    export names who paid.
24. As an account holder, I do **not** want an internal `created_at` timestamp column, so that
    the report isn't cluttered with audit metadata — the expense's own `date` is what matters.
25. As an account holder, I want amounts written as plain, unformatted decimal numbers (no
    thousands separators, no currency symbols), so that the spreadsheet parses them as
    numeric.
26. As an account holder with 3-decimal (KWD) or 0-decimal (JPY) currencies, I want amounts
    written at their true precision (`1.234`, `100`), so that no precision is lost or padded.
27. As an account holder, I want one machine-friendly `snake_case` header row, so that
    re-importing the file needs no column-name fixups.
28. As an account holder opening the file in Excel on Windows, I want accented descriptions
    and currency symbols to render correctly, so that the text isn't mangled.
29. As an account holder whose description contains commas, quotes, or newlines, I want those
    fields correctly quoted and escaped, so that the file doesn't break on import.
30. As an account holder, I want the downloaded file named for its range
    (`expensif-2026-01-01_2026-07-15.csv`), so that I can tell exports apart on disk.
31. As an account holder, I want the download to be a bookmarkable/shareable URL, so that I
    can re-run or share the same filtered export.
32. As an account holder whose filter matches no expenses, I want a valid header-only CSV
    (not an error), so that "zero rows" is an unambiguous, legitimate result.
33. As an account holder who typed a bad or reversed date range into the download URL, I want
    a clear 400 rather than a silent empty file, so that a mistake is obvious.
34. As an account holder exporting a cold (never-before-exported) range, I want the export to
    tolerate the one-time rate fetch's latency and, if the provider is down, fail cleanly
    rather than hang, so that I'm not left waiting indefinitely.
35. As an account holder in a comma-decimal locale, I want a documented note that the file is
    US/ISO format and how to import it via my spreadsheet's text-import dialog, so that I know
    how to open it correctly.
36. As a maintainer, I want the export's central logic (filtering, rate backfill, row
    building, encoding) tested at fast, well-chosen seams, so that the feature is trustworthy
    without asserting on rendered HTML.
37. As a maintainer, I want the domain glossary and ADR 0001 updated to record the Export
    feature and its second vanilla-TS island, so that the docs stay honest about what shipped.

## Implementation Decisions

### The filtering seam (ticket 01)

- **One filter value, one deep query.** Introduce an `ExpenseFilter` value — a required date
  range plus an optional category and payer, where zero-values mean "all":

  ```go
  type ExpenseFilter struct {
      Start, End string // required date range (YYYY-MM-DD, inclusive)
      Category   string // "" = all categories
      PayerID    int64  // 0  = all payers
  }
  ```

  It drives a single repository method `ExpensesMatching(ctx, ExpenseFilter) ([]domain.Expense, error)`,
  with a service passthrough. The SQL is the existing range query with two optional predicates
  appended when set (`AND e.category = ?`, `AND e.paid_by = ?`). Payer filtering is pure SQL —
  the repo already `LEFT JOIN`s `users` and populates `PaidByName`, so nothing new joins.
- **"Unset" is the zero value** — no separate sentinel; this is why the struct beats a
  positional parameter list.
- **The seam returns raw `[]domain.Expense`; the handler converts.** `ConvertedAmount` is not
  populated by the query, keeping it a pure read and confining every rate decision to the
  handler layer. `PaidByName` *is* populated (it rides the existing JOIN).
- **Blast radius stays small:** `ExpensesMatching` becomes the single home of the SELECT+scan;
  the existing `ListExpensesInRange(ctx, start, end)` becomes a one-line delegate
  (`ExpensesMatching(ctx, ExpenseFilter{Start: start, End: end})`), so daily-timeline's callers
  are untouched while the duplicated SQL collapses. Wiring the List View to this seam is
  explicitly out of scope.
- **Export drives three reads off one filter** (refined by tickets 08 and 09): `ExpensesMatching`
  (download), `CountMatching` (preview count), and a `LIMIT 10` sample read — all constructing
  their `WHERE` from the *same* `ExpenseFilter` via one shared predicate helper.

### Rate date and conversion basis (ticket 02)

- **Per-expense-date conversion over a persisted historical rate store** — not the latest rate
  set, and not per-date-with-silent-fallback (an earlier resolution reopened and replaced
  because the fallback shipped inaccurate numbers).
- **The store already has the right shape:** `exchange_rates(base, date, target, rate)` with
  `SaveRates`/`GetRates`. Base stays `USD`; conversion is the existing cross-rate
  (`ConvertWithRates`). We *populate* it, we don't build it.
- **Lazy backfill on export, then cached.** For a range `[from, to]`, compute which dates the
  store lacks; if any, fetch Frankfurter's time-series endpoint once for the whole range
  (`GET /v1/{from}..{to}?base=USD`) and upsert every returned business-day rate. Subsequent
  exports over the same range read the store with no network call. **New rate-client capability:**
  `Timeseries(ctx, base, start, end)` on the `RateFetcher` interface (which today has only
  `Latest`).
- **Failure is a hard fail, no file.** If after the fetch attempt the store still can't supply a
  rate set for a needed date, the export returns 503 and produces no CSV — never a partial file
  of latest-rate guesses.
- **Non-trading days resolve to the nearest stored rate on-or-before** the expense's date, via a
  new repository method `RatesOnOrBefore(base, date) → (rates, matchedDate)` over the now-dense
  store (a Saturday expense correctly uses Friday's rate).
- **Surfacing:** a per-row `rate_date` column (`YYYY-MM-DD`), the date of the rate actually used.
- **Unconvertible currency → blank, not zero, and *not* a hard fail.** If the date's rate set is
  present but doesn't cover the row's currency (Frankfurter covers ~31; the domain accepts any
  string), leave both `amount_converted` and `rate_date` empty. The date's rates were obtained;
  this one currency just isn't quotable.
- **Orchestration factored for testability** (refined by ticket 08): the fetch-missing/upsert
  orchestration lives in a discrete service method `EnsureRatesForRange(ctx, from, to)` the
  handler calls before converting.

### Column set and row formatting (ticket 03)

- **Ten columns, fixed order, one `snake_case` header row (record 0):**

  ```
  id,date,category,description,paid_by,amount,currency,amount_converted,currency_converted,rate_date
  ```

  `id` is the reconciliation/re-import key; `paid_by` because payer is a filter dimension;
  `created_at` is **omitted** as internal audit metadata.
- **Money via `strconv.FormatFloat(v, 'f', -1, 64)`** for both `amount` and `amount_converted` —
  the shortest decimal that round-trips the stored `float64`, plain notation, no thousands
  separator, no imposed decimal places (KWD `1.234` stays `1.234`, JPY `100` stays `100`,
  EUR `12.5` stays `12.5`). `%.2f` was rejected as lossy for 3-decimal currencies. (Money is
  stored as `float64`, a pre-existing modelling choice this effort does not relitigate.)
- **Dates:** `date` emitted as the stored `YYYY-MM-DD` unchanged; `rate_date` is `YYYY-MM-DD`
  or empty.
- **Absence is an empty cell, never a zero or placeholder:** `amount_converted` **and**
  `rate_date` blank when unconvertible; `paid_by` blank when no payer; `currency_converted` (the
  preferred currency) present on *every* row even when the converted amount is blank.
- **Rows assembled as `[]string` records in the handler** — no rate-date field bolted onto
  `domain.Expense`. Factored into a pure function `buildRecords(rows, resolve, pref) [][]string`
  (refined by ticket 08) that takes an injected `resolve(date) → (rates, rateDate)`; the header
  is record 0.

### Download route and delivery (ticket 04)

- **Two download-side routes:** `GET /export` renders the page (a `method="GET"` form);
  `GET /export.csv?from&to&category&payer` streams the download. Both are plain `mux.HandleFunc`
  lines in `server.go` beside `/calendar`, `/expenses`. GET (not POST) so the download is a
  bookmarkable, shareable URL; each param maps 1:1 onto an `ExpenseFilter` field; absent/empty
  param = "all". GET is safe despite the backfill side effect because that only warms the rate
  cache (idempotent upsert), mutating no user data.
- **Delivery streams to the `ResponseWriter`** at constant memory: after the pre-write checks,
  `csv.NewWriter(w)` writes the header record then one row at a time, ending with `Flush()`.
  Safe because every status-bearing failure happens before the first byte. A DB fault
  mid-`rows.Next()` truncates the file (status already sent) — logged and accepted as rare.
  Buffering-then-flushing was weighed and rejected (doesn't scale to an unbounded range).
- **Error order, all pre-stream:**
  1. Unparseable `from`/`to` → **400** via the existing `httpDateRangeError` (`ErrInvalidDate`).
  2. Inverted range (`start > end`) → **400** (`ErrInvalidRange`), reusing the daily-view
     precedent.
  3. Rate backfill runs; rates unobtainable → **503**, no file. (The rate client already carries
     a 10s timeout, so a hung provider fails rather than hangs.)
  4. `category`/`payer` are **not** validated — an unknown value matches nothing (empty result),
     not an error.
- **Zero matches → header-only CSV, 200** (a legitimate export, not an error).
- **Headers:** `Content-Type: text/csv; charset=utf-8`; `Content-Disposition: attachment;
  filename="…"`. Filename `expensif-{from}_{to}.csv` when both bounds given, else
  `expensif-expenses-{today}.csv`. Category/payer are not encoded into the filename.

### CSV encoding correctness (ticket 06)

- **`encoding/csv.Writer` for escaping, never hand-rolled** — it quotes any field containing a
  comma, double-quote, `\r`, or `\n` and doubles embedded quotes, per RFC 4180. The spec states
  this so no one "optimises" it into a `strings.Join`.
- **BOM:** prepend the three bytes `EF BB BF` once, before the header record (write order:
  `w.Write(utf8BOM)` → `csv.NewWriter(w)` → header → rows → `Flush()`), so Excel on Windows
  detects UTF-8. Consequence: Go's `csv.Reader` does not strip a BOM, so re-import/tests must
  strip the leading `﻿` before asserting the header.
- **Delimiter: comma** (default). **Line endings: CRLF** (`Writer.UseCRLF = true`; RFC 4180). A
  field-internal `\n` stays `\n` inside quotes while row terminators are `\r\n`.
- **Locale: one canonical US/ISO file, documented.** Dot decimals + comma delimiter. The app has
  no per-user locale; comma-decimal-locale users import via their spreadsheet's text-import
  dialog (comma delimiter + dot decimal). A semicolon/comma-decimal alternate was rejected.

### Export page UI (ticket 05)

- **A hybrid layout:** a guided two-step single-column form (variant C's layout) carrying a
  **live preview** (variant B). Approved markup:
  [`assets/export-page.html.approved`](./assets/export-page.html.approved).
- **Controls:** date range = **presets-as-links + custom** (*This month / This year / All time*
  chips that server-fill `from`/`to`; a *Custom…* toggle reveals two `<input type="date">`).
  **Category and Payer are single `<select>` dropdowns** (the seam is single-value equality — no
  multi-select); `""`/`0` are the "All …" defaults. The form is `method="GET"`,
  `action="/export.csv"`, emitting exactly `from,to,category,payer`.
- **Category options** come from the account's **DISTINCT existing categories** (category is
  free-text upstream — a net-new `SELECT DISTINCT category` the impl map adds); payer options are
  the existing users.
- **The page carries one island** (`data-island="export-preview"`) — the user explicitly accepted
  it for the live preview. This reverses the effort's standing "a form + link needs no island"
  preference (recorded as a *decided* reversal in ticket 07, not assumed).
- **Nav placement:** the `⋮` overflow menu, beside Preferences — not top-level.
- **Empty/error states belong to the route** (400/503/header-only-200 are `/export.csv`'s); the
  page's only "state" is the preview island's loading/empty/error (ticket 09's contract).

### Live-preview endpoint contract (ticket 09)

- **A raw-only sample behind a third route,** `GET /export/preview?from&to&category&payer`, in
  `handlers_html.go` (it serves HTML, like `GET /daily/older`). Filters map 1:1 onto
  `ExpenseFilter`, identically to `/export.csv`.
- **Raw amounts only — the sharp decision.** The sample shows entered `amount`+`currency`, *not*
  `amount_converted`. Showing converted figures would run ticket 02's per-date backfill (slow,
  503-capable) on every filter change, or show a partly-blank column. The preview's job is
  reassurance about *which rows and how many*. **Consequence: the preview does zero rate work, so
  it can never 503** — the entire provider-failure surface stays on `/export.csv`.
- **Payload: a rendered HTML fragment from a Go partial, not JSON** — a count line plus the sample
  rows, money/dates formatted once in Go by the same `FormatFloat(v,'f',-1,64)` rule the CSV uses.
  The island `fetch`es and does `el.innerHTML = text`; it parses and formats nothing. (A JSON DTO
  would force the TS to re-format and drift from the Go/CSV formatting — the lesson daily-timeline
  ticket 07 was undermined to learn.)
- **Sample size: first `N = 10` rows,** ordered by the same clause `/export.csv` emits, so the
  sample is genuinely "the first rows of your file."
- **Query:** a new `CountMatching(ctx, ExpenseFilter) (int, error)` (`SELECT COUNT(*)`, a sibling
  of `ExpensesMatching`) for the exact total, plus a bounded `LIMIT 10` read for the sample.
  Both, and the download's `ExpensesMatching`, construct their `WHERE` from the same
  `ExpenseFilter` via **one shared predicate helper** — so the preview count equals the download's
  row count **by construction**. A `len()` of a materialised set, and a `LIMIT`-only "10+", were
  both rejected.
- **The island: input-driven, debounced ~250 ms, one fetch in flight** (a newer edit supersedes an
  in-flight one). **First paint is server-rendered** by the `/export` page handler, so the island
  only fetches on change. It is the **second vanilla-TS island** (not React — it renders a server
  fragment). `data-props` reduces to the four filter values (`prefCurrency` drops — a raw sample
  never converts).
- **Error/empty states, collapsed by the raw choice:** zero matches → `0 expenses` + "No expenses
  match these filters" (a 200, agreeing with ticket 04); bad/inverted range mid-edit → **400**
  (reusing `httpDateRangeError`) treated **softly** as an inline "Check the dates" hint that leaves
  the last good preview and the Download button in place; **503 cannot occur**; network/5xx →
  "Preview unavailable" with no armed observer to disarm (the next filter edit retries naturally).

### Docs and architecture (ticket 07) — decided text the impl map writes

1. **Amend `docs/adr/0001-react-islands-architecture.md`'s existing "Note (daily-timeline effort)"**
   to name the Export preview as the **second** vanilla-TS island (fixing the stale "the first
   island that is not React"), marking that it is **input-driven** where daily-scroll is
   **scroll-triggered**, and keeping the shared rule ("React owns the islands that carry
   client-side state or markup; an island that only moves server-rendered HTML into the page does
   not"). **No new ADR** — this is a second instance of a pattern 0001 already owns. The draft
   amendment lives in ticket 07; the effort's "a form + link needs no island" reversal is recorded
   here as *decided* (argued in ticket 05).
2. **Add `Export` and `Export Preview` to `CONTEXT.md` `## Language`** as two user-facing terms,
   with the code seam names (`ExpensesMatching`, `CountMatching`) riding *inside* the Export entry
   the way **Payer** notes `paidBy` — not as their own glossary entries. Generalize the **Island**
   entry to name both vanilla islands; optionally touch **List View** to note its client-side
   filtering contrasts with Export. Draft text lives in ticket 07.
3. **Leave `AGENTS.md` unchanged** — its Layout section documents Go package structure, which
   Export doesn't alter (handlers + templates *within* `internal/web`, no new package).

## Testing Decisions

Good tests here assert **external behaviour at well-chosen seams**, never rendered HTML, class
names, or copy. Logic is pushed to fast/pure surfaces so the one HTTP test stays thin — the
daily-timeline strategy (its ticket 10) applied to a CSV byte stream instead of an HTML fragment.
Tests are written **before** implementation throughout; tickets 01–06 already designed every
interface, so an implementation-first order has nothing to discover.

**Four seams, each owning one concern:**

| Seam | Test surface | What it asserts | Tickets |
|---|---|---|---|
| **Service** | `ExpensesMatching` over `expenseRepoStub` | each filter, filter combinations, and the sharp **unset = all** case (a zero-value `ExpenseFilter` returns every row) | 01 |
| **Service** | `EnsureRatesForRange` with `mockRateClient.Timeseries` **counting calls** | fetches only the dates the store lacks; a 2nd export over the same range makes **zero** calls (cache); a `Timeseries` error → the method returns an error | 02 |
| **Pure** | `buildRecords(rows, resolve, pref)` with a fake resolver | per-date convert; **KWD → 3 decimals**, **JPY → 0**; unconvertible → **blank** `amount_converted` + `rate_date` but present `currency_converted`; payer-less → blank `paid_by`; header is record 0; **zero rows → header-only** | 02, 03 |
| **Repository** | `RatesOnOrBefore` over **in-memory SQLite** | a **weekend expense resolves to the prior trading day** (Sat → Fri). The **first repository test in the codebase** (in-memory `:memory:`, migrate, insert Friday's rate, assert Saturday resolves to it) | 02 |
| **httptest** | `GET /export.csv` via `httptest.NewRecorder` against the export handler | **400** on unparseable/inverted range; **503** when `EnsureRatesForRange` errors; **200** header-only on zero matches; `Content-Disposition` filename for ranged and open-ended cases; **one** end-to-end round-trip of the hostile field parsed back through `csv.Reader` (proving BOM strip, CRLF, quote-doubling on the *actual bytes*) | 04, 06 |

- **The pure seam stays pure because the rate lookup is injected.** `buildRecords` takes
  `resolve(date) → (rates, rateDate)`; production backs it with the repo, tests pass a canned
  fake. The weekend→Friday rule is therefore proven once at the repository seam where the SQL
  lives, not in `buildRecords`.
- **The encoding contract is only real on the written bytes.** `buildRecords` emits `[][]string`
  and does not apply the BOM/CRLF/escaping (those are the streaming writer's), so ticket 06's
  contract is verified exactly once, in the httptest, against the real response body. Hostile
  fixture: a description like `He said "hi", then left` followed by an embedded newline, which
  must round-trip through a real `csv.Reader` byte-identical.
- **The 503 is asserted at two levels deliberately** — the service test proves
  `EnsureRatesForRange` errors when rates can't be had; the httptest proves the handler maps that
  to 503 (same shape as daily-timeline's `ErrInvalidRange` → 400: logic at the service, status at
  the handler).
- **Leading red test:** `TestBuildRecords_ColumnsFormattingAndBlanks` (web package) — one fixed
  fixture (a KWD row, a JPY row, an unconvertible-currency row, a payer-less row) plus a fake
  resolver, asserting the exact `[][]string` with the header as record 0. It cannot compile until
  `buildRecords` and its signature exist; that failure is the red.
- **Prior art / scaffolding to extend** (no new stubs invented): `mockRateClient`
  (`service_test.go`) grows a counting `Timeseries` double; `expenseRepoStub` grows a `matching`
  field for `ExpensesMatching`; the rate repository grows `RatesOnOrBefore` with the codebase's
  first in-memory-SQLite test. Handler-in-package `httptest` follows the daily-timeline precedent
  (`server_test.go`).

**Not tested:** the Export page HTML (prototyped and eyeballed against its `.approved` asset, never
asserted); chrome/class-names/copy anywhere; the rate client's real HTTP call to Frankfurter
(mocked at the `RateFetcher` seam); timezone/DST (Export's range is user-supplied query params, not
an anchored "today").

## Out of Scope

- **Formats other than CSV** — Excel/`.xlsx`, JSON, PDF. Another format is a fresh effort.
- **Scheduled or recurring exports**, and **emailing/uploading** the file anywhere. The page
  produces a download on demand, nothing more.
- **Wiring the List View (`/expenses`) to the server-side `ExpensesMatching` seam.** The struct is
  general enough to serve that later migration for free, but no code is built for a caller that
  doesn't yet exist; the List View keeps its client-side `data-table` filtering.
- **Multi-select category/payer.** The seam is single-value equality; the UI cannot offer more.
- **Comma-decimal-locale accommodation.** One canonical US/ISO file is produced; other-locale users
  import via their spreadsheet's text-import dialog (documented, not accommodated).
- **Migrating money from `float64` to integer minor units** — a pre-existing modelling choice this
  effort does not relitigate.
- **Executing the build.** This is plan-only; code lands under a separate `export-csv-impl` map,
  the way `daily-timeline` → `daily-timeline-impl` ran.

## Further Notes

- **Domain context:** Go server-rendered templates + React Islands (partial hydration), SQLite,
  single binary. `Amount`+`Currency` are the entered figures; `ConvertedAmount` and `PaidByName`
  are computed at render time, not stored — so the exporter runs the same conversion/naming the
  views do rather than reading columns off SQL.
- **Three routes total:** `GET /export` (page), `GET /export.csv` (download),
  `GET /export/preview` (preview fragment). Only the download can 503; the preview never does.
- **Two vanilla-TS islands now exist** app-wide: daily-scroll (scroll-triggered) and export-preview
  (input-driven). Both are thin `fetch` → `innerHTML` swaps of server-rendered HTML — the deliberate
  pattern that keeps Go the only formatter.
- **New external dependency surface:** Frankfurter's time-series endpoint (`GET /v1/{from}..{to}?base=USD`),
  reached once per never-before-exported range via the new `Timeseries` rate-client method and then
  cached in the existing `exchange_rates` store.
- **The `-impl` map's build-order hint:** the leading red is `TestBuildRecords_...`; the seam work
  (`ExpenseFilter` + `ExpensesMatching`/`CountMatching`, `EnsureRatesForRange` + `Timeseries` +
  `RatesOnOrBefore`) underpins it, and the httptest + encoding contract close it out. Docs
  (ticket 07) land last, mirroring how `daily-timeline`'s docs landed in `daily-timeline-impl`.
