package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"expensif/internal/domain"
	"expensif/internal/repository"
)

// ruleStore is an in-memory RecurringRuleRepository, so the service's rule API
// can be driven without SQL. The guarantees SQL owns — the migration and the
// (rule, date) uniqueness constraint — are proven in the repository tests
// instead; a store like this cannot demonstrate either.
type ruleStore struct {
	rules  map[int64]domain.RecurringRule
	nextID int64
}

func newRuleStore() *ruleStore {
	return &ruleStore{rules: make(map[int64]domain.RecurringRule), nextID: 1}
}

func (s *ruleStore) CreateRule(_ context.Context, r domain.RecurringRule) (int64, error) {
	r.ID = s.nextID
	s.nextID++
	s.rules[r.ID] = r
	return r.ID, nil
}

func (s *ruleStore) GetRule(_ context.Context, id int64) (*domain.RecurringRule, error) {
	r, ok := s.rules[id]
	if !ok {
		return nil, fmt.Errorf("no recurring rule with id %d", id)
	}
	return &r, nil
}

func (s *ruleStore) ListRules(_ context.Context) ([]domain.RecurringRule, error) {
	var out []domain.RecurringRule
	for id := int64(1); id < s.nextID; id++ {
		if r, ok := s.rules[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *ruleStore) UpdateRule(_ context.Context, r domain.RecurringRule) error {
	if _, ok := s.rules[r.ID]; !ok {
		return fmt.Errorf("no recurring rule with id %d", r.ID)
	}
	s.rules[r.ID] = r
	return nil
}

func (s *ruleStore) DeleteRule(_ context.Context, id int64) error {
	if _, ok := s.rules[id]; !ok {
		return fmt.Errorf("no recurring rule with id %d", id)
	}
	delete(s.rules, id)
	return nil
}

// prefsStub serves fixed Preferences, so the defaults a rule inherits are the
// test's choice rather than the machine's.
type prefsStub struct {
	prefs domain.Preferences
	err   error
}

func (p *prefsStub) GetPreferences(_ context.Context) (*domain.Preferences, error) {
	if p.err != nil {
		return nil, p.err
	}
	cp := p.prefs
	return &cp, nil
}

func (p *prefsStub) SavePreferences(_ context.Context, prefs domain.Preferences) error {
	p.prefs = prefs
	return nil
}

var _ repository.RecurringRuleRepository = (*ruleStore)(nil)
var _ repository.PreferenceRepository = (*prefsStub)(nil)

// monthly builds a monthly rule anchored on day, valid enough to schedule.
func monthly(day int) domain.RecurringRule {
	return domain.RecurringRule{
		IntervalUnit:  domain.IntervalMonths,
		IntervalCount: 1,
		AnchorDay:     day,
	}
}

// TestNextChargeDate_AnchorSeriesNeverDrifts is the assertion this whole effort
// rests on. A rule anchored on the 31st clamps to Feb 28 — and then must return
// to Mar 31, because the next date is computed from the stored anchor and not by
// adding a month to the clamped one. An implementation that accumulates returns
// Mar 28 here and stays plausible forever after, drifting a few days earlier
// every year until it no longer matches the statement being reconciled against.
//
// Each result is deliberately fed back in as the next input: that is exactly how
// generation advances a rule, so it is the path the trap is on.
func TestNextChargeDate_AnchorSeriesNeverDrifts(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	rule := monthly(31)

	date := "2026-01-31"
	for _, want := range []string{"2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"} {
		got, err := svc.NextChargeDate(rule, date)
		if err != nil {
			t.Fatalf("NextChargeDate(anchor 31, %s): %v", date, err)
		}
		if got != want {
			t.Fatalf("from %s: got %s, want %s", date, got, want)
		}
		date = got
	}
}

func TestNextChargeDate(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})

	tests := []struct {
		name string
		rule domain.RecurringRule
		from string
		want string
	}{
		{"anchor 31 clamps into a short month", monthly(31), "2026-01-31", "2026-02-28"},
		{"anchor 31 clamps into a leap February", monthly(31), "2028-01-31", "2028-02-29"},
		{"anchor 31 recovers out of a leap February", monthly(31), "2028-02-29", "2028-03-31"},
		{"anchor 30 clamps into February", monthly(30), "2026-01-30", "2026-02-28"},
		{"anchor 30 recovers out of February", monthly(30), "2026-02-28", "2026-03-30"},
		{"anchor 29 lands on a leap February", monthly(29), "2028-01-29", "2028-02-29"},
		{"mid-month anchor never clamps", monthly(15), "2026-01-15", "2026-02-15"},
		{"mid-month anchor across the year boundary", monthly(15), "2026-12-15", "2027-01-15"},
		{
			"quarterly keeps its anchor across a clamp",
			domain.RecurringRule{IntervalUnit: domain.IntervalMonths, IntervalCount: 3, AnchorDay: 31},
			"2026-01-31", "2026-04-30",
		},
		{
			"quarterly recovers the anchor after clamping",
			domain.RecurringRule{IntervalUnit: domain.IntervalMonths, IntervalCount: 3, AnchorDay: 31},
			"2026-04-30", "2026-07-31",
		},
		{
			"quarterly across the year boundary",
			domain.RecurringRule{IntervalUnit: domain.IntervalMonths, IntervalCount: 3, AnchorDay: 31},
			"2026-10-31", "2027-01-31",
		},
		{
			"fortnightly crosses a month boundary",
			domain.RecurringRule{IntervalUnit: domain.IntervalWeeks, IntervalCount: 2},
			"2026-01-25", "2026-02-08",
		},
		{
			"weekly crosses a leap February",
			domain.RecurringRule{IntervalUnit: domain.IntervalWeeks, IntervalCount: 1},
			"2028-02-26", "2028-03-04",
		},
		{
			"ten-day interval crosses a month boundary",
			domain.RecurringRule{IntervalUnit: domain.IntervalDays, IntervalCount: 10},
			"2026-01-25", "2026-02-04",
		},
		{
			"daily crosses the year boundary",
			domain.RecurringRule{IntervalUnit: domain.IntervalDays, IntervalCount: 1},
			"2026-12-31", "2027-01-01",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.NextChargeDate(tc.rule, tc.from)
			if err != nil {
				t.Fatalf("NextChargeDate: %v", err)
			}
			if got != tc.want {
				t.Errorf("from %s: got %s, want %s", tc.from, got, tc.want)
			}
		})
	}
}

// TestNextChargeDate_MonthlyIgnoresTheDayItAdvancesFrom pins the mechanism
// directly: whatever day of the month you advance from, a monthly rule lands on
// its anchor. The clamped date carries no memory.
func TestNextChargeDate_MonthlyIgnoresTheDayItAdvancesFrom(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	for _, from := range []string{"2026-01-01", "2026-01-15", "2026-01-28", "2026-01-31"} {
		got, err := svc.NextChargeDate(monthly(20), from)
		if err != nil {
			t.Fatalf("NextChargeDate(%s): %v", from, err)
		}
		if got != "2026-02-20" {
			t.Errorf("from %s: got %s, want 2026-02-20", from, got)
		}
	}
}

func TestNextChargeDate_Invalid(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})

	tests := []struct {
		name string
		rule domain.RecurringRule
		from string
		want error
	}{
		{"unparseable date", monthly(15), "not-a-date", ErrInvalidDate},
		{"empty date", monthly(15), "", ErrInvalidDate},
		{
			"unknown interval unit",
			domain.RecurringRule{IntervalUnit: "fortnights", IntervalCount: 1, AnchorDay: 1},
			"2026-01-01", ErrInvalidIntervalUnit,
		},
		{
			"interval count below one",
			domain.RecurringRule{IntervalUnit: domain.IntervalMonths, IntervalCount: 0, AnchorDay: 1},
			"2026-01-01", ErrInvalidIntervalCount,
		},
		{"anchor day out of range", monthly(32), "2026-01-01", ErrInvalidAnchorDay},
		{"missing anchor day on a monthly rule", monthly(0), "2026-01-01", ErrInvalidAnchorDay},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.NextChargeDate(tc.rule, tc.from); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// --- Rule CRUD through the service ---

func newRuleService(t *testing.T, prefs domain.Preferences) (*Service, *ruleStore) {
	t.Helper()
	store := newRuleStore()
	svc := New(repository.Repos{
		Rules:       store,
		Preferences: &prefsStub{prefs: prefs},
	}, &mockRateClient{})
	return svc, store
}

func validRule() domain.RecurringRule {
	return domain.RecurringRule{
		Name:           "Netflix",
		Amount:         15.99,
		Currency:       "USD",
		Category:       "subscriptions",
		IntervalUnit:   domain.IntervalMonths,
		IntervalCount:  1,
		AnchorDay:      15,
		NextChargeDate: "2026-08-15",
		Enabled:        true,
	}
}

func TestCreateRule_RoundTripsThroughGetAndList(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	ctx := context.Background()

	id, err := svc.CreateRule(ctx, validRule())
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	got, err := svc.GetRule(ctx, id)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	want := validRule()
	want.ID = id
	if *got != want {
		t.Errorf("GetRule returned %+v, want %+v", *got, want)
	}

	rules, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != id {
		t.Fatalf("ListRules returned %+v, want the one rule with id %d", rules, id)
	}
}

func TestCreateRule_DefaultsCurrencyFromPreferences(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "MYR"})
	ctx := context.Background()

	r := validRule()
	r.Currency = ""
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, err := svc.GetRule(ctx, id)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.Currency != "MYR" {
		t.Errorf("currency = %q, want the Preferred Currency MYR", got.Currency)
	}
}

// TestCreateRule_KeepsAnExplicitPayer guards the optional payer: a rule states
// its own payer, and one left unset stays unset rather than inheriting the
// default payer behind the caller's back.
func TestCreateRule_KeepsAnExplicitPayer(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD", UserID: 7})
	ctx := context.Background()

	withPayer := validRule()
	withPayer.PaidByID = 3
	id, err := svc.CreateRule(ctx, withPayer)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, _ := svc.GetRule(ctx, id)
	if got.PaidByID != 3 {
		t.Errorf("PaidByID = %d, want 3", got.PaidByID)
	}

	noPayer := validRule()
	id, err = svc.CreateRule(ctx, noPayer)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, _ = svc.GetRule(ctx, id)
	if got.PaidByID != 0 {
		t.Errorf("PaidByID = %d, want 0 (unset stays unset)", got.PaidByID)
	}
}

// TestCreateRule_DefaultsAnchorDayFromTheStartDate lets a monthly rule be
// created from a start date alone — the anchor is the day that start date falls
// on, which is what a form asking only "when does it first charge?" implies.
func TestCreateRule_DefaultsAnchorDayFromTheStartDate(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	ctx := context.Background()

	r := validRule()
	r.AnchorDay = 0
	r.NextChargeDate = "2026-08-31"
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, _ := svc.GetRule(ctx, id)
	if got.AnchorDay != 31 {
		t.Errorf("AnchorDay = %d, want 31 (the start date's day)", got.AnchorDay)
	}
}

func TestCreateRule_TrimsNameAndCategory(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	ctx := context.Background()

	r := validRule()
	r.Name = "  Netflix  "
	r.Category = "  subscriptions  "
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, _ := svc.GetRule(ctx, id)
	if got.Name != "Netflix" || got.Category != "subscriptions" {
		t.Errorf("got name %q category %q, want them trimmed", got.Name, got.Category)
	}
}

func TestCreateRule_Invalid(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		mut  func(*domain.RecurringRule)
		want error
	}{
		{"zero amount", func(r *domain.RecurringRule) { r.Amount = 0 }, ErrInvalidAmount},
		{"negative amount", func(r *domain.RecurringRule) { r.Amount = -1 }, ErrInvalidAmount},
		{"blank name", func(r *domain.RecurringRule) { r.Name = "   " }, ErrMissingName},
		{"blank category", func(r *domain.RecurringRule) { r.Category = "" }, ErrMissingCategory},
		{"interval count of zero", func(r *domain.RecurringRule) { r.IntervalCount = 0 }, ErrInvalidIntervalCount},
		{"unknown interval unit", func(r *domain.RecurringRule) { r.IntervalUnit = "years" }, ErrInvalidIntervalUnit},
		{"anchor day out of range", func(r *domain.RecurringRule) { r.AnchorDay = 32 }, ErrInvalidAnchorDay},
		{"missing start date", func(r *domain.RecurringRule) { r.NextChargeDate = "" }, ErrInvalidDate},
		{"unparseable start date", func(r *domain.RecurringRule) { r.NextChargeDate = "15/08/2026" }, ErrInvalidDate},
		{"unparseable end date", func(r *domain.RecurringRule) { r.EndDate = "soon" }, ErrInvalidDate},
		{"end date before the first charge", func(r *domain.RecurringRule) { r.EndDate = "2026-07-01" }, ErrInvalidRange},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newRuleService(t, domain.Preferences{Currency: "USD"})
			r := validRule()
			tc.mut(&r)
			if _, err := svc.CreateRule(ctx, r); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if len(store.rules) != 0 {
				t.Errorf("a rejected rule was stored anyway: %+v", store.rules)
			}
		})
	}
}

// TestCreateRule_EndDateOnTheFirstCharge allows the degenerate one-charge rule:
// an end date equal to the first charge date means that charge still happens.
func TestCreateRule_EndDateOnTheFirstCharge(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	r := validRule()
	r.EndDate = r.NextChargeDate
	if _, err := svc.CreateRule(context.Background(), r); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
}

func TestUpdateRule_ValidatesAndPersists(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	ctx := context.Background()

	id, err := svc.CreateRule(ctx, validRule())
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	edited := validRule()
	edited.ID = id
	edited.Amount = 19.99
	edited.Name = "Netflix Premium"
	if err := svc.UpdateRule(ctx, edited); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	got, _ := svc.GetRule(ctx, id)
	if got.Amount != 19.99 || got.Name != "Netflix Premium" {
		t.Errorf("got %+v, want the edited amount and name", *got)
	}

	invalid := edited
	invalid.Amount = 0
	if err := svc.UpdateRule(ctx, invalid); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("UpdateRule with a zero amount: got %v, want %v", err, ErrInvalidAmount)
	}
	got, _ = svc.GetRule(ctx, id)
	if got.Amount != 19.99 {
		t.Errorf("a rejected edit changed the stored rule: %+v", *got)
	}
}

func TestDeleteRule(t *testing.T) {
	svc, _ := newRuleService(t, domain.Preferences{Currency: "USD"})
	ctx := context.Background()

	id, err := svc.CreateRule(ctx, validRule())
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if err := svc.DeleteRule(ctx, id); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if _, err := svc.GetRule(ctx, id); err == nil {
		t.Error("GetRule succeeded after the rule was deleted")
	}
	rules, _ := svc.ListRules(ctx)
	if len(rules) != 0 {
		t.Errorf("ListRules returned %+v after the only rule was deleted", rules)
	}
}

// TestCreateRule_FallsBackToUSDWithoutPreferences keeps rule creation working on
// a store that has never had preferences saved, matching CreateExpense's
// long-standing fallback.
func TestCreateRule_FallsBackToUSDWithoutPreferences(t *testing.T) {
	store := newRuleStore()
	svc := New(repository.Repos{
		Rules:       store,
		Preferences: &prefsStub{err: sql.ErrNoRows},
	}, &mockRateClient{})

	r := validRule()
	r.Currency = ""
	id, err := svc.CreateRule(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if got := store.rules[id].Currency; got != "USD" {
		t.Errorf("currency = %q, want USD", got)
	}
}
