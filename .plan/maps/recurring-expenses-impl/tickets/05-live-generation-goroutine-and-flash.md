---
type: task
blocked_by: [02]
claimed_by: s44f7cbf18724
claimed_at: 2026-07-31T15:06:54Z
---

# Live generation: the background goroutine and its Flash

## Question

Make generation actually happen. Until this ticket, Rules only mint Expenses when a test asks
them to.

Build **a single background goroutine** that runs generation once at startup, then on a ticker,
cancelled by context on shutdown — the same construction as the existing exchange-rate fetcher,
which is the prior art to copy rather than improve on. The startup run is what makes the app
self-healing: it is the thing that closes a gap left by a container restart or a machine that
was off, and it is the reason Catch-up exists.

Because only one goroutine ever generates, **concurrent double-generation is impossible by
construction** rather than by locking. Preserve that property. Lazy generate-on-page-render was
considered and rejected during grilling — it would make GET requests mutate the database and
reintroduce exactly the races this design eliminates — so a later ticket wanting "freshness"
should reach for a shorter ticker, never a handler.

**The current date must be computed in the user's configured Timezone**, not the server's, since
that is what decides whether a charge is due today. Pick the ticker interval deliberately and
say why in the answer: charges are day-granular and already in the past by the time they are
generated, so a few hours of lag is invisible, and a tight interval buys nothing.

**The one seam the spec left thin — settle it first, before writing the goroutine.** Catch-up
returns a count so a caller can report it as a Flash, but a background goroutine has no request
to attach a Flash to. Something must carry "4 charges were added" from the goroutine to the next
page a human loads. The obvious shape is a small in-memory notice the goroutine sets and the
next render consumes, which fits the existing Flash plumbing in the page data and survives
exactly as long as it needs to. Confirm that shape, or propose a better one, and record the
decision in the answer — this is the one place in this map where a genuine design choice was
left open, and it should be made explicitly rather than in passing.

Consider what happens when generation fails: it must not take the server down, and a failure at
startup must not prevent the app from serving. Log it and let the next tick retry.

Tested at the existing server-lifecycle seam — the prior art already covers a background
goroutine started and cancelled cleanly on shutdown. Prove the goroutine starts, generates, and
stops on context cancellation; prove a generation error doesn't kill the server. The generation
*behaviour* itself is already covered at the service seam and must not be re-asserted here.

**Done when:** the app generates due Expenses at startup and on its ticker without duplication
across restarts; a Catch-up count reaches the next rendered page as a Flash; a generation
failure is logged and survived; shutdown is clean; and the ticker interval and the Flash
mechanism are both justified in the answer.

## Answer

Generation is live. `web.Generator` (`internal/web/generator.go`) is the app's only caller of
`GenerateCharges`; `main.go` starts it with `go web.NewGenerator(svc, htmlHandler.Notice()).Run(ctx)`
on the same context the existing rate refresher uses, so the existing `cancel()` on shutdown ends
both. `Run` is the rate fetcher's shape verbatim — generate, then `select` on `time.After` and
`ctx.Done()` — copied rather than improved on.

**The seam, settled first: `web.Notice`** (`internal/web/notice.go`). A mutex-guarded in-memory
count the generator adds to and the next *whole page* consumes exactly once, rendered through the
Flash plumbing that already exists in `PageData`. Three details are decisions, not accidents:

- **Counts accumulate until read.** Two ticks either side of a page load report their sum, so no
  Expense is ever minted without eventually being announced.
- **Fragments never consume it.** `HandleDailyOlder` builds a `PageData` too, but draws no base
  layout and so has nowhere to show a Flash; consuming there would swallow the message on a
  scroll. Consumption lives in `h.render` (every whole page) plus `HandleExportPage`, which
  carries its own view model and renders outside `h.render`.
- **A Flash the page already set wins**, and the notice waits for the page after — an error the
  human just caused is the more urgent of the two, and a courtesy message should not displace it.

The alternatives: persisting the notice (a table, a "seen" flag and a GET that writes, all to
redeliver one sentence — the message is a courtesy, the Expenses are the durable record);
deriving it at render time from recently-minted rows (needs a watermark, same problem); and
dropping the Flash entirely (fails user story 25). Losing it on a crash costs nothing that
matters. The handler owns the Notice and hands it out (`htmlHandler.Notice()`) rather than being
handed one, because the rendered page is the only place a Flash can appear — that also kept every
existing `NewHTMLHandler` call site unchanged and introduced no nil to guard.

**Ticker interval: one hour** (`generationInterval`). Correctness needs almost nothing, because
every enabled rule already keeps one charge materialized *ahead* of today: the day a charge falls
due it is already in the ledger, and a tick only lays down the next one, an interval away.
Catch-up covers anything longer. The hour is for the two moments a human is watching — a rule
created through the Recurring page has no Pending Charge in Upcoming until generation runs, and a
Timezone change moves what "today" means — and it costs one indexed query plus at most an insert
per rule, no network, so it is far cheaper than the 6-hourly rate fetch it sits beside. Anything
tighter buys nothing at day granularity.

`today` is `time.Now().In(tzLocation(prefs.Timezone))`, read from Preferences on **every** pass so
a Timezone change takes effect without a restart; unreadable preferences fall back to the server's
zone (the same fallback the renderer already makes) and log, because generating a day early beats
not generating.

**Done-when, clause by clause.**

- *Generates at startup and on the ticker* — `TestGenerator_RunsAtStartupAndStopsOnCancellation`
  leaves the interval at an hour, so anything minted came from the startup run;
  `TestGenerator_SurvivesAFailedRunAndRetriesOnTheNextTick` mints only after a later tick.
- *Without duplication across restarts* — verified against the real app (below), and structurally
  it is ticket 02's `ON CONFLICT DO NOTHING`; the goroutine adds no state of its own.
- *A Catch-up count reaches the next rendered page as a Flash* —
  `TestGeneratedCharges_FlashOnTheNextRenderedPage` (shown once, gone on the second render) and
  `TestGeneratedCharges_FlashWaitsBehindAnErrorTheHumanCaused`.
- *A generation failure is logged and survived* — the same retry test: a rule with an unusable
  schedule (only a hand-edited database can store one) fails run after run, and the rule minted
  once repaired proves the loop outlived them.
- *Shutdown is clean* — both goroutine tests assert `Run` returns on cancellation; a real SIGTERM
  logged "shutting down server..." → "server stopped" and the process exited.

**All four tests were verified red**, not assumed: removing the notice consumption from `h.render`
fails both Flash tests; moving the startup run behind the first tick times out the lifecycle test;
turning the logged error into a `panic` fails the survival test. Green after restoring, under
`-race`. `go test ./...`, `go vet ./...`, gofmt clean. No TypeScript changed and no template
changed — `base.html` already renders a Flash — so the UI checks were not run.

**Driven against the real app**, on a scratch DB (`DATA_DIR` under the session scratchpad; the
user's `~/.expensif` was never touched), with a rent rule anchored on the 31st stored at
2026-05-31 and the Timezone set to Asia/Kuala_Lumpur. Startup logged `count=4`, the ledger's first
page showed **"4 recurring charges added"** and the second showed nothing; the minted rows were
May 31 → Jun 30 → **Jul 31** → Aug 31 — the anchor canary holding in production code — with
2026-08-31 rendering in Upcoming. A clean restart minted nothing, added no Flash and left four
expenses. With a deliberately broken rule added, startup logged the error, `GET /` still answered
200, and the good rule's charges were untouched.

**Decisions and doubts for a human:**

1. **The Flash fires for every mint, not only for Catch-up** — wording is "N recurring charges
   added", which fits ticket 02's count (backfilled *and* pending). I did not change
   `GenerateCharges`' signature to split the two, though ticket 02 sanctioned it. Steady state is
   quieter than it first looks: a run that mints nothing says nothing, so the message appears once
   per charge cycle per rule, reading "1 recurring charge added" as the next Pending Charge lands
   in Upcoming. If a human finds that noisy, the fix is to have generation report backfilled and
   pending counts separately and flash only the former — a contained change to `generate` and
   `Notice`.
2. **A newly created rule has no Pending Charge until the next tick** (≤1 hour), because
   `CreateRule` mints nothing while `UpdateRule` does. Ticket 04 will feel this. Calling
   generation from the create handler would break the one-goroutine property this ticket exists to
   preserve, so I left it: if the gap is unacceptable, that is a decision to raise, not a liberty
   for ticket 04 to take.
3. **No `recover()` in the goroutine.** A panic in generation would still take the process down.
   The prior art has none and the map says copy it rather than improve on it, so this stays as
   named risk rather than a quiet deviation.

**Deliberately left out:** the Recurring page and Monthly Equivalent (04), the List View's
bounding (06), and the CONTEXT.md/ADR vocabulary landing, which the map assigns to the final
ticket. No Island, no template and no existing view changed.
