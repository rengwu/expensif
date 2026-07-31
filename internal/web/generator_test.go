package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
	"expensif/internal/service"
)

// generatorFixture builds a service over the mock repository with the rules
// wired in — generation is the only thing in the app that reads them.
func generatorFixture(t *testing.T) (*mockRepo, *service.Service) {
	t.Helper()
	repo := newMockRepo()
	repo.prefs = domain.Preferences{Currency: "USD", Timezone: "UTC"}
	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
		Rules:       repo,
	}, &mockRateClient{})
	return repo, svc
}

func monthlyRule(name, nextCharge string) domain.RecurringRule {
	return domain.RecurringRule{
		Name: name, Amount: 15.99, Currency: "USD", Category: "subscriptions",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 15,
		NextChargeDate: nextCharge, Enabled: true,
	}
}

// waitFor polls until cond holds, failing the test if it never does. The
// goroutine under test runs on its own schedule, so there is nothing to
// synchronise on but its effect on the repository.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func expenseCount(t *testing.T, repo *mockRepo) int {
	t.Helper()
	expenses, err := repo.ListExpenses(context.Background(), 100)
	if err != nil {
		t.Fatalf("list expenses: %v", err)
	}
	return len(expenses)
}

// TestGenerator_RunsAtStartupAndStopsOnCancellation is the lifecycle assertion:
// the goroutine mints without being asked to, and the same context that shuts
// the server down ends it. The interval is left long deliberately — anything
// generated here came from the startup run, which is the run that closes the
// gap a restart left behind.
func TestGenerator_RunsAtStartupAndStopsOnCancellation(t *testing.T) {
	repo, svc := generatorFixture(t)
	repo.CreateRule(t.Context(), monthlyRule("Netflix", "2026-07-15"))

	g := NewGenerator(svc, &Notice{})
	g.interval = time.Hour
	g.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		g.Run(ctx)
		close(stopped)
	}()

	waitFor(t, "the startup run to mint a charge", func() bool { return expenseCount(t, repo) > 0 })

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("generator did not stop on context cancellation")
	}
}

// TestGenerator_SurvivesAFailedRunAndRetriesOnTheNextTick pins the promise that
// a broken rule cannot take the app with it. Only a hand-edited database can
// store an interval count of zero, which is exactly why the loop has to outlive
// meeting one: the run reports its error, the goroutine keeps its ticker, and
// the rule minted once it is repaired proves both.
func TestGenerator_SurvivesAFailedRunAndRetriesOnTheNextTick(t *testing.T) {
	repo, svc := generatorFixture(t)
	broken := monthlyRule("Netflix", "2026-07-15")
	broken.IntervalCount = 0
	id, err := repo.CreateRule(t.Context(), broken)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	g := NewGenerator(svc, &Notice{})
	g.interval = 10 * time.Millisecond
	g.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		g.Run(ctx)
		close(stopped)
	}()

	// Several ticks' worth of failing runs. A rule with no usable schedule can
	// never mint, so this is a wait, not a race.
	time.Sleep(100 * time.Millisecond)
	if n := expenseCount(t, repo); n != 0 {
		t.Fatalf("expected a broken rule to mint nothing, got %d expenses", n)
	}

	repaired := broken
	repaired.ID = id
	repaired.IntervalCount = 1
	if err := repo.UpdateRule(t.Context(), repaired); err != nil {
		t.Fatalf("update rule: %v", err)
	}
	waitFor(t, "the tick after the failures to mint", func() bool { return expenseCount(t, repo) > 0 })

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("generator did not stop on context cancellation")
	}
}

// TestGeneratedCharges_FlashOnTheNextRenderedPage follows the seam end to end:
// a goroutine with no request to answer leaves a count on the notice, and the
// next whole page a human loads shows it — once. The loop itself is tested
// above, so this drives a single pass.
func TestGeneratedCharges_FlashOnTheNextRenderedPage(t *testing.T) {
	repo, svc := generatorFixture(t)
	// Five days late for its July charge, so the run mints that one and the
	// August Pending Charge behind it: two Expenses, one message.
	repo.CreateRule(t.Context(), monthlyRule("Netflix", "2026-07-15"))

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	g := NewGenerator(svc, h.Notice())
	g.now = h.now
	g.generate(t.Context())

	body := getBody(t, h.HandleDaily, "/")
	if !strings.Contains(body, "2 recurring charges added") {
		t.Fatal("the generated charges never reached the page as a Flash")
	}
	if strings.Contains(getBody(t, h.HandleDaily, "/"), "recurring charge") {
		t.Fatal("the Flash was shown a second time: a notice is consumed by the page that shows it")
	}
}

// TestGeneratedCharges_FlashWaitsBehindAnErrorTheHumanCaused covers the one
// place two Flashes compete for one slot. The rejected form is the message the
// human is waiting for; the background notice is a courtesy, so it stands aside
// rather than being overwritten and lost.
func TestGeneratedCharges_FlashWaitsBehindAnErrorTheHumanCaused(t *testing.T) {
	repo, svc := generatorFixture(t)
	repo.CreateRule(t.Context(), monthlyRule("Netflix", "2026-07-15"))

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	g := NewGenerator(svc, h.Notice())
	g.now = h.now
	g.generate(t.Context())

	// An expense with no amount: the add page comes back carrying the error.
	form := strings.NewReader("amount=0&category=food&description=lunch&date=2026-07-20")
	req := httptest.NewRequest("POST", "/expenses/new", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if !strings.Contains(rec.Body.String(), service.ErrInvalidAmount.Error()) {
		t.Fatalf("expected the rejected form's error, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "recurring charges added") {
		t.Fatal("the notice displaced the error the human was waiting for")
	}
	if !strings.Contains(getBody(t, h.HandleDaily, "/"), "2 recurring charges added") {
		t.Fatal("the notice was consumed by the page that could not show it")
	}
}
