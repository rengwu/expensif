---
type: grilling
blocked_by: [02]
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

## Answer

**Ten columns, fixed order, one header row; amounts as exact shortest decimals; absence is
an empty cell.**

- **Columns and order** (the header row, verbatim):

  ```
  id,date,category,description,paid_by,amount,currency,amount_converted,currency_converted,rate_date
  ```

  `id` is included as the reconciliation / re-import key; `paid_by` because payer is a
  filter dimension (ticket 01), so a filtered export should name it. **`created_at` is
  omitted** — it is internal audit metadata that clutters an expense report; the
  expense's own `date` is the meaningful one. Header names are machine-friendly
  `snake_case` (`amount_converted`, not "Amount (converted)") so the file re-imports
  without fixups.

- **Money: `strconv.FormatFloat(v, 'f', -1, 64)`** for both `amount` and
  `amount_converted`. This is the shortest decimal that round-trips the stored `float64` —
  plain notation (never scientific), no thousands separator, and **no imposed decimal
  places**, so it is exact for every currency: KWD `1.234` stays `1.234`, JPY `100` stays
  `100`, EUR `12.5` stays `12.5`. Fixed `%.2f` was rejected: it truncates 3-decimal
  currencies (a real precision loss) and pads 0-decimal ones. Decimal places therefore vary
  row to row; that is cosmetic and the spreadsheet reformats on display. (The domain stores
  money as `float64`, not integer minor units — a pre-existing modelling choice this ticket
  does not relitigate.)

- **Dates.** `date` is emitted as the stored `YYYY-MM-DD` string unchanged. `rate_date`
  (from ticket 02) is `YYYY-MM-DD` or empty. There is no `created_at` column, so its
  timestamp format is moot.

- **Absence is an empty cell, never a zero or a placeholder.**
  - `amount_converted` **and** `rate_date` are both empty when the row's currency is
    unconvertible (ticket 02's rule).
  - `paid_by` is empty when the expense has no payer (`PaidByID == 0` / null join).
  - `currency_converted` is the preference currency and is present on every row (it names
    the target even when the converted amount is blank), matching the charting preview and
    ticket 02's example.

- **The row is assembled as a `[]string` record in the export handler**, one per expense,
  built from the raw `[]domain.Expense` the ticket-01 seam returns plus ticket 02's
  per-row converted amount and effective rate date. Building `[]string` directly (rather
  than a struct the encoder reflects over) keeps every formatting rule above in one place
  and hands `encoding/csv` exactly what it escapes — there is **no** rate-date field bolted
  onto `domain.Expense`. The header is record 0.

**Facts later tickets depend on.** Ticket 06 escapes/encodes these `[]string` records and
owns the header-is-row-0 write; nothing above dictates quoting — that is 06's. Ticket 08
asserts: the exact header line and column order; a KWD amount emitting three decimals and a
JPY amount none; an unconvertible row blanking `amount_converted` + `rate_date` (but not
`currency_converted`); a payer-less row blanking `paid_by`; and a zero-match export being a
header-only file.
