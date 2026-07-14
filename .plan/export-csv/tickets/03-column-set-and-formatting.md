---
type: grilling
blocked_by: [02]
claimed_by: session-john-goh-2026-07-15
claimed_at: 2026-07-14T17:37:14Z
---

# The full column set and row formatting

## Question

Fix the exact columns, their order, their header names, and how each value is formatted.
Waits on ticket 02 because whether the rate date is its own column is decided there.

- **Which fields?** `Date`, `Category`, `Description`, original `Amount` + `Currency`,
  converted amount + preference currency are the spine (map premise). Decide the fate of
  `ID` (needed for round-tripping/reconciliation, noise otherwise), `PaidByName`,
  `CreatedAt`, and any rate-date column 02 calls for.
- **Money formatting.** `Amount` is a `float64`. Raw (`12.5`) or fixed to 2 decimals
  (`12.50`)? A spreadsheet wants an unformatted number (no thousands separators, no
  currency symbol) so it parses as numeric — confirm and state it.
- **Date formatting.** `Date` is already a `YYYY-MM-DD` string; `CreatedAt` is a
  `time.Time`. Pin the wire format for each (ISO 8601 is the safe default).
- **Header row.** Confirm there is one, and pin machine-friendly header names
  (`amount_converted`, not `Amount (converted)`) so re-import is painless.
- **Column order.** State it once, explicitly, so the exporter and any test agree.

The output of this ticket is a concrete example row the spec can show — the same shape as
the preview locked during charting.
