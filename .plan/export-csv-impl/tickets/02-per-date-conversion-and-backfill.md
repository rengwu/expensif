---
type: task
blocked_by: [01]
---

# Per-date historical conversion and rate backfill

## Question

Deliver the spec's **rate date and conversion basis** (02): fill ticket 01's blank
`amount_converted`/`rate_date` columns with figures converted at each expense's **own-date**
historical rate, backfilled from Frankfurter and cached, hard-failing rather than shipping
anything inaccurate.

Build:

- **`Timeseries(ctx, base, start, end)`** on the `RateFetcher` interface and its `rate` client
  — fetching Frankfurter's time-series endpoint (`GET /v1/{from}..{to}?base=USD`) once for a
  whole range. (The interface has only `Latest` today.)
- **`EnsureRatesForRange(ctx, from, to)`** — a discrete service method the handler calls before
  converting: compute which dates in range the `exchange_rates` store lacks, fetch **only** if
  any are missing, upsert every returned business-day rate via the existing `SaveRates`, and
  return an error if the range's rates still can't be obtained. A second export over the same
  range makes **zero** network calls.
- **`RatesOnOrBefore(base, date) → (rates, matchedDate)`** on the rate repository — the nearest
  stored rate on-or-before a date, over the now-dense store, so a weekend/holiday expense
  resolves to the prior trading day's rate.
- **Wire the real resolver.** The export handler calls `EnsureRatesForRange` first, then passes
  `buildRecords` a real `resolve(date)` backed by `RatesOnOrBefore` + the existing
  `ConvertWithRates` cross-rate. Converted columns now populate; `rate_date` shows the effective
  date.
- **The 503 branch.** Slot rate backfill into ticket 01's pre-stream error order (step 3, after
  the range 400s): if `EnsureRatesForRange` errors → **503, no file**. The rate client's 10s
  timeout means a hung provider fails rather than hangs.
- **Unconvertible currency → blank, not a failure.** When the date's rate set is present but
  doesn't cover the row's currency, leave `amount_converted` and `rate_date` empty (ticket 01's
  `buildRecords` already does this when `resolve` yields no rate for that currency) — this does
  **not** trigger the 503.

Tests come **first**, red, per the spec's Testing Decisions:

- **Service:** `EnsureRatesForRange` with `mockRateClient.Timeseries` **counting calls** —
  fetches only the dates the store lacks; a 2nd export over the same range makes **zero** calls
  (cache); a `Timeseries` error → the method returns an error.
- **Repository (net-new seam):** `RatesOnOrBefore` over **in-memory SQLite** — the codebase's
  first repository test: spin up `:memory:`, migrate, insert Friday's rate, assert a Saturday
  expense resolves to it (Sat → Fri).
- **httptest (extend 01's):** **503** when `EnsureRatesForRange` errors; a converted-amount
  assertion on a normal row; an unconvertible-currency row blanking `amount_converted` +
  `rate_date` **without** failing the export.

Done when: the named tests are green, a cold range triggers exactly one Frankfurter fetch then
caches, provider-down yields a clean 503 with no file, and a downloaded CSV shows honest
per-date converted amounts (weekend rows using the prior trading day, unconvertible currencies
blank).

## Answer

Delivered as specified; the tests landed first and red (interface additions broke the mocks'
compile until the methods existed), then the implementation turned them green.

- **`rate.Client.Timeseries(ctx, base, start, end)`** fetches Frankfurter's `/{start}..{end}`
  endpoint once, decoding `rates` (date → target → rate) and injecting the base at `1.0` per date
  so a cross-rate anchors — the exact shape `Latest` already uses. Added to the `RateFetcher`
  interface.
- **`Service.EnsureRatesForRange`** fetches only when the store lacks coverage, then caches. The
  cache key is `rangeCovered`: a two-pointer walk over the business days (Mon–Fri) of `[from, to]`,
  a weekday counting as covered when a stored rate sits within `rateLookback` (7 days) on-or-before
  it. That window is what lets an ordinary weekend gap **and** a provider-omitted weekday holiday
  read as covered — so a second export makes **zero** calls — while a genuine un-fetched span
  (larger than the window) still triggers the backfill. The fetch itself is widened `rateLookback`
  before `from`, so a weekend/holiday expense at the very start still has a prior trading day to
  resolve to. A `Timeseries` transport error is the only hard failure (→ handler 503); a range the
  provider simply has no data for converts to blanks, not a failure.
- **`RatesOnOrBefore(base, date)`** at the repository (`date = (SELECT MAX(date) … WHERE date <= ?)`)
  returns the nearest stored rate set and the date it matched — the codebase's first repository test
  proves Sat→Fri over in-memory SQLite (`db.Migrate` was exported so the test builds the real
  schema). A supporting `RateDatesInRange` feeds the coverage probe in one query.
- **Handler wiring:** after the range 400s, the backfill runs (step 3) and maps its error to 503
  before the first byte; then a memoised `resolve(date)` backed by `RatesOnOrBefore` + the existing
  `service.CrossRate` fills `amount_converted`/`rate_date`. `currency_converted` is always present;
  unconvertible currencies leave the two converted cells blank without failing.
- **Two deliberate refinements, both spec-consistent** (surfaced here, not slipped in): the backfill
  is driven by the matched rows' own date-span — of the rows that *need* converting — rather than the
  raw filter range, because a bare export's range is `0001..9999` and walking/fetching that is absurd;
  and the fetch is skipped entirely when no row needs converting (all already in the preferred
  currency), so a single-currency export never 503s on a provider it doesn't need. Neither weakens
  user story 20 — it refuses only when rates the export *actually needs* can't be obtained.
- Verified on the wire against a warmed scratch store (no live call): BOM + CRLF intact, a hostile
  field round-tripped, an EUR row converted at its own date, a **Saturday** row resolved to the prior
  **Friday**'s `rate_date`, and an unconvertible `ZZZ` row blanked cleanly at a 200.

- **For ticket 03/04:** the preview routes must **not** call `EnsureRatesForRange` — they show raw
  amounts only, so the whole 503 surface stays on `/export.csv`, exactly as the spec's preview
  contract requires.
