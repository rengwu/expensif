---
type: grilling
blocked_by: []
claimed_by: session-john-goh-2026-07-14b
claimed_at: 2026-07-14T09:22:56Z
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
