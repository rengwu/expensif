---
type: task
blocked_by: [01]
assets: [.plan/export-csv/assets/export-page.html.approved]
---

# The Export page and its preview fragment

## Question

Deliver the spec's **Export page UI** (05) and the server side of the **live-preview endpoint**
(09) as a working page rendered against
[`export-page.html.approved`](../../export-csv/assets/export-page.html.approved). This ticket
ships the page and its first-paint preview; the *live* updating (island + `/export/preview`
route) is ticket 04. Blocked by 01 only — the preview is **raw-only** and does no rate work, so
it does not wait on ticket 02's conversion.

Build:

- **`GET /export`** — the guided single-column page: date range as **presets-as-links**
  (*This month / This year / All time*, server-filling `from`/`to`) plus a **`Custom…`** toggle
  revealing two `<input type="date">`; **Category** and **Payer** as single `<select>`s
  (`""`/`0` = "All …"). Category options come from a net-new `SELECT DISTINCT category` (category
  is free-text upstream); Payer options are the existing users. The form is `method="GET"`,
  `action="/export.csv"`, emitting exactly `from,to,category,payer` (ticket 01's route). Add the
  nav entry in the **`⋮` overflow menu**, beside Preferences — not top-level.
- **`CountMatching(ctx, ExpenseFilter) (int, error)`** — a `SELECT COUNT(*)` sibling of
  `ExpensesMatching`, built from the **same** filter→`WHERE` helper (ticket 01) so its count
  equals the download's row count by construction. Plus a bounded `LIMIT 10` sample read ordered
  by the same clause `/export.csv` emits.
- **The preview partial.** A reusable Go partial rendering a count line plus up to ten **raw**
  sample rows — entered `amount`+`currency`, *not* `amount_converted` — money/dates formatted
  once in Go by the same `FormatFloat(v,'f',-1,64)` rule the CSV uses. The `/export` handler
  server-renders this partial into the preview container on **first paint** for the page's
  initial filters. (Ticket 04 serves the same partial as a fragment over `/export/preview`.)

Testing: the page HTML is **prototyped and eyeballed** against the `.approved` asset, never
asserted (per the spec — no chrome/class-name/copy tests). `CountMatching` gets a service test
mirroring `ExpensesMatching`'s (each filter, combinations, unset = all), and the invariant that
count and the download's row count agree for the same filter is worth a direct assertion.

Done when: `/export` renders matching the approved markup on a scratch DB; presets and the
Custom toggle set the range; Category/Payer selects are populated from the account's distinct
categories and users; submitting downloads the right filtered file via `/export.csv`; and the
first-paint preview shows a correct count + raw sample for the initial filters (not yet live).

## Answer

Delivered as specified. The seam additions landed first (they broke the mocks' compile until the
methods existed), then the page and preview turned green.

- **Filtering seam grows two siblings on the shared `filterWhere`.** `CountMatching` (`SELECT
  COUNT(*)`) and `SampleMatching(f, limit)` (`LIMIT 10`) join `ExpensesMatching` on one new private
  `queryExpenses(f, limit)` helper — same column list, same `ORDER BY`, optional `LIMIT` — so the
  sample is a genuine prefix of the download and the **preview count equals the download row count by
  construction**. That invariant is asserted at the service seam (`TestCountMatching`) and, over
  in-memory SQLite, at the repository seam (`expenses_test.go`, extending ticket 02's `db.Migrate`
  pattern). A net-new `DistinctCategories` (`SELECT DISTINCT category`) feeds the dropdown — separate
  from the recency/frequency-ranked `ListCategories` the add form uses. All three are thin service
  passthroughs, matching the existing seam style.
- **`GET /export`** renders `templates/export.html` (the approved ticket-05 markup) through a new
  `ExportPageData` view model that embeds `PageData` and adds the page's controls + first-paint
  preview; `Renderer.RenderPage(any)` renders it (the base layout reads only the promoted fields). A
  pure `exportRange(preset, from, to, today)` resolves the presets server-side into effective
  `from`/`to` by string surgery on the account's `YYYY-MM-DD` today — so a preset fills the (hidden
  but still-submitted) date inputs and the download the form issues matches the preview exactly.
  Custom bounds flip `CustomRange`, revealing the inputs. A bad/inverted range on the *page* stays
  forgiving (renders chrome, previews nothing) — the 400 belongs to `/export.csv`, not here.
- **The `export-preview` partial** renders a count line plus up to ten **raw** rows (entered
  `amount`+`currency`, never converted), money pre-formatted in Go via `ExportSampleRow` by the CSV's
  `FormatFloat(v,'f',-1,64)` rule — so the template (and ticket 04's island) render strings with no
  formatting logic. The handler server-renders it on first paint. The `data-island="export-preview"`
  mount is present but **not** added to `data.Islands`, so it stays inert — first paint is static,
  the live fetch is ticket 04.
- **Nav** sits in the `⋮` overflow beside Preferences on desktop (base.html) and mobile — `MobileNav`'s
  single `preferences` slot generalised to a `secondary[]` array carrying Export + Preferences.
- **Two spec-consistent notes** (surfaced, not slipped in): the island `data-props` drops
  `prefCurrency` per the spec's live-preview contract (a raw sample never converts), overriding the
  older ticket-05 asset that still carried it (the asset self-labels `data-props` "provisional"); and
  the overflow-menu entry was carried to mobile too, so "beside Preferences" holds on both.
- Verified live on a scratch DB: the preview count equalled the submitted-filter download across
  `preset=all|month`, a category narrow, and a custom range; sample rows showed raw amounts
  (`1.234 KWD`, `12.5 USD`, `80 EUR`); the category dropdown listed the distinct categories and kept
  its selection; the active preset highlighted; and an out-of-range filter showed the empty state.
