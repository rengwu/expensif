---
type: grilling
blocked_by: [03]
---

# CSV encoding correctness

## Question

The parts of CSV that bite in production. Waits on 03 because the columns and their
formats are what gets encoded.

- **Escaping.** A `Description` can contain commas, double quotes, and newlines. Go's
  `encoding/csv` quotes and escapes these correctly — the decision is to **use it** rather
  than hand-rolling string joins, and to say so in the spec so no one "optimises" it away.
  Name a hostile description as a test fixture.
- **Encoding and the Excel BOM.** UTF-8 is the default, but Excel on Windows misreads
  UTF-8 without a leading byte-order mark, mangling accented text and currency symbols.
  Decide whether to prepend a UTF-8 BOM (helps Excel, mildly pollutes strict parsers) —
  this is the crux of this ticket.
- **Delimiter and line endings.** Comma vs semicolon (the latter dodges the decimal-comma
  locale clash — see the fog patch), and `\r\n` vs `\n`. Pin both; RFC 4180 says CRLF.
- **The decimal-separator locale patch.** Resolve or explicitly defer the **"locale and
  the decimal separator"** fog here: does the spec target US-format numbers only, or
  accommodate comma-decimal locales (which forces the delimiter choice above)?

The answer is a short, testable set of rules — an encoding contract ticket 08 asserts
against.
