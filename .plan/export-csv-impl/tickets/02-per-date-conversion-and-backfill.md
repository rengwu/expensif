---
type: task
blocked_by: [01]
claimed_by: claude-code-session-2026-07-16
claimed_at: 2026-07-16T06:43:51Z
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
