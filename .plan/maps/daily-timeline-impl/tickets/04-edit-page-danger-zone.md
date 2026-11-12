---
type: task
blocked_by: []
assets: [../../daily-timeline/assets/edit-delete-danger-zone.html.approved]
---

# The edit page's danger zone

## Question

Deliver the spec's **Delete and the edit page** decisions from the approved asset: a
"Danger zone" at the foot of the edit page as a **sibling** form (the page is already
a form; nesting is invalid HTML) posting to the existing delete route behind a
`confirm()` naming the expense ID. The return path is a server-issued `?return=`: the
edit handler passes it through to a hidden field, and the delete handler validates it
as a local path (begins with `/`, not `//`, no scheme) with a `/` fallback.

Independent of the ledger: nothing sends `?return=` yet (the ledger ticket threads it
into the row's Edit link), but the plumbing is fully verifiable by hand-typed URL.
Touch neither the List View nor its data-table — it is the intentional bulk-delete
surface.

Tests: the delete handler's `?return=` validation (accepted local path, rejected
`//host`, rejected scheme, absent → `/`), following the existing handler-test shape.
Verify live: delete an expense from the edit page on a scratch DB and land where
`?return=` says.

Done when: delete works from the edit page with confirm and a validated return, and
checks are green.

## Answer

Shipped as approved. The danger zone is the asset's markup verbatim in
`templates/edit.html`, a sibling of the edit form; `PageData` gains `ReturnTo`
(`internal/web/renderer.go:42`); `HandleEdit` passes `?return=` through to the hidden
field; `HandleDelete` redirects to `localPath(r.FormValue("return"))`.

**Validation sits at the redirect, not at the pass-through** — deliberately, and it is
the one design call this ticket made. `HandleEdit` renders whatever `?return=` says
into the hidden field without judging it (`html/template` escapes it, so the page is
safe either way); `localPath` (`handlers_html.go:454`) is the single gate, on the one
line that can actually send a browser somewhere. Validating in both places would have
put the rule in two homes to go stale in one. `localPath` rejects anything that could
leave the origin — no leading `/`, a leading `//`, a leading `/\` (browsers fold the
backslash into a second slash), or a `url.Parse` yielding a scheme or host — and falls
back to `/`. The two rules the spec named are there; `/\` and the `url.Parse` check are
the same rule enforced against the vectors it named, not new policy.

Tests: `TestHandleDelete_ReturnPath` (`handlers_html_test.go:57`), a table of seven —
the spec's four (local path honoured, `//host` rejected, scheme rejected, absent → `/`)
plus empty, `/\host`, and a bare relative path. Each asserts the redirect *and* that the
expense died regardless: a rejected return path must not veto the destructive action.

Verified live on a scratch DB (`DATA_DIR=<tmp> PORT=8090`), not the user's: the edit
page rendered the zone with `value="/daily?date=2026-07-10"`, deleting landed on
`/daily?date=2026-07-10`, and a hand-typed `?return=https://evil.example/x` rode into
the hidden field and still redirected to `/`. Both expenses were really gone.

**Known gap, recorded not fixed:** the return path does not survive a *failed* save.
`HandleUpdate`'s error branch re-renders the edit page without `ReturnTo` (the edit
form does not carry `return` as a field, so the POST never sends it), and a delete from
that re-rendered page falls back to `/`. Closing it means threading `return` through the
shared `form` partial, which `new.html` also uses — more surface than this ticket owns,
and it belongs with [the ledger](./02-the-ledger.md), which is what starts *sending*
`?return=` in the first place. Nothing the spec promised is broken; the normal flow
(GET the edit page, delete) is exact.

Untouched, per the ticket: the List View and its data-table — still the intentional
bulk-delete surface. `go test ./...`, `go vet ./...` green; `gofmt` drift in
`renderer.go`'s `PageData` block (pre-existing, from ticket 01) fixed in passing.
No TypeScript changed.
