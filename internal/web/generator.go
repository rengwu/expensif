package web

import (
	"context"
	"log/slog"
	"time"

	"expensif/internal/service"
)

// generationInterval is how often the app looks for charges it owes.
//
// An hour is far more often than correctness needs and still costs nothing: a
// tick is one indexed query plus, at most, an insert per rule — no network,
// unlike the rate refresh this goroutine is modelled on. Correctness needs
// almost nothing because every enabled rule already keeps one charge
// materialized ahead of today, so the day a charge falls due it is already in
// the ledger; a tick only lays down the *next* one, which is an interval away.
// Catch-up covers any gap longer than that. The hour is there for the two
// moments a human is watching: a rule created through the Recurring page has no
// Pending Charge in Upcoming until generation runs, and a Timezone change moves
// what "today" means.
const generationInterval = time.Hour

// Generator is the app's only caller of GenerateCharges. One goroutine, one
// loop: concurrent double-generation is impossible by construction rather than
// by locking, which is why generation lives here and not in a handler. A GET
// that mutated the database would reintroduce exactly the races this shape
// removes, so a later want for fresher charges is a shorter interval, never a
// page render.
type Generator struct {
	svc      *service.Service
	notice   *Notice
	interval time.Duration
	now      func() time.Time // injectable so tests can pin "today"
}

func NewGenerator(svc *service.Service, notice *Notice) *Generator {
	return &Generator{svc: svc, notice: notice, interval: generationInterval, now: time.Now}
}

// Run generates once immediately, then on the ticker, until ctx is cancelled.
//
// The immediate run is what makes the app self-healing: it is the thing that
// closes the gap left by a container restart or a machine that was off, and it
// is the reason Catch-up exists. It is also why nothing here is fatal — a
// generation that fails must not stop the server from serving, so it is logged
// and the next tick retries.
func (g *Generator) Run(ctx context.Context) {
	for {
		g.generate(ctx)
		select {
		case <-time.After(g.interval):
		case <-ctx.Done():
			return
		}
	}
}

// generate is one pass: mint what is owed as of today, and leave a notice for
// the next rendered page if anything was minted. A rule with an unusable
// schedule reports an error without stopping the others, so a partial failure
// still records its count.
func (g *Generator) generate(ctx context.Context) {
	today := g.today(ctx)
	minted, err := g.svc.GenerateCharges(ctx, today)
	if err != nil {
		slog.Error("recurring generation failed", "error", err, "today", today)
	}
	if minted > 0 {
		slog.Info("recurring charges generated", "count", minted, "today", today)
		g.notice.AddCharges(minted)
	}
}

// today is the current date in the user's configured Timezone, which is what
// decides whether a charge is due — not the server's zone, and never a clock
// read inside the service. Preferences are read every pass so a Timezone change
// takes effect without a restart; when they are unreadable the server's zone is
// the same fallback the renderer already makes, and generating a day early or
// late is far better than not generating at all.
func (g *Generator) today(ctx context.Context) string {
	tz := ""
	prefs, err := g.svc.Preferences(ctx)
	if err != nil {
		slog.Error("recurring generation: preferences unreadable, dating charges in the server's zone", "error", err)
	} else {
		tz = prefs.Timezone
	}
	return g.now().In(tzLocation(tz)).Format("2006-01-02")
}
