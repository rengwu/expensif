---
type: grilling
blocked_by: [01, 03, 06]
---

# Test strategy

## Question

Where the seams are tested, so the impl session knows what to write and when. Waits on the
tickets that define the testable surfaces: the filtering seam (01), the row/column shape
(03), and the encoding contract (06).

- **The filtering seam (01).** Service-level tests for each filter and their combination —
  and the sharp edge that an *unset* filter means *all*, not *none*, so a blank form
  exports everything rather than nothing.
- **Row generation (03).** Assert the concrete column set, order, and formatting against a
  fixed fixture — including a zero-expense result yielding a header-only file.
- **The encoding contract (06).** A hostile `Description` (comma + quote + newline)
  round-trips through a real CSV reader unchanged; the BOM/delimiter/line-ending rules hold.
  Test through `encoding/csv`'s reader, not by eyeballing bytes.
- **The route (04).** A bad/reversed range is a 400, not a silent empty file; the
  `Content-Disposition` filename matches the convention. Follow the daily-timeline
  precedent of testing the handler in-package (`server_test.go`).
- **Tests-first?** State which tests land red before implementation, echoing the
  daily-timeline strategy where the first gap-fill test led the code.

Pin the seams, not the assertions on rendered HTML — the page (05) is prototyped and
eyeballed against its `.approved` asset, not asserted.
