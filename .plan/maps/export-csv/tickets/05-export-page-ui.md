---
type: prototype
blocked_by: [01, 03]
assets: [.plan/export-csv/assets/export-page.html.approved]
---

# The Export page UI

## Question

Design the page itself — a HITL prototype, because "how should it look" is the question.
Waits on 01 (what filters exist) and 03 (what the file will contain, so the page can
describe it).

- **The controls.** A date-range pair, a category selector, a payer selector, and a
  download button. Decide each control's form: category/payer as dropdowns, checkboxes, or
  a multi-select; the date range as two date inputs or a preset ("this month", "this
  year", "all time") plus custom. Ground the options in ticket 01's filter model — the UI
  can only offer what the seam supports.
- **Server-rendered, no island?** The standing preference is server-render, and a form +
  link should need no island (unlike `/expenses`, whose `data-table` island is exactly
  what Export sidesteps). Confirm the whole page is a plain `<form>` — ticket 07 checks
  this against ADR 0001, but flag here if any control tempts an island.
- **Preview or blind download?** Does the page show a row count / sample before download,
  or is the button a blind fetch? A count reassures; it also costs a query.
- **Nav placement.** Where does "Export" live in the existing nav, alongside Daily,
  Calendar, Expenses, Preferences?
- **Empty and error states** on the page, coordinated with the route's answers (04) for
  zero-match and bad-range.

Link the prototype markup as an asset under `assets/` and, once approved, as an
`.approved` file the impl session renders against.

## Answer

**A hybrid won: variant C's guided two-step single-column layout, carrying variant B's
*live* preview. The page takes an island — reversing the effort's "no island" preference —
and that reversal surfaces a new ticket.** Approved markup:
[`assets/export-page.html.approved`](../assets/export-page.html.approved).

Three radically different variants were built behind one `?variant=` switcher
(`assets/export-page-prototype.html`, since deleted) and flipped through live:

- **A — Plain form.** Blind download, one card, zero JS. *Lost:* no reassurance before
  committing to a download.
- **B — Presets + live preview.** Two-column; a right panel shows a live row count + sample
  rows + filename, refetched as filters change. *Its live preview won; its two-column layout
  lost.*
- **C — Guided.** Single column, two labelled steps; step 2 was a *static* "here's the file
  shape" (fixed header + example row, no query). *Its layout won; its static preview lost.*

Decisions:

- **Controls.** Date range = **presets + custom** (`This month / This year / All time`
  chips as links that server-fill `from`/`to`; a `Custom…` toggle reveals two `<input
  type="date">`). **Category and Payer are single-`<select>` dropdowns**, not multi-select —
  ticket 01's seam is single-value equality (`Category string`, `PayerID int64`), so the UI
  cannot offer more; `""`/`0` are the "All …" default options. The form is `method="GET"`,
  `action="/export.csv"`, emitting exactly `from,to,category,payer` (ticket 04).
- **Category options come from the account's DISTINCT existing categories.** Category is
  **free-text** in this app (the add form uses a `category-pills` island, `templates/form.html`
  — no enum), so the dropdown is populated from a small `SELECT DISTINCT category`. This is a
  net-new query the `-impl` map adds; payer options are the existing users.
- **Preview: live, not blind or static.** The user chose B's live preview over both A's
  blind button and C's static shape, and **explicitly accepted an island/fetch here.** So
  the page **carries one island** (`data-island="export-preview"`): it watches the filter
  inputs and refetches a count + sample as they change. This is a deliberate reversal of the
  map's standing "a form + link needs no island" preference — recorded so ticket 07 records
  the *fact*, not the assumption, against ADR 0001.
- **Nav placement: the `⋮` overflow menu**, beside Preferences (`base.html:26-28`), **not**
  top-level. Export is a less-frequent action; Daily / Calendar stay the top nav.
- **Empty and error states belong to the route, not the page.** Bad/inverted range → 400,
  rates unavailable → 503, zero matches → header-only 200 are all ticket 04's, owned by
  `/export.csv`. The page's own only "state" is the preview island's loading / empty / error,
  which is ticket 09's contract, not chrome decided here.

**Surfaces a new ticket (created: 09 — the live-preview endpoint contract).** The live
preview needs a *server contract* the prototype could not answer: the endpoint's route and
payload, and crucially **whether the sample shows converted amounts** — which would run
ticket 02's per-date rate backfill (potentially slow, 503-capable) on every filter change —
**or raw amounts**. The `.approved` markup's `data-props` on the mount is provisional pending
that decision.

**Undermines ticket 04's "no island" fact.** Ticket 04 predicted "07 confirms no island /
two new routes." The page now has an island, and ticket 09 may add a third (preview) route.
Ticket 04's streaming-download, error-order, and filename decisions are untouched; only that
forward-looking aside is overturned (04 marked `undermined_by: [05]`).
