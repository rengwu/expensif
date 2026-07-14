---
type: grilling
blocked_by: [01]
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

## Answer

**Two routes: `GET /export` renders the page, `GET /export.csv` streams the download.
Filters ride query params that map 1:1 onto `ExpenseFilter`. Validation and rate backfill
run first and own every error status; then the CSV streams at constant memory.**

- **Routes and verb.** `GET /export` → the Export page (a `method=GET` form).
  `GET /export.csv?from=&to=&category=&payer=` → the download. Both are plain
  `mux.HandleFunc` lines in `server.go` beside `/calendar`, `/expenses`. GET (not POST) so
  the download is a **bookmarkable, shareable URL**; the page's form action *is*
  `/export.csv`, so submitting it navigates to the download and `Content-Disposition`
  makes the browser save the file without leaving the page. Each param maps to an
  `ExpenseFilter` field; an **absent or empty param is "unset" = all** (ticket 01), so a
  bare `GET /export.csv` exports everything. GET is honoured despite ticket 02's backfill
  because that side effect only **warms the rate cache** (idempotent upsert) — it mutates
  no user data.

- **Delivery: stream to the `ResponseWriter`.** After the pre-write checks below,
  `csv.NewWriter(w)` writes ticket 03's header record then streams one row at a time,
  ending with `Flush()`. **Flat memory for any range**, and lower time-to-first-byte on a
  wide export. This is safe precisely because every failure that must become an HTTP
  status happens **before the first byte** — see below. The one residual risk, a DB read
  fault mid-`rows.Next()`, truncates the file (status already sent); it is logged, and
  accepted as rare against the memory/latency win. Buffering-then-flushing was weighed —
  it turns that truncation into a clean 500 — but rejected: the integrity gain is small
  once the status-bearing errors are pre-stream, and holding the whole file in memory
  doesn't scale with an unbounded range.

- **Error order, all pre-stream.**
  1. Parse `from`/`to` when present; unparseable → **400** via the existing
     `httpDateRangeError` (`handlers_html.go:244`, `ErrInvalidDate`).
  2. Inverted range (`start > end`) → **400** (`ErrInvalidRange`), reusing the daily
     view's precedent rather than answering with an empty file.
  3. Rate backfill (ticket 02) runs; if the range's rates can't be obtained → **503**, no
     file. The rate client already carries a 10s timeout (`rate/client.go:19`), so a hung
     provider fails rather than hanging the request.
  4. `category`/`payer` are **not** validated — an unknown value simply matches nothing
     (`ExpenseFilter` equality), yielding the empty result below, not an error.

- **Empty result → a header-only CSV, 200.** A zero-match filter is a legitimate export,
  not an error; it returns the header row and nothing else (consistent with tickets 03 and
  08). No redirect, no special-case message — the file says "no rows matched" by being
  empty, and the page stays put.

- **Headers.** `Content-Type: text/csv; charset=utf-8` (the exact charset/BOM detail is
  ticket 06's) and `Content-Disposition: attachment; filename="…"`. **Filename convention:**
  `expensif-{from}_{to}.csv` when both bounds are given (e.g.
  `expensif-2026-01-01_2026-07-15.csv`), self-documenting the range; when a bound is
  omitted (open-ended export), fall back to `expensif-expenses-{today}.csv`. Category/payer
  are *not* encoded into the filename — it would grow unreadable, and the range is the
  primary axis.

**Facts later tickets depend on.** Ticket 05's page is a `method=GET` form whose `action`
is `/export.csv` and whose controls emit exactly `from,to,category,payer`; its error/loading
states must cover a download that can **503 or be slow** on a cold range (the fetch precedes
the file). Ticket 07 records two new routes in `server.go` and confirms **no island** — the
page is a plain form and the download is a byte stream. Ticket 08 tests: 400 on unparseable
and on inverted range; 503 when the backfill fails; a header-only file on zero matches; the
`Content-Disposition` filename for both the ranged and open-ended cases; and that a bare
`GET /export.csv` exports everything.
