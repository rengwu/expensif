---
type: prototype
blocked_by: [01, 03]
claimed_by: session-0fc3eb7d
claimed_at: 2026-07-15T15:22:44Z
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
