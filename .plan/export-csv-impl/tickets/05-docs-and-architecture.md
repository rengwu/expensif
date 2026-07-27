---
type: task
blocked_by: [02, 04]
claimed_by: s2f6ac0569fd7
claimed_at: 2026-07-27T07:57:11Z
---

# Docs and architecture catch up with what shipped

## Question

Deliver the spec's **docs and architecture** obligations (07), now that the routes, the
conversion machinery, and the island all exist. The draft text lives in the planning map's
[ticket 07](../../export-csv/tickets/07-docs-and-architecture.md) — this ticket writes it into
the shipped docs, exactly as `daily-timeline`'s docs landed in `daily-timeline-impl`. Blocked by
02 and 04 — the final download behaviour and the island are the facts being recorded.

Build (docs only — no code changes, so the suite should stay green untouched):

- **Amend `docs/adr/0001-react-islands-architecture.md`'s existing "Note (daily-timeline
  effort)"** — **no new ADR**. Name the `export-preview` island as the **second** vanilla-TS
  island (fixing the stale "the first island that is not React"), mark that it is
  **input-driven** where daily-scroll is **scroll-triggered**, and keep the shared rule: React
  owns the islands that carry client-side *state or markup*; an island that only moves
  server-rendered HTML into the page does not. Record the effort's "a form + link needs no
  island" reversal here as *decided* (argued in planning ticket 05), not assumed. Use ticket
  07's drafted amendment.
- **`CONTEXT.md`** — add **Export** and **Export Preview** to `## Language` as two user-facing
  terms, with the code seam names (`ExpensesMatching`, `CountMatching`) riding *inside* the
  Export entry the way **Payer** notes `paidBy` — not as their own glossary entries. Generalize
  the **Island** entry to name both vanilla islands. Optionally touch **List View** to note its
  client-side filtering contrasts with Export, only if it reads naturally. Use ticket 07's
  drafted text; apply `domain-modeling`.
- **`AGENTS.md`** — **no change** (Export adds handlers/templates within `internal/web`, not a
  new package; the user-facing pages aren't enumerated as routes there).
- **Leftover sweep** — remove any prototype throwaways, dead template branches, or stray island
  registrations the earlier tickets left behind (as daily-timeline's docs ticket did); report if
  the sweep finds nothing.

Testing: docs only, so no new tests; run `go test ./...` / `go vet ./...` to confirm nothing
regressed, and `cd ui && npx tsc --noEmit` if any TS was touched by the sweep.

Done when: a fresh agent reading `AGENTS.md`, `CONTEXT.md`, and the ADRs gets an accurate picture
of the shipped Export feature — its three routes, its second vanilla island, and the vocabulary —
and `git grep` finds no leftover prototype scaffolding.

## Answer

**The docs now describe what shipped, and the sweep found one real class of leftover: comments in
shipped code that still spoke about earlier tickets in the future tense.** No production behaviour
changed — every edit is markdown or a comment — so the suite stayed green untouched.

### ADR 0001 — the existing note amended, no new ADR

`docs/adr/0001-react-islands-architecture.md` gains a second paragraph under its note, using ticket
07's drafted text verbatim: the Export preview is the **second** vanilla-TS island, **input-driven**
where daily-scroll is **scroll-triggered**, and the shared rule is restated in the generalized form
("React owns the islands that carry client-side *state or markup*; an island that only moves
server-rendered HTML into the page does not — whether a scroll appends it or a filter change
replaces it"). The effort's "a form + link needs no island" reversal is recorded there as *decided*,
pointing at planning ticket 05 where it was argued. The daily-scroll paragraph is left standing: "the
first island that is not React" is still true as ordering, and the staleness the ticket names is
fixed by the new paragraph naming the second. **One deviation from the draft:** the note's heading
changed from `## Note (daily-timeline effort)` to `## Note (daily-timeline and export-csv efforts)`,
because the note now carries both efforts' facts and a heading naming only one would misfile the
second for the next reader.

### CONTEXT.md — two user-facing terms, plus the Island entry generalized

**Export** and **Export Preview** land in `## Language` after **List View** (with the other
page/view terms), from ticket 07's drafted text. The seam names ride *inside* the Export entry
(`ExpensesMatching`, with `CountMatching` as the live count's sibling), the way **Payer** notes
`paidBy` — neither gets a glossary entry of its own. **Island** now reads "the Daily View's infinite
scroll *and the Export page's live preview* are vanilla-TypeScript islands that fetch and swap
server-rendered HTML". **List View was deliberately left alone** — the ticket made it optional, the
client-side/server-side contrast already lives inside the Export entry, and `domain-modeling` warns
the glossary off implementation detail, so a second mention would only add drift surface.

### AGENTS.md — changed, contrary to the ticket, and flagged

The ticket says **no change**, and its *reason* holds completely: Export adds handlers and templates
within `internal/web`, no new package, and the Layout section doesn't enumerate routes — so **the
Layout section is untouched**. But the Done-when names `AGENTS.md` as one of the three files a fresh
agent reads, and one sentence elsewhere in it was made inaccurate by this effort: the `.plan/`
inventory listed `daily-timeline` as the *active* map and knew nothing of `export-csv`,
`export-csv-impl`, or `daily-timeline-impl`. A fresh agent following it would have been pointed at a
finished map. Fixed minimally: the inventory now lists all five efforts, names the `<slug>` /
`<slug>-impl` pairing, and the "read `.plan/daily-timeline/map.md`" pointer became a generic "read
the map's `map.md`" — so the sentence can't rot the same way again. **This is surfaced, not slipped
in:** planning ticket 07 decided "no `AGENTS.md` edit" while considering only the routes/package
question. If a human reads the decision as absolute, revert that one hunk; nothing else depends on it.

### Leftover sweep — found something, in comments rather than code

No prototype throwaways, dead template branches, or stray island registrations: `vite.config.ts`'s
six entries all resolve to real files, `templates/` holds no orphans, and the working tree was clean.
What the sweep *did* find was five comment blocks written mid-effort that still described shipped
work as pending, which is the same inaccuracy this ticket exists to fix:

- `internal/web/export.go` — "The live updating (island + `/export/preview`) is ticket 04; on this
  page the preview is static" (false since 04), the `rateResolver` doc still saying "this ticket
  passes a no-op resolver, ticket 02 supplies the real historical-rate lookup" (false since 02 — the
  handler passes the real one), and one "in ticket 04, the island" aside.
- `templates/export.html` — "until that island ships … (not yet live)" and "until it ships, this
  preview is static", twice asserting a static preview that is now live.
- `templates/partials/export-preview.html` — "served standalone as a fragment by ticket 04", now
  simply `GET /export/preview`.

All rewritten to describe the present, with ticket numbers dropped in favour of the thing itself
(provenance references that are still true — "built against the ticket 05 approved markup" — stay).

**One leftover deliberately *not* removed, for a human to rule on:** the `data-props` attribute on
`templates/export.html`'s `[data-island="export-preview"]` mount is **dead**. Only `hydrate.tsx`
(the React path) reads `dataset.props`, and ticket 04's island reads the four filter values live off
the form instead — so nothing consumes it, and it misleads a reader into thinking the island is
prop-driven. It survives here because the spec explicitly prescribes it ("`data-props` reduces to
the four filter values"), and deleting spec'd markup is a deviation, not a sweep. It is a one-line
removal whenever a human agrees.

### Checks

`go vet ./...` clean; `go test ./...` green (repository, service, web). No TypeScript was touched, so
`tsc`/`vitest` were not re-run. Because template *comments* changed, the templates were re-parsed for
real: the server booted against a scratch `DATA_DIR` on :8093 and `GET /export`, `GET /export/preview`
and `GET /export.csv` each returned **200**, proving the edits didn't break template parsing at
startup. The `review-code` skill's two axes were run as sequential passes rather than subagents;
Standards caught one thing worth fixing — the new glossary entries were hard-wrapped while every
existing `CONTEXT.md` entry is a single unwrapped line — and they were unwrapped to match.

**Nothing here undermines another ticket.** The zero-match count-line wording that blocker 04 flagged
in ticket 03's partial is untouched: it is a behaviour question inside a resolved ticket's scope, not
a docs one, and it is already recorded on 04 for a human.
