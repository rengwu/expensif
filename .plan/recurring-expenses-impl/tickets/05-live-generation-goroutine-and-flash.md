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
