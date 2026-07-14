---
type: grilling
blocked_by: [01]
claimed_by: session-john-goh-2026-07-15
claimed_at: 2026-07-14T17:54:50Z
---

# The download route and delivery

## Question

How does the browser actually receive the file? Waits on ticket 01 because the filter
inputs are what the route carries.

- **Route and verb.** `GET /export/csv?from=&to=&category=&payer=` (bookmarkable,
  shareable, cache-neutral) versus a `POST` from the page's form (no filter state in the
  URL, but not linkable). Decide, and state how the Export page (05) hands filters to it.
- **Headers.** `Content-Type: text/csv; charset=utf-8` and a `Content-Disposition:
  attachment; filename=...`. Pin the **filename convention** — e.g.
  `expensif-2026-01-01_2026-07-14.csv` encoding the range, or a fixed name plus a
  timestamp. A bad filename is the user's first impression of the file.
- **Streaming vs buffering.** Write rows straight to the `ResponseWriter` via
  `encoding/csv` (constant memory, but an error mid-stream can't become a clean HTTP
  status), or buffer then flush (simpler errors, whole file in memory). This is where the
  **"streaming vs buffering a large export"** fog patch resolves.
- **Bad input.** An unparseable or reversed date range — reuse the daily view's
  `ErrInvalidRange` → 400 precedent (see `daily-timeline` ticket 10), or clamp silently?
- **Empty result.** A filter matching zero expenses — a CSV with only a header row, or a
  friendly "nothing matched" back on the page? Decide before it surprises someone.
