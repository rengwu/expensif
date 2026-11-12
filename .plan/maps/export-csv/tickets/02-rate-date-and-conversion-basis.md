---
type: grilling
blocked_by: []
---

# The rate date and conversion basis

## Question

Rows carry a converted amount (map premise), so the CSV depends on an exchange rate — and
a rate has a date. Pin the basis so the file is reproducible and self-documenting.

- **Which rate?** The single **latest** rate set (what the Daily and List views use via
  `GetRatesForConversion`, `service.go:299`), or the rate **as of each expense's own
  date**? The latter is more truthful for historical rows but is not something the current
  rate machinery does — decide whether that truthfulness is worth net-new work, or out of
  scope for this effort.
- **How is the rate date surfaced?** As its own column on every row, a single value in the
  filename, a trailing footer/comment row, or omitted. This directly shapes ticket 03's
  column set, which is why 03 waits on this.
- **Missing rate.** If a currency has no rate, what does `ConvertedAmount` hold and what
  does the cell show — blank, `0`, the original amount, an error marker? The views today
  fall back silently (`ShowConverted = convertedTotal > 0`); decide if the export should be
  louder, since a silent `0` corrupts a spreadsheet sum.

`ConvertExpensesTotal` (`service.go:330`) and `ConvertWithRates` (`:315`) are the existing
conversion entry points to reason from.

## Answer

*(Re-grilled 2026-07-14. A first resolution chose per-expense-date with a silent
fallback to the latest set; it was reopened because that fallback ships inaccurate
numbers. This resolution keeps per-date but makes it accurate — the fallback is gone,
replaced by a persisted, backfilled historical store. Prior version is in git.)*

**Per-expense-date conversion over a persisted historical rate store: lazily backfilled
from Frankfurter on export and cached, hard-failing the export if the range's rates can't
be obtained rather than emitting anything inaccurate. Per-row `rate_date` column; a
currency the provider doesn't cover leaves converted and rate_date blank.**

- **Basis: per-expense-date, sourced from real historical rates — not the latest set, and
  not per-date-with-silent-fallback.** Each row converts at the rate that was actually
  current on its date. The single-latest-set basis was rejected as inaccurate for
  historical rows; the first pass's fallback-to-latest was rejected this re-grill because a
  row silently converted at today's rate is exactly the inaccuracy the file must not ship.

- **The store already has the right shape.** `exchange_rates(base, date, target, rate)`
  with `SaveRates`/`GetRates` exists — we *populate* it, we don't build it. Base stays
  `USD`; conversion is the existing cross-rate (`ConvertWithRates`).

- **Population: lazy backfill on export, then cached.** For an export over `[from, to]`:
  compute which dates in range the store lacks; if any, fetch **Frankfurter's time-series**
  endpoint once for the whole range (`GET /v1/{from}..{to}?base=USD`) and upsert every
  returned business-day rate via `SaveRates`. Subsequent exports over the same range read
  the store with no network call. **New rate-client capability required** —
  `Timeseries(ctx, base, start, end)`; the client today has only `Latest` (`rate/client.go:32`).

- **Failure: hard-fail, no file.** If after the fetch attempt the store still can't supply
  a rate set for a date the range needs (offline, API down, provider error), the export
  returns an error (503 / error page) and produces **no CSV**. Never a partial file of
  latest-rate guesses. This is the accuracy guarantee the re-grill was about.

- **Non-trading days: nearest stored rate on-or-before.** Frankfurter omits weekends and
  holidays, so even a complete fetch leaves gaps *inside* the range. Convert each row at
  the nearest stored rate dated on-or-before its date (`RatesOnOrBefore(base, date)` →
  rates + matched date) over the now-dense store — a Saturday expense correctly uses
  Friday's rate. (This mechanism survives from the first pass, but now it walks a densely
  populated store, not a near-empty one, so it resolves to a genuine adjacent-day rate
  rather than to "latest".)

- **Surfacing: a per-row `rate_date` column** (`YYYY-MM-DD`), the date of the rate actually
  used — equal to the expense date on a trading day, a day or two earlier across a weekend.
  Because the store is dense and the fetch hard-fails, this is now an honest per-date value,
  not a fallback marker. Adds a column to ticket 03's set:
  `...,amount_converted,currency_converted,rate_date`. A filename value can't vary per row;
  a footer row breaks CSV round-trip (ticket 06) — both rejected.

- **Unconvertible currency → blank, not zero (distinct from a fetch failure).** If the
  date's rate set is present but simply doesn't include the row's currency (Frankfurter
  covers ~31 currencies; the domain accepts any string), leave **both** `amount_converted`
  and `rate_date` empty. This does *not* trigger the hard-fail — the date's rates were
  obtained; this one currency just isn't quotable. Blank is skipped by spreadsheet
  `SUM`/`AVG`; `0` is a silent landmine; the original `amount`+`currency` still carry the
  truth.

**Scope.** This brings historical-rate fetching **into scope** for the effort — the
charting-era note that per-date was "net-new work, possibly out of scope" is superseded
here by the user's requirement that per-date be *accurate*. No separate decision ticket is
spawned (the mechanism is decided, not open); the capability is a spec requirement the
`-impl` map will carry as build tickets.

**Facts later tickets depend on.**
- **03:** a `rate_date` column (`YYYY-MM-DD` or blank) and an export-specific per-row type
  carrying original + converted + effective rate date (`domain.Expense` has no rate-date
  field).
- **04 (route):** export now (a) performs a network fetch as a side effect — latency and a
  sane timeout matter — and (b) has a new **failure status** (503, rates unavailable)
  alongside the bad-range 400. The route's error handling must cover it.
- **05 (page UI):** a blind-download button can now fail *after* the click (fetch down) and
  can be slow on a cold range; the page's error/loading states gain this case.
- **08 (tests):** backfill fetches only missing dates; a second export over the same range
  makes no network call (cache); the hard-fail path when the fetch fails and rates are
  absent; nearest-on-or-before resolving a weekend expense to the prior trading day; an
  unconvertible-currency row blanking converted + rate_date *without* failing the export.
- **Ticket 01 is not undermined.** Its decision (raw rows from the seam, handler converts)
  stands; the handler now also triggers backfill before converting, which lives in the
  handler layer 01 designated. Only 01's illustrative `GetRatesForConversion` call is
  superseded by the per-date lookup.
