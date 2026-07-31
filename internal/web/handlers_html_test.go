package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
	"expensif/internal/service"
)

// TestHandleDaily_TodayNamedInPreferencesTimezone freezes the handler's
// clock late on a UTC day and asserts the window is anchored at today in
// the user's timezone. At 2026-07-11T23:00Z it is already July 12 in
// Pacific/Auckland; a handler that consults UTC ends the timeline a day
// early and today's row never renders.
func TestHandleDaily_TodayNamedInPreferencesTimezone(t *testing.T) {
	repo := newMockRepo()
	repo.prefs = domain.Preferences{Currency: "USD", Timezone: "Pacific/Auckland"}
	// One past expense so the timeline renders rather than the
	// zero-expense-ever empty state.
	repo.CreateExpense(t.Context(), domain.Expense{Amount: 5, Category: "food", Description: "x", Date: "2026-07-01", Currency: "USD"})

	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
	}, &mockRateClient{})

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 7, 11, 23, 0, 0, 0, time.UTC) }

	rec := httptest.NewRecorder()
	h.HandleDaily(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// The gap-filled window must include July 12 (today in Auckland). The
	// date string is the observable; asserting the day exists is behaviour,
	// not chrome.
	if !strings.Contains(rec.Body.String(), "Jul 12") {
		t.Fatal("window does not include 2026-07-12: today was named in UTC, not the Preferences timezone")
	}
}

// TestHandleDaily_TimelineAndSingleDayShareDayEntryPartial is the spec's one
// "same partial" assertion: the timeline and the ?date= view both draw a
// populated day through the day-entry partial. data-day is the partial's
// machine-readable marker — nothing else emits it — so its presence in both
// responses is evidence of one implementation, without asserting on chrome.
func TestHandleDaily_TimelineAndSingleDayShareDayEntryPartial(t *testing.T) {
	repo := newMockRepo()
	repo.prefs = domain.Preferences{Currency: "USD", Timezone: "UTC"}
	repo.CreateExpense(t.Context(), domain.Expense{Amount: 5, Category: "food", Description: "x", Date: "2026-07-05", Currency: "USD"})

	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
	}, &mockRateClient{})

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }

	for _, target := range []string{"/", "/?date=2026-07-05"} {
		rec := httptest.NewRecorder()
		h.HandleDaily(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: expected 200, got %d: %s", target, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `data-day="2026-07-05"`) {
			t.Fatalf("%s: the day did not render through the day-entry partial", target)
		}
	}
}

// TestHandleDaily_PendingChargeIsIndistinguishableFromAHandEnteredExpense checks
// the claim the whole design rests on rather than assuming it: a Pending Charge
// is an ordinary future-dated expense, so the Ledger needs no idea that rules
// exist. The page is rendered twice over identical data — once with the rule
// reference set, once without — and the two responses must be byte-identical.
// The rule reference is the only difference between the two, so any byte that
// differed would be the Ledger having learned a new concept.
func TestHandleDaily_PendingChargeIsIndistinguishableFromAHandEnteredExpense(t *testing.T) {
	render := func(ruleID int64) string {
		t.Helper()
		repo := newMockRepo()
		repo.prefs = domain.Preferences{Currency: "USD", Timezone: "UTC"}
		// A past expense so the timeline renders, and a future one that is the
		// Pending Charge in every run but the control.
		repo.CreateExpense(t.Context(), domain.Expense{Amount: 5, Category: "food", Description: "x", Date: "2026-07-05", Currency: "USD"})
		repo.CreateExpense(t.Context(), domain.Expense{
			Amount: 15.99, Category: "subscriptions", Description: "Netflix",
			Date: "2026-07-20", Currency: "USD", RecurringRuleID: ruleID,
		})

		svc := service.New(repository.Repos{
			Expenses:    repo,
			Users:       repo,
			Preferences: repo,
			Rates:       repo,
			Rules:       repo,
		}, &mockRateClient{})

		renderer, err := NewRenderer("../../templates", false, nil)
		if err != nil {
			t.Fatalf("renderer init failed: %v", err)
		}
		h := NewHTMLHandler(svc, renderer)
		h.now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }
		return getBody(t, h.HandleDaily, "/")
	}

	minted, handEntered := render(7), render(0)
	// Upcoming is where the forward visibility lives, and it is the section the
	// spec says a Pending Charge finally fills.
	if !strings.Contains(minted, `data-day="2026-07-20"`) {
		t.Fatal("the Pending Charge's day never reached the Ledger's Upcoming section")
	}
	if !strings.Contains(minted, "Netflix") {
		t.Fatal("the Pending Charge's description never rendered")
	}
	if minted != handEntered {
		t.Error("the Ledger renders a Pending Charge differently from a hand-entered expense")
	}
}

// dailyFixture builds a handler over a real renderer with the clock frozen at
// 2026-07-13, so the first window is [2026-06-14, 2026-07-13].
func dailyFixture(t *testing.T, dates ...string) *HTMLHandler {
	t.Helper()
	repo := newMockRepo()
	repo.prefs = domain.Preferences{Currency: "USD", Timezone: "UTC"}
	for _, d := range dates {
		repo.CreateExpense(t.Context(), domain.Expense{Amount: 5, Category: "food", Description: "x", Date: d, Currency: "USD"})
	}

	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
	}, &mockRateClient{})

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }
	return h
}

func getBody(t *testing.T, handler http.HandlerFunc, target string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != 200 {
		t.Fatalf("%s: expected 200, got %d: %s", target, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestHandleDailyOlder_CursorWalksOlderWindows follows the scroll one step: the
// page issues the first cursor, and the fragment it names issues the next. The
// island does no date arithmetic, so these two attributes are the entire
// pagination contract — nothing else about the fragment is asserted.
func TestHandleDailyOlder_CursorWalksOlderWindows(t *testing.T) {
	// Earliest expense far older than two windows, so neither cursor clamps.
	h := dailyFixture(t, "2026-01-10", "2026-07-05")

	// First window is [2026-06-14, 2026-07-13], so the one older than it is
	// the 30 days ending the day before it starts.
	page := getBody(t, h.HandleDaily, "/")
	if !strings.Contains(page, `data-next-start="2026-05-15"`) || !strings.Contains(page, `data-next-end="2026-06-13"`) {
		t.Fatalf("page did not server-render the first cursor: %s", page)
	}
	if !strings.Contains(page, `data-state="idle"`) {
		t.Fatal("foot is not armed: with older expenses to fetch, its state must be idle")
	}

	frag := getBody(t, h.HandleDailyOlder, "/daily/older?start=2026-05-15&end=2026-06-13")
	if !strings.Contains(frag, `data-next-start="2026-04-15"`) || !strings.Contains(frag, `data-next-end="2026-05-14"`) {
		t.Fatalf("fragment did not issue the next cursor: %s", frag)
	}
}

// TestHandleDailyOlder_TerminalAtEarliestExpense asserts the scroll's stop condition,
// which lives server-side: the final window is clamped to the earliest expense
// and carries no cursor. A fragment that still offered one would scroll the
// user into an infinite empty pre-history.
func TestHandleDailyOlder_TerminalAtEarliestExpense(t *testing.T) {
	// 2026-05-20 sits inside the window older than the first, so that window is
	// clamped to start there and nothing older than it exists.
	h := dailyFixture(t, "2026-05-20", "2026-07-05")

	page := getBody(t, h.HandleDaily, "/")
	if !strings.Contains(page, `data-next-start="2026-05-20"`) {
		t.Fatalf("the final window was not clamped to the earliest expense: %s", page)
	}

	frag := getBody(t, h.HandleDailyOlder, "/daily/older?start=2026-05-20&end=2026-06-13")
	if strings.Contains(frag, "data-next-") {
		t.Fatalf("the fragment at the earliest expense still carries a cursor: %s", frag)
	}
}

// TestHandleDaily_TerminalOnTheFirstWindowWhenNothingIsOlder covers the account
// whose only expense is future-dated: its earliest date sits after the window
// starts, so there is nothing older to fetch and the island must never fire.
func TestHandleDaily_TerminalOnTheFirstWindowWhenNothingIsOlder(t *testing.T) {
	h := dailyFixture(t, "2026-08-01")

	page := getBody(t, h.HandleDaily, "/")
	if strings.Contains(page, "data-next-") {
		t.Fatalf("a cursor was issued with no expense older than the window: %s", page)
	}
	if !strings.Contains(page, `data-state="end"`) {
		t.Fatal("the foot did not server-render its terminal state")
	}
}

// TestHandleDailyOlder_BadRange: the cursor is server-issued, so any range that is
// unparseable, inverted, or longer than a window is a hand-crafted request.
// None may render — an inverted range would otherwise reach the island as an
// empty fragment, indistinguishable from the terminal.
func TestHandleDailyOlder_BadRange(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"unparseable start", "start=banana&end=2026-06-13"},
		{"unparseable end", "start=2026-05-15&end=banana"},
		{"missing both", ""},
		{"inverted range", "start=2026-06-13&end=2026-05-15"},
		{"longer than a window", "start=2020-01-01&end=2026-06-13"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := dailyFixture(t, "2026-01-10")
			rec := httptest.NewRecorder()
			h.HandleDailyOlder(rec, httptest.NewRequest("GET", "/daily/older?"+tt.query, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHandleDelete_ReturnPath covers the danger zone's return path. The value
// arrives in a hidden form field, so it is attacker-supplied in principle: a
// delete that honoured it blindly would be an open redirect off the back of a
// destructive POST.
func TestHandleDelete_ReturnPath(t *testing.T) {
	tests := []struct {
		name   string
		form   string
		wantTo string
	}{
		{"local path is honoured", "return=/daily?date=2026-07-01", "/daily?date=2026-07-01"},
		{"absent falls back to root", "", "/"},
		{"empty falls back to root", "return=", "/"},
		{"protocol-relative host is rejected", "return=" + url.QueryEscape("//evil.example"), "/"},
		{"absolute URL with a scheme is rejected", "return=" + url.QueryEscape("https://evil.example/x"), "/"},
		{"backslash-folded host is rejected", "return=" + url.QueryEscape(`/\evil.example`), "/"},
		{"bare relative path is rejected", "return=" + url.QueryEscape("evil.example"), "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			id, err := repo.CreateExpense(t.Context(), domain.Expense{
				Amount: 5, Category: "food", Description: "x", Date: "2026-07-01", Currency: "USD",
			})
			if err != nil {
				t.Fatalf("seed expense: %v", err)
			}

			svc := service.New(repository.Repos{
				Expenses:    repo,
				Users:       repo,
				Preferences: repo,
				Rates:       repo,
			}, &mockRateClient{})
			h := NewHTMLHandler(svc, nil)

			req := httptest.NewRequest("POST", "/expenses/delete/"+strconv.FormatInt(id, 10), strings.NewReader(tt.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetPathValue("id", strconv.FormatInt(id, 10))

			rec := httptest.NewRecorder()
			h.HandleDelete(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("expected 303, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tt.wantTo {
				t.Fatalf("redirected to %q, want %q", got, tt.wantTo)
			}
			// The delete itself must happen either way — a rejected return
			// path redirects home, it does not veto the destructive action.
			if _, err := repo.GetExpense(t.Context(), id); err == nil {
				t.Fatal("expense survived the delete")
			}
		})
	}
}
