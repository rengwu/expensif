---
type: task
blocked_by: [03]
assets: [.plan/export-csv/assets/export-page.html.approved]
claimed_by: s1b45620e7b79
claimed_at: 2026-07-25T18:22:50Z
---

# The live-preview endpoint and island

## Question

Deliver the client side of the spec's **live-preview endpoint** (09): make ticket 03's
first-paint preview update **live** as the filters change, via a third route and the app's
**second vanilla-TS island**. Raw-only, so it does zero rate work and can never 503 — the entire
provider-failure surface stays on `/export.csv`.

Build:

- **`GET /export/preview?from&to&category&payer`** in `handlers_html.go` (it serves HTML, like
  `GET /daily/older`) — parses filters identically to `/export.csv` and returns ticket 03's
  preview **partial** as a fragment (count line + up to ten raw sample rows, formatted in Go).
- **The `export-preview` island** — the second vanilla-TS island (not React; it renders a server
  fragment). It watches the form's `from/to/category/payer` inputs, **debounces ~250 ms**, keeps
  **one fetch in flight** (a newer edit supersedes an in-flight one), and does `el.innerHTML =
  text` — parsing nothing, formatting nothing. **First paint stays server-rendered** (ticket 03),
  so the island fetches only **on change**. `data-props` is just the four filter values
  (`prefCurrency` drops — a raw sample never converts). Register/build it through Vite like the
  existing islands.
- **Error / empty states**, coordinated with ticket 01's route statuses:
  - **Zero matches** → the fragment renders `0 expenses` + "No expenses match these filters" — a
    **200**, agreeing with the header-only download; the Download button stays enabled.
  - **Bad / inverted / unparseable range typed mid-edit** → the endpoint returns **400** (reusing
    `httpDateRangeError`); the island treats it **softly** — an inline "Check the dates" hint that
    leaves the last good preview and the Download button in place.
  - **503 cannot occur** (no rate work).
  - **Network / 5xx** → "Preview unavailable"; there is no armed observer to disarm (it's
    input-driven), so the next filter edit retries naturally — no Retry button, and a persistent
    failure never blocks the Download button.

Testing: the island and fragment are **eyeballed live in a browser** against the approved markup,
not asserted (per the spec — the machine-readable contract is the CSV bytes, tested in tickets
01/02; the preview is chrome). A thin handler test that `/export/preview` returns the fragment for
valid filters and a soft 400 for a bad range is reasonable if it mirrors existing `handlers_html`
tests, but assert status/shape, not markup.

Done when: editing any filter on `/export` updates the count + sample within a debounced beat; a
burst of keystrokes is one request; a mid-edit inverted range shows the soft "Check the dates"
hint without clearing the last good preview; a killed network shows "Preview unavailable" and
recovers on the next edit; and the Download button is never blocked by the preview.
