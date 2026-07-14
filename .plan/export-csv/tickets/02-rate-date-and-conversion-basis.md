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

**Per-expense-date conversion using the nearest stored rate on-or-before each row's date,
falling back to the latest set; the effective rate date is a per-row column; an
unconvertible currency leaves both money-converted and rate-date blank.**

- **Basis: per-expense-date, not the single latest set.** Each row is converted at the
  rate that was current on its own date. This was chosen over the simpler
  `GetRatesForConversion` single-set basis (which every view uses) with eyes open — see
  the constraints below.

- **"The rate for its date" = nearest stored rate dated on-or-before the expense.** New
  repository method, shape:

  ```go
  // most recent rate set not after `date`; matched is the date actually used
  RatesOnOrBefore(ctx, base, date string) (rates map[string]float64, matched string, err error)
  // SQL: WHERE base=? AND date <= ? ORDER BY date DESC LIMIT 1  (then that date's full set)
  ```

  Exact-date matching was **rejected**: rates are stored only on refresh days, so an
  expense's exact date almost never has a row, and exact-match would make every row fall
  back to latest — collapsing the per-date choice into a no-op. On-or-before uses the rate
  that was genuinely current, so per-date conversion actually fires as history
  accumulates.

- **Fallback: the latest set** (`GetLatestRates` / `GetRatesForConversion`) when *nothing*
  precedes the expense's date (a row older than any stored rate). This is the accepted
  cost of the basis: on a freshly-run instance the store is sparse, so early on many rows
  still fall back to latest; the per-row `rate_date` column (below) makes every such
  fallback **visible** rather than a silent claim of date-accuracy.

- **Surfacing: a per-row `rate_date` column**, not a filename or footer. Because the basis
  now varies row to row, the rate date *must* be per-row — which also makes the column
  informative (it tells you whether a row used its own era's rate or fell back). A footer
  row was rejected for breaking CSV round-trip (ticket 06 keeps the file a clean
  header-plus-rows); a filename value can't vary per row. This adds a column to ticket
  03's set: `...,amount_converted,currency_converted,rate_date`.

- **Unconvertible currency → blank, not zero.** When a row's currency is absent from the
  chosen rate set (`ConvertWithRates` errors), leave **both** `amount_converted` *and*
  `rate_date` empty. Blank is skipped by spreadsheet `SUM`/`AVG`, so it can't silently
  corrupt a total the way `0` would; the original `amount` + `currency` columns still
  carry the true figure, so nothing is lost. Louder than the views' silent `continue`,
  deliberately — a `0` in a money column is a landmine.

**Facts later tickets depend on.** Ticket 03 gains a `rate_date` column and must define its
format (a `YYYY-MM-DD` date or blank) and the blank-on-unconvertible rule for the two
converted columns. The handler assembles a per-row export value (original + converted +
effective rate date) rather than mutating `domain.Expense`, since `Expense` has no
rate-date field — an export-specific row type is ticket 03's to shape. A within-export
cache of rate-set-by-effective-date avoids an N-query blowup (many rows share a date);
implementation detail, flagged for ticket 08's perf awareness. Ticket 08 tests: a row
older than any stored rate falls back to latest; an unconvertible-currency row emits blank
converted + blank rate_date; an on-or-before match uses the earlier date, not today's.

**No new ticket spawned.** The chosen sourcing needs no historical-rate *fetch* (that was
only option A) — `RatesOnOrBefore` reads what the store already has. Ticket 01 is **not**
undermined: it deferred the rate choice to this ticket explicitly, and its decision (raw
rows from the seam, handler converts) stands; only 01's illustrative `GetRatesForConversion`
call is superseded by the per-row lookup.
