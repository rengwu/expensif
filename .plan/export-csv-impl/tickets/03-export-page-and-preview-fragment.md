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
