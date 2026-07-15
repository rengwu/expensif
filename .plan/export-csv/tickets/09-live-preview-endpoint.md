---
type: grilling
blocked_by: [02, 04, 05]
claimed_by: claude-session-3ef0a2a8
claimed_at: 2026-07-15T19:41:15Z
---

# The live-preview endpoint contract

## Question

Ticket 05 chose a page with a **live preview** — an island that refetches a row count and a
few sample rows as the filters change — and the user accepted the island for it. The
prototype answered *what it looks like*; it could not answer the **server contract** behind
it. Decide that contract, grilling with `codebase-design` for the seam and `domain-modeling`
for any term minted.

- **Route and payload.** A new `GET /export/preview?from&to&category&payer` (filters map 1:1
  onto `ExpenseFilter`, same as `/export.csv`) returning JSON (`{count, rows:[…]}`) that the
  island renders? Or an HTML fragment the island swaps in (matching the daily-scroll island's
  fragment style, `daily-timeline` ticket 06)? Pin the shape and the sample size (first N
  rows — how many?).
- **Converted or raw sample — the sharp one.** The download converts each row at its own
  date, which means ticket 02's per-date rate backfill (a Frankfurter time-series call,
  upsert, potentially slow, and **503-capable**). If the preview shows `amount_converted`, it
  either pays that cost **on every filter change** (bad) or reads only already-cached rates
  (then the sample's converted column may be blank until the real download warms them).
  Decide: does the preview show **raw amounts only** (cheap `COUNT(*)` + a `LIMIT N` read, no
  rate work) with conversion deferred to the actual download, or does it show converted
  figures and accept the cost / a partial-blank sample? The whole point of the live count is
  reassurance — weigh it against making every keystroke hit the rate provider.
- **Does the island debounce / how does count relate to the download?** The count must equal
  the number of rows `/export.csv` will emit for the same filters (or the reassurance lies).
  Confirm both go through the one `ExpensesMatching` seam (ticket 01).
- **Reuse `ExpensesMatching` vs a dedicated count query.** A `COUNT(*)` over the filter is
  cheaper than materialising rows just to count them; decide whether the seam grows a count
  method or the preview reuses the existing one with a `LIMIT`.
- **Error / empty in the island.** Zero matches, a bad range typed mid-edit, a 503 from the
  provider (if the sample converts) — what the island shows, coordinated with ticket 04's
  route statuses so the preview and the eventual download agree.

This is the one architectural fact ticket 07 still waits on: whether Export adds a **third**
route and an island, and how that reconciles with ADR 0001.
