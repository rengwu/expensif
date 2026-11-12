---
type: prototype
blocked_by: [08]
assets: [../assets/edit-delete-danger-zone.html.approved]
---

# Where delete lives once the ledger lands

## Question

[The day-entry ledger](./08-day-entry-ledger-redesign.md) demoted delete "to the edit
page". **The edit page has no delete affordance.** Verified, not assumed:

- `templates/edit.html` is **nine lines** — a page title and a `<form>` wrapping the
  shared `form` partial. `grep -i delete` returns nothing.
- Delete exists today **only inside the `data-table` island's actions column**
  (`templates/daily.html:25`): a `type: "form"` action posting to
  `/expenses/delete/{id}`, `variant: "danger"`, with a `confirm` dialog
  (`"Delete expense #{id}?"`).
- Ticket 08 **drops `data-table` from the daily view**. So delete does not *move* —
  it **disappears**, and the page it was demoted to has nothing to receive it.
- The route survives regardless: `POST /expenses/delete/{id}` → `HandleDelete`
  (`internal/web/server.go:30`). `list.html` keeps its `data-table`, so delete still
  exists *there*. This ticket is about the daily view's loss of it, not the route's.

So the demotion was a decision to build something that does not exist. This ticket
builds it — or overturns the demotion.

This is a prototype ticket: put an affordance on `edit.html`, look at it, react.

Resolve, one at a time:

- **Does delete belong on the edit page at all?** Deleting via a page whose title is
  "Edit Expense #12" and whose submit button says "Save Changes" is a mode error waiting
  to happen. The alternative is that ticket 08's demotion was wrong and the ledger row
  needs *some* destructive affordance — but that reopens an approved design, so it needs
  a real argument, not a preference.
- **What it looks like on a nine-line page.** Danger button below the form? A separate
  form (it must be — a nested `<form>` is invalid HTML, and the page is already one
  `<form method="POST">`)? Where, relative to "Save Changes", so it is reachable but not
  mis-clickable?
- **The confirm step.** The current dialog is a JS `confirm()` supplied by the island.
  Ticket 08's whole point is that the daily view becomes **JS-free**. Does the edit page
  keep a JS confirm (it may — it is a different page), or does delete become a two-step
  server-rendered confirmation, or does it need no confirm at all once it is off the
  one-click table row?
- **The return path.** `HandleDelete` redirects — to where? From the daily view, back to
  the daily view was implicit. From the edit page, "back to daily" is a guess: the user
  may have arrived from `/list`, or from `?date=`.
- **Discoverability.** Delete moves from one click on the row to: notice the row, click
  Edit, find delete on the next page. For the "I fat-fingered the amount" case that is
  fine. Confirm there is no case where it is not.

## Answer

**Delete stays demoted to the edit page, and it is built there as a "Danger zone" at the
foot of the form.** Approved markup:
[`assets/edit-delete-danger-zone.html.approved`](../assets/edit-delete-danger-zone.html.approved).
Ticket 08's demotion stands; no approved design is reopened.

Three variants were built on the **real** edit page behind `?variant=`, against a real
expense, and thrown away once judged (A — danger zone at the foot with `confirm()`; B — a
server-rendered confirmation page showing the expense being destroyed; C — a trash icon in
the title row with a CSS-only `<details>` reveal). **Variant A won.**

### The zero-JS argument was void, and it is what decided this

B and C were both designed to need no JavaScript, on the assumption that ticket 08's
JS-free property extended to the edit page. **It does not.** `HandleEdit`
(`handlers_html.go:367`) already does:

```go
data.Islands = append(data.Islands, "category-pills", "description-pills")
```

The edit page mounts two React islands and has always required JS. Ticket 08's JS-free
property belongs to the **daily view**, and does not reach here. So B and C's central
advantage bought nothing, and A — the least new code, closest to today's behaviour — won
on the remaining merits.

### Accepted cost

`confirm('Delete expense #49?')` states the ID and nothing else. It cannot show the
amount, the description, or the date — only variant B's page could do that, and it lost.
**The guard is only as good as the number in the dialog.** Recorded here rather than
re-litigated.

### Delete must be a sibling form, never nested

`edit.html` is already `<form method="POST" action="/expenses/edit/{id}">`. A nested
`<form>` is invalid HTML, so the danger zone is a **separate sibling form** posting to
`/expenses/delete/{id}`, placed after the edit form's closing tag. This is a structural
constraint, not a preference — it is why delete cannot simply be another button in the
form's button row, and it is visible in the prototype: the delete button and "Save
Changes" sit near each other as peers in two different forms doing opposite things.

### The return path: a server-issued `?return=`

`HandleDelete` (`handlers_html.go:409-417`) currently hard-redirects to `/`. That is wrong
once delete lives on the edit page, which is reachable from **three** places: the daily
timeline, `?date=` (linked from every calendar cell — [ticket 05](./05-converge-handledaily-branches.md)),
and `/expenses`. Deleting the last expense of March 4th from `?date=2026-03-04` would dump
the user on today's timeline with the filter gone.

So: the ledger row's **Edit link carries where it came from** (`?return=/daily?date=2026-03-04`),
`HandleEdit` passes it through, and the danger-zone form submits it as a hidden field.
`HandleDelete` validates it as a **local path** — must begin with `/`, must not begin with
`//`, must not contain a scheme — and falls back to `/` when absent or rejected. Same
server-issued-state principle [ticket 07](./07-scroll-island-contract.md) used for the
scroll cursor: no client state, no `Referer` header, nothing spoofable.

Ticket 08 is rewriting the row's Edit link anyway, so this costs one attribute in a
partial that is already being built.

### `/expenses` is the bulk-delete surface — and that is now a constraint

Delete goes from one click on the row to: notice the row → Edit → danger zone → confirm.
For "I fat-fingered the amount" that is the correct price. The case it hurts is **bulk
cleanup** — clearing five test or duplicate expenses becomes five page journeys.

That case is already covered, by accident: **ticket 08 drops `data-table` from the daily
view only.** `list.html` keeps its `data-table`, and with it a row-level one-click Delete.

This is now **intentional and load-bearing**, not incidental:

> The daily view is read-and-add. `/expenses` is where expenses are removed in bulk, and
> `list.html`'s row-level Delete is the only fast path left. **Any future effort that drops
> `data-table` from `list.html` must replace that affordance first** — "finishing the
> islands migration" would otherwise silently delete the last one-click delete in the app.

### What the implementation session does

1. Build the danger zone from the approved markup, as a sibling form in `edit.html`.
2. Add `?return=` — Edit link → `HandleEdit` → hidden field → `HandleDelete`, with
   local-path validation and a `/` fallback.
3. Touch neither `list.html` nor its `data-table`.
