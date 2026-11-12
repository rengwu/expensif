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

## Answer

**The encoding contract: `encoding/csv` for escaping, UTF-8 with a leading BOM, comma
delimiter, CRLF line endings, one canonical US/ISO format that the spec documents.**

- **Escaping: `encoding/csv.Writer`, never hand-rolled.** It quotes any field containing a
  comma, double-quote, `\r`, or `\n`, and doubles embedded quotes, per RFC 4180. The spec
  states this outright so no one "optimises" it into a `strings.Join`. **Hostile fixture**
  (for ticket 08): a description like `He said "hi", then left` followed by an embedded
  newline — it must round-trip through a real `csv.Reader` byte-identical.

- **BOM: prepend the three bytes `EF BB BF` once, before the header record.** In ticket
  04's streaming handler the write order is: `w.Write(utf8BOM)` → `csv.NewWriter(w)` →
  header record → data rows → `Flush()`. Excel on Windows then detects UTF-8 and renders
  accented descriptions correctly. **Consequence carried to ticket 08 and any re-import:**
  Go's `csv.Reader` does not strip a BOM, so tests must strip the leading `﻿` before
  asserting the header equals `id,date,…`.

- **Delimiter: comma** (`encoding/csv`'s default; `Writer.Comma` untouched).

- **Line endings: CRLF.** Set `Writer.UseCRLF = true` — Go defaults to bare `\n`; RFC 4180
  specifies `\r\n` and it is what Excel expects. One field-internal `\n` (in a quoted
  description) therefore stays `\n` inside quotes while row terminators are `\r\n`, which
  ticket 08 should pin.

- **Locale: one canonical US/ISO file, documented — comma-decimal locales not
  accommodated.** Amounts keep ticket 03's dot decimals (`12.5`) with the comma delimiter.
  The app has no per-user locale to key off, and a single well-defined format beats a
  guessed one; the spec notes that comma-decimal-locale users (DE/FR Excel) import via
  their spreadsheet's text-import dialog, choosing comma delimiter + dot decimal. This is a
  deliberate scope boundary, **closing the "locale and the decimal separator" fog patch**
  rather than leaving it open. A semicolon/comma-decimal alternate format was rejected: it
  merely moves the friction to US Excel and contradicts ticket 03.

**Facts later tickets depend on.** Ticket 08's encoding tests: the hostile field
round-trips through `csv.Reader` unchanged; the output begins with `EF BB BF` and the
header asserts only after stripping it; row terminators are `\r\n`; a field with a comma /
quote / newline is quoted with quotes doubled. Ticket 04 owns the BOM's placement in the
stream (it is the first bytes written, before the `csv.Writer`). Ticket 07 records the
locale limitation as a documentation note (a user-facing "opens in Excel; other-locale
users import manually" line); no `CONTEXT.md` term is minted here.
