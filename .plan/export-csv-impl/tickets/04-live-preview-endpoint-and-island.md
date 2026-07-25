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

## Answer

**The code for this ticket was already written and committed before this session claimed it** —
`5937f81` (`feat: resolve ticket 04 …`) and `9b9fe3d` (the map's Decisions-so-far entry). What was
missing was this `## Answer`, which is what a ticket's status is derived from, so chartr saw 04 as
unresolved and spawned this session against work already on disk. The wayfinder linter names the
same gap: *"Decisions-so-far lists 04, which is claimed, not resolved."* This session therefore did
not rewrite the implementation. It **re-read the delivered code against the Done-when, ran the
live-browser verification the previous session explicitly deferred to the operator**, and recorded
the outcome here. The delivery below is a description of what is on disk, confirmed working, not a
fresh build.

**What is built.** `GET /export/preview` (`internal/web/export.go`, routed in `server.go`) parses
`from/to/category/payer` through the same `parseExportRange` + `parsePayerID` the download uses,
calls `CountMatching` and `SampleMatching` — and touches **no rate code at all**, so the route has no
503 branch to reach; the whole provider-failure surface stays on `/export.csv`. It renders ticket
03's `export-preview` partial through a small `exportPreviewData` view model, which is why
`Renderer.RenderFragment` widened from `PageData` to `any`. The island (`ui/src/entries/
export-preview.ts`, registered in `vite.config.ts` as `exportPreview`) is the second vanilla-TS one
beside `daily-scroll`: it reads the four values **live off the form** rather than from `data-props`,
debounces 250 ms, keeps one fetch in flight via `AbortController` (the older fetch's `catch` sees
`signal.aborted` and bows out), and does `slot.innerHTML = text` — parsing and formatting nothing.
It splits the mount into a slot holding the last good preview plus a hidden hint line, seeded from
the server's first paint, so no fetch happens until an edit.

**Each Done-when clause, verified live** (production Vite build, `DATA_DIR=<scratch> PORT=8090`,
13 seeded expenses across five categories, two currencies and one payer; driven in Chrome):

- *Editing any filter updates count + sample within a debounced beat* — all four filters exercised
  individually: `payer=1` → `1 matching expense`; clearing it → `13`; `from=2026-07-01` → `5`;
  `to=2026-07-31` → `5`. Category likewise (`food` → 5, `utilities` → 2, `travel` → 3). Rows stay
  **raw** (`12.5 USD`, `80 EUR`, `1.234 KWD`).
- *A burst of keystrokes is one request* — six `change` events 20 ms apart produced **exactly one**
  `/export/preview` call (counted at a patched `window.fetch`, corroborated by the server log), and
  the rendered result was the **last** value in the burst, not a stale one.
- *Mid-edit inverted range → soft hint, last good preview intact* — `from=2026-05-01&to=2026-04-01`
  returned 400; the amber "Check the dates" line appeared **below** an unchanged `2 matching
  expenses` preview, with `button[type=submit].disabled === false`.
- *Killed network → "Preview unavailable", recovers on the next edit* — verified for both a
  rejecting `fetch` (dropped connection) and a 500 response. Three consecutive fail→recover cycles:
  each showed the hint over the retained preview, and the **next** edit alone repainted correctly
  (3 / 2 / 5 matches) and cleared the hint. No Retry button; nothing to disarm.
- *Download never blocked* — the submit button was enabled in every state above, and the download for
  the live filters matched the preview exactly: preview `5`, CSV 6 lines (header + 5), CRLF intact.

**Endpoint contract, checked on the wire:** bare request → 200 with `12 matching expenses` against a
13-line CSV (header + 12); zero matches → **200** with the empty state, agreeing with the header-only
download; inverted range and unparseable date → **400** each.

`go vet ./...`, `go test ./...`, `cd ui && npx tsc --noEmit`, `cd ui && npx vitest run` (7 passing)
are all green. `TestHandleExportPreview` asserts status/shape only — 200 with both sample rows, 200
on zero matches, 400 on bad and inverted ranges — never markup, as the ticket asks.

**Deliberately not done, and two things flagged:**

- The handler lives in `export.go` beside `HandleExportCSV`/`parseExportRange`, not `handlers_html.go`
  as this ticket's prose says. The prose predates the `export.go` split; already surfaced on the map
  entry and left as-is.
- **The zero-match fragment renders only "No expenses match these filters" — not the `0 expenses`
  count line this ticket describes.** The count line is inside the `{{if .PreviewCount}}` branch of
  ticket 03's partial, so a zero count drops it. Status (200) and behaviour (Download stays enabled)
  are exactly as specified; only the literal count text differs. Left unchanged because the partial is
  ticket 03's, already resolved — a human should decide whether that is a wording slip in this ticket
  or a real gap in 03.
- Two of roughly a dozen recovery observations did not repaint within a ~800 ms settle window while
  the network-failure harness was monkey-patching `window.fetch`; every clean run at a 1.5 s window
  recovered correctly, and no real-server failure ever reproduced it. Recorded as a harness-timing
  artifact rather than an island defect, but noted here rather than swept up.
