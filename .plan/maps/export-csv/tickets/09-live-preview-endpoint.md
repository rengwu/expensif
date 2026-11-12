---
type: grilling
blocked_by: [02, 04, 05]
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

## Answer

**A raw-only sample behind a third route, `GET /export/preview`, returning an HTML fragment
that a second vanilla-TS island swaps in. No rate work, so the preview can never 503 — only
the download can.** Every decision below falls out of the raw-sample choice, and the whole
thing echoes the daily-scroll island's settled shape: logic and formatting stay in Go, the
island holds no domain knowledge.

- **Raw amounts only — the sharp one, decided.** The sample shows each row's entered
  `Amount`+`Currency`, *not* `amount_converted`. Conversion is a download-time concern:
  showing it would run ticket 02's per-date backfill (a Frankfurter time-series call, upsert —
  slow, **503-capable**) on **every filter change**, or read only already-cached rates and show
  a partly-blank converted column until the real download warms them. Neither serves the
  preview's job, which is reassurance about **which rows and how many**. The page's own static
  note already tells the user amounts convert at each expense's date on download
  (`export-page.html.approved:110-113`). *Consequence that matters:* the preview does **zero
  rate work**, so it **cannot 503** — the entire provider-failure surface stays on
  `/export.csv`, and the island's error handling shrinks accordingly (below).

- **Route: `GET /export/preview?from&to&category&payer`, in `handlers_html.go`.** A **third**
  Export route beside `/export` (the page) and `/export.csv` (the download). Filters map 1:1
  onto `ExpenseFilter`, identically to `/export.csv` (ticket 04) — same parse, same empty =
  all. It lives in `handlers_html.go`, not `handlers_api.go`, because it **serves HTML**, the
  way `daily-timeline`'s `GET /daily/older` landed in `handlers_html.go` once its payload
  became a fragment rather than a JSON DTO.

- **Payload: a rendered HTML fragment from a Go partial, not JSON.** The endpoint returns a
  small fragment — a count line plus the sample rows — with money and dates formatted **once,
  in Go**, by the same `FormatFloat(v,'f',-1,64)` rule the CSV uses (ticket 03). The island
  `fetch`es and does `el.innerHTML = text`; it parses nothing and formats nothing. This is the
  lesson `daily-timeline` ticket 07 was *undermined* to learn: a JSON DTO forced the TSX to
  re-format money/dates and drift from the Go/CSV formatting at the seam. A fragment has no DTO
  to drift. It also matches **ADR 0001**'s existing vanilla-TS island — see the architecture
  note below.

- **Sample size: first `N = 10` rows.** Enough to reassure (ticket 05's "a few sample rows"),
  a screenful, and the natural `LIMIT`. Ordered by the same clause `/export.csv` emits, so the
  sample is genuinely "the first rows of your file", not an arbitrary 10.

- **Query: a new `CountMatching` (`SELECT COUNT(*)`) for the exact total, plus a bounded
  `LIMIT 10` read for the sample — both built from the same filter→WHERE as the download.**
  The seam grows a sibling of ticket 01's `ExpensesMatching`:

  ```go
  // repository, plus a service passthrough — mirrors ExpensesMatching
  CountMatching(ctx, ExpenseFilter) (int, error)   // SELECT COUNT(*) … <same WHERE>
  ```

  The count is a real `COUNT(*)`, **not** `len()` of a materialised set — materialising every
  matching row on each keystroke (a broad filter is thousands of rows) was rejected by the
  ticket's own cost note. A `LIMIT`-only "10+" was rejected too: ticket 05 wants a **live row
  count**, and an inexact one can't be said to equal the download. So two cheap queries per
  preview: one `COUNT(*)`, one `LIMIT 10`. **The invariant that makes the count honest** is
  that the download's `ExpensesMatching`, the preview's `CountMatching`, and the sample read
  all construct their `WHERE` from the *same* `ExpenseFilter` via one shared predicate helper —
  the count equals `/export.csv`'s row count **by construction**, not by coincidence. (This
  gently **refines ticket 01**: Export now drives *three* reads off `ExpenseFilter`, not one —
  but all Export, all from the same filter value, so 01's "one deep filtering concept" premise
  holds. Not an undermine; the same shape ticket 08 gave 01/02 when it factored testable
  seams.)

- **The island: input-driven, debounced, one fetch in flight — no scroll, no observer.**
  It watches the form's `from/to/category/payer` inputs, **debounces ~250 ms** so a burst of
  keystrokes is one request, and keeps a single fetch in flight (a newer edit supersedes an
  in-flight one). This is `data-island="export-preview"` from the approved markup, now pinned
  as the **second vanilla-TS island** (not React — it renders a server fragment). **First paint
  needs no round-trip:** the `/export` page handler server-renders the initial fragment into the
  container for the page's initial filters, so the island only fetches *on change*. `data-props`
  therefore reduces to the four filter values — `prefCurrency` drops, since a raw sample never
  converts and the page's static note already names the currency.

- **Error / empty states — collapsed by the raw choice, coordinated with ticket 04.**
  - **Zero matches** → the fragment renders `0 expenses` + "No expenses match these filters".
    This is **not** an error: it agrees with ticket 04, where zero matches is a header-only
    **200**, so the Download button stays enabled and produces a header-only file.
  - **Bad / inverted / unparseable range typed mid-edit** → the endpoint returns **400**,
    reusing ticket 04's `httpDateRangeError`, so preview and download agree on what a bad range
    is. The island treats a 400 **softly** — an inline "Check the dates" hint, leaving the last
    good preview and the Download button in place — because a user editing a `type="date"` field
    transiently produces inverted ranges; a hard error on every keystroke would be hostile.
  - **503** → **cannot occur.** The raw sample does no rate work. The provider-failure case is
    the download's alone.
  - **Network / 5xx** → the island shows "Preview unavailable" and stops; there is **no armed
    observer to disarm** (it's input-driven, not scroll-driven), so the next filter edit retries
    naturally — no explicit Retry button is needed, and a persistent failure never blocks the
    Download button, which fetches independently.

- **Architecture, for ticket 07 and ADR 0001.** Export adds a **third route**
  (`/export/preview`) and a **second vanilla-TS island** to the app. ADR 0001 already carves out
  one vanilla-TS island (`daily-scroll`); this is the second of the same species — a server-HTML
  fragment swapped by a thin fetch-and-`innerHTML` island, deliberately **not** React precisely
  to keep Go the only formatter. Ticket 07 records this as the concrete reconciliation: the
  island count and route count Export adds are now known, and the "no island / two routes" asides
  in the map Notes and ticket 04 (already `undermined_by: [05]`) are fully discharged — 05
  predicted the reversal, 09 realises it. No new fog surfaces; the map's **Not yet specified**
  stays empty, and only ticket 07 (docs) remains.

**Terms minted** (for ticket 07 / `CONTEXT.md`): **`CountMatching`** — the `SELECT COUNT(*)`
sibling of `ExpensesMatching`, sharing its `ExpenseFilter`→`WHERE` construction so the preview's
count equals the download's row count by construction. **Export preview** — the raw-sample,
count-plus-`LIMIT 10` HTML fragment served by `GET /export/preview` and swapped in by the
`export-preview` vanilla island.
