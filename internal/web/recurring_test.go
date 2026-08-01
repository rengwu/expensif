package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
	"expensif/internal/service"
)

// recurringToday is the date every Recurring page test runs on. Rule editing,
// pausing and deleting all turn on where "today" falls, so the clock is pinned
// rather than borrowed from the machine.
const recurringToday = "2026-08-02"

// recurringFixture wires the app's real router over an in-memory repository,
// with the handler's clock frozen at recurringToday. Requests go through the
// mux production runs, so path values and method matching are exercised too.
func recurringFixture(t *testing.T) (http.Handler, *mockRepo) {
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

	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	h := NewHTMLHandler(svc, renderer)
	h.now = func() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }
	return routes(NewAPIHandler(svc), h), repo
}

// getPage fetches a whole page through the router, reusing the ledger tests'
// getBody — a router's ServeHTTP is an http.HandlerFunc like any other.
func getPage(t *testing.T, router http.Handler, target string) string {
	t.Helper()
	return getBody(t, router.ServeHTTP, target)
}

func postForm(t *testing.T, router http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ruleForm is a complete, valid submission of the rule form.
func ruleForm() url.Values {
	return url.Values{
		"name":             {"Netflix"},
		"amount":           {"15.99"},
		"currency":         {"USD"},
		"category":         {"subscriptions"},
		"paid_by_id":       {"0"},
		"interval_unit":    {"months"},
		"interval_count":   {"1"},
		"next_charge_date": {"2026-08-31"},
		"end_date":         {""},
		"enabled":          {"on"},
	}
}

// storedRule reads back the single rule the repository holds.
func storedRule(t *testing.T, repo *mockRepo) domain.RecurringRule {
	t.Helper()
	rules, err := repo.ListRules(t.Context())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected exactly 1 rule, got %d", len(rules))
	}
	return rules[0]
}

func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("redirected to %q, want %q", got, want)
	}
}

// TestRecurringPage_CreateEditPauseResumeDelete is the ticket's first Done-when
// clause end to end: every rule operation driven only through the page's own
// forms, with the store read back after each one.
func TestRecurringPage_CreateEditPauseResumeDelete(t *testing.T) {
	router, repo := recurringFixture(t)

	// Create.
	assertRedirect(t, postForm(t, router, "/recurring/new", ruleForm()), "/recurring?flash=created")
	rule := storedRule(t, repo)
	if rule.Name != "Netflix" || rule.Amount != 15.99 || rule.Category != "subscriptions" {
		t.Fatalf("created rule is %+v", rule)
	}
	// The form asks only when it first charges; the Anchor Day comes from that
	// date, which is what keeps a rule started on the 31st on the 31st.
	if rule.AnchorDay != 31 || rule.NextChargeDate != "2026-08-31" || !rule.Enabled {
		t.Fatalf("created rule scheduled as %+v", rule)
	}

	// The edit form carries the stored Anchor Day back, so saving an unrelated
	// change cannot silently re-anchor the rule to a clamped charge date.
	form := getPage(t, router, "/recurring/edit/1")
	if !strings.Contains(form, `name="anchor_day" value="31"`) {
		t.Errorf("the edit form does not carry the Anchor Day: %s", form)
	}

	// Edit: a price rise, on the same schedule.
	edit := ruleForm()
	edit.Set("amount", "18.99")
	edit.Set("anchor_day", "31")
	assertRedirect(t, postForm(t, router, "/recurring/edit/1", edit), "/recurring?flash=updated")
	if got := storedRule(t, repo); got.Amount != 18.99 {
		t.Fatalf("edited amount is %v, want 18.99", got.Amount)
	}

	// Pause, then resume.
	assertRedirect(t, postForm(t, router, "/recurring/toggle/1", url.Values{"enabled": {"false"}}), "/recurring?flash=paused")
	if storedRule(t, repo).Enabled {
		t.Fatal("rule is still enabled after pausing through the page")
	}
	assertRedirect(t, postForm(t, router, "/recurring/toggle/1", url.Values{"enabled": {"true"}}), "/recurring?flash=resumed")
	if !storedRule(t, repo).Enabled {
		t.Fatal("rule is still paused after resuming through the page")
	}

	// Delete.
	assertRedirect(t, postForm(t, router, "/recurring/delete/1", nil), "/recurring?flash=deleted")
	rules, err := repo.ListRules(t.Context())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("rule survived the delete: %+v", rules)
	}
}

// TestRecurringForm_KeepsSubmittedValuesOnAValidationError follows the expense
// form's convention: a rejected save re-renders the form with the error and
// everything typed, rather than dropping the human back to an empty one.
func TestRecurringForm_KeepsSubmittedValuesOnAValidationError(t *testing.T) {
	router, repo := recurringFixture(t)

	form := ruleForm()
	form.Set("name", "Spotify Family")
	form.Set("amount", "0") // rejected by the service

	rec := postForm(t, router, "/recurring/new", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected the form to re-render (200), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, service.ErrInvalidAmount.Error()) {
		t.Errorf("the error was not shown: %s", body)
	}
	if !strings.Contains(body, "Spotify Family") {
		t.Error("the submitted name was lost from the re-rendered form")
	}
	if rules, _ := repo.ListRules(t.Context()); len(rules) != 0 {
		t.Fatalf("a rejected submission created a rule: %+v", rules)
	}
}

// TestRecurringPage_ListsRulesSoonestFirstWithPausedLast covers the page's
// ordering and the one thing a paused rule must not claim: its stored date is a
// stopped clock, so it is never advertised as a charge that is coming.
func TestRecurringPage_ListsRulesSoonestFirstWithPausedLast(t *testing.T) {
	router, repo := recurringFixture(t)
	ctx := t.Context()

	// Seeded out of order, and the paused rule holds the soonest date of all —
	// so ordering by date alone would put it first.
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Gym", Amount: 60, Currency: "USD", Category: "health",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 1,
		NextChargeDate: "2026-08-03", Enabled: false,
	})
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Rent", Amount: 1200, Currency: "USD", Category: "housing",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 25,
		NextChargeDate: "2026-08-25", Enabled: true,
	})
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Netflix", Amount: 15.99, Currency: "USD", Category: "subscriptions",
		IntervalUnit: domain.IntervalWeeks, IntervalCount: 2,
		NextChargeDate: "2026-08-10", Enabled: true,
	})

	body := getPage(t, router, "/recurring")

	netflix, rent, gym := strings.Index(body, "Netflix"), strings.Index(body, "Rent"), strings.Index(body, "Gym")
	if netflix < 0 || rent < 0 || gym < 0 {
		t.Fatalf("a rule the page was given never rendered (netflix=%d rent=%d gym=%d)", netflix, rent, gym)
	}
	if !(netflix < rent && rent < gym) {
		t.Errorf("rules are ordered netflix=%d rent=%d gym=%d; want soonest charge first with the paused one last", netflix, rent, gym)
	}
	if !strings.Contains(body, "Every 2 weeks") {
		t.Error("a fortnightly rule's interval never rendered")
	}
	if !strings.Contains(body, "Paused") {
		t.Error("the paused rule is not marked as paused")
	}
	// Its stored 2026-08-03 could have overrun an end date; the page must not
	// present it as a charge that is going to happen.
	if strings.Contains(body, "Aug 3") {
		t.Error("the paused rule advertised a next charge date")
	}
}

// TestRecurringPage_MonthlyEquivalentAcrossIntervalsAndCurrencies asserts the
// figure itself: intervals normalized to a month, converted into the Preferred
// Currency, summed, and the paused rule left out of it.
func TestRecurringPage_MonthlyEquivalentAcrossIntervalsAndCurrencies(t *testing.T) {
	router, repo := recurringFixture(t)
	ctx := t.Context()
	if err := repo.SaveRates(ctx, "USD", "2026-08-01", map[string]float64{"USD": 1, "EUR": 0.5}); err != nil {
		t.Fatalf("seed rates: %v", err)
	}

	// USD 120 quarterly = 40.00 a month.
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Insurance", Amount: 120, Currency: "USD", Category: "insurance",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 3, AnchorDay: 1,
		NextChargeDate: "2026-09-01", Enabled: true,
	})
	// EUR 50 a fortnight = 50 * 365.25/7/12/2 EUR a month, doubled into USD.
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Cleaner", Amount: 50, Currency: "EUR", Category: "home",
		IntervalUnit: domain.IntervalWeeks, IntervalCount: 2,
		NextChargeDate: "2026-08-10", Enabled: true,
	})
	// Paused, and expensive: it must not reach the figure.
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Storage", Amount: 999, Currency: "USD", Category: "home",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 5,
		NextChargeDate: "2026-08-05", Enabled: false,
	})

	body := getPage(t, router, "/recurring")

	want := "≈ $257.41 / month"
	if !strings.Contains(body, want) {
		t.Errorf("the Monthly Equivalent does not read %q: %s", want, body)
	}
}

// TestRecurringPage_MonthlyEquivalentHidesWhenARuleCannotBeConverted is the
// hidden-not-wrong clause. One unconvertible rule hides the whole figure: a
// partial total is a wrong number with nothing on screen to say so.
func TestRecurringPage_MonthlyEquivalentHidesWhenARuleCannotBeConverted(t *testing.T) {
	router, repo := recurringFixture(t)
	ctx := t.Context()

	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Rent", Amount: 1200, Currency: "USD", Category: "housing",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 25,
		NextChargeDate: "2026-08-25", Enabled: true,
	})
	repo.CreateRule(ctx, domain.RecurringRule{
		Name: "Cleaner", Amount: 50, Currency: "EUR", Category: "home",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 10,
		NextChargeDate: "2026-08-10", Enabled: true,
	})

	body := getPage(t, router, "/recurring") // no rates stored at all
	if strings.Contains(body, "/ month") {
		t.Errorf("a figure was shown with a rule that could not be converted: %s", body)
	}
	if !strings.Contains(body, "Monthly equivalent unavailable") {
		t.Error("the page does not say the figure is unavailable")
	}
	// The rules themselves still list — only the figure is withheld.
	if !strings.Contains(body, "Rent") || !strings.Contains(body, "Cleaner") {
		t.Error("hiding the figure also hid the rules")
	}
}

// TestRecurringPage_MonthlyEquivalentSurvivesMissingRates: rates being absent
// is only fatal to a figure that needed them. An account billed entirely in its
// Preferred Currency always gets one.
func TestRecurringPage_MonthlyEquivalentSurvivesMissingRates(t *testing.T) {
	router, repo := recurringFixture(t)

	repo.CreateRule(t.Context(), domain.RecurringRule{
		Name: "Rent", Amount: 1200, Currency: "USD", Category: "housing",
		IntervalUnit: domain.IntervalMonths, IntervalCount: 1, AnchorDay: 25,
		NextChargeDate: "2026-08-25", Enabled: true,
	})

	body := getPage(t, router, "/recurring")
	if !strings.Contains(body, "≈ $1200.00 / month") {
		t.Errorf("no figure for an account that needed no conversion: %s", body)
	}
}

// TestRecurringPage_ReachableFromTheNavigation checks the ticket's navigation
// clause from a different page — the entry lives in the shared layout, so the
// Ledger carrying it is the general claim.
func TestRecurringPage_ReachableFromTheNavigation(t *testing.T) {
	router, _ := recurringFixture(t)
	if !strings.Contains(getPage(t, router, "/"), `href="/recurring"`) {
		t.Error("no navigation entry leads to the Recurring page")
	}
}

// TestRecurringEdit_UnknownRuleIs404 matches the user pages' precedent for an
// id that names nothing.
func TestRecurringEdit_UnknownRuleIs404(t *testing.T) {
	router, _ := recurringFixture(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/recurring/edit/404", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestRecurringDelete_FailureLandsOnTheListWithTheError: deleting has no form
// of its own, so its failure has to appear on the list rather than as a bare
// error page.
func TestRecurringDelete_FailureLandsOnTheListWithTheError(t *testing.T) {
	router, _ := recurringFixture(t)
	rec := postForm(t, router, "/recurring/delete/404", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected the list to re-render (200), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no recurring rule with id 404") {
		t.Errorf("the failure was not reported on the page: %s", rec.Body.String())
	}
}
