package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
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

// --- Generation ---

// chargeStore is an in-memory ExpenseRepository for the generation tests. It
// enforces the one guarantee generation leans on — a rule cannot mint two
// charges on one date — because in production that is the (rule, date) unique
// index doing it, and a double that quietly allowed duplicates would make every
// idempotence assertion below meaningless. The rest of the expense surface,
// which generation never touches, comes from the embedded stub.
type chargeStore struct {
	expenseRepoStub
	expenses []domain.Expense
	nextID   int64
}

func newChargeStore() *chargeStore { return &chargeStore{nextID: 1} }

func (s *chargeStore) MintCharge(_ context.Context, e domain.Expense) (bool, error) {
	for _, existing := range s.expenses {
		if existing.RecurringRuleID == e.RecurringRuleID && existing.Date == e.Date {
			return false, nil
		}
	}
	e.ID = s.nextID
	s.nextID++
	s.expenses = append(s.expenses, e)
	return true, nil
}

func (s *chargeStore) ListExpensesInRange(_ context.Context, start, end string) ([]domain.Expense, error) {
	var out []domain.Expense
	for _, e := range s.expenses {
		if e.Date >= start && e.Date <= end {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date }) // newest first, as the SQL orders
	return out, nil
}

var _ repository.ExpenseRepository = (*chargeStore)(nil)

// newGeneratingService wires a service over both in-memory stores, the pair
// generation reads and writes.
func newGeneratingService(t *testing.T) (*Service, *ruleStore, *chargeStore) {
	t.Helper()
	rules, charges := newRuleStore(), newChargeStore()
	svc := New(repository.Repos{
		Rules:       rules,
		Expenses:    charges,
		Preferences: &prefsStub{prefs: domain.Preferences{Currency: "USD"}},
	}, &mockRateClient{})
	return svc, rules, charges
}

// chargeDates reports the dates of every expense that exists, oldest first —
// "which expenses exist after generation runs", read back through the service.
func chargeDates(t *testing.T, svc *Service) []string {
	t.Helper()
	expenses, err := svc.ExpensesInRange(context.Background(), "0001-01-01", "9999-12-31")
	if err != nil {
		t.Fatalf("ExpensesInRange: %v", err)
	}
	var dates []string
	for _, e := range expenses {
		dates = append(dates, e.Date)
	}
	sort.Strings(dates)
	return dates
}

func assertDates(t *testing.T, svc *Service, want ...string) {
	t.Helper()
	got := chargeDates(t, svc)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("charges exist on %v, want %v", got, want)
	}
}

// generatingRule is validRule() with its first charge on 2026-08-15, ready to be
// stored and generated from.
func generatingRule() domain.RecurringRule {
	return validRule()
}

// TestGenerateCharges_MaterializesOneChargeAhead covers the whole "a Pending
// Charge is an ordinary Expense" clause: one charge exists after the supplied
// date, it carries every field the rule states, and the rule's advertised next
// charge date is that charge's date.
func TestGenerateCharges_MaterializesOneChargeAhead(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	r := generatingRule()
	r.PaidByID = 4
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	minted, err := svc.GenerateCharges(ctx, "2026-07-31")
	if err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	if minted != 1 {
		t.Fatalf("minted %d charges, want the one ahead", minted)
	}

	expenses, err := svc.ExpensesInRange(ctx, "0001-01-01", "9999-12-31")
	if err != nil {
		t.Fatalf("ExpensesInRange: %v", err)
	}
	if len(expenses) != 1 {
		t.Fatalf("got %d expenses, want exactly one Pending Charge: %+v", len(expenses), expenses)
	}
	got := expenses[0]
	want := domain.Expense{
		ID:              got.ID,
		Amount:          15.99,
		Category:        "subscriptions",
		Description:     "Netflix", // the rule's Name is the charge's Description
		Date:            "2026-08-15",
		Currency:        "USD",
		PaidByID:        4,
		RecurringRuleID: id,
	}
	if got != want {
		t.Errorf("Pending Charge = %+v, want %+v", got, want)
	}

	rule, _ := svc.GetRule(ctx, id)
	if rule.NextChargeDate != "2026-08-15" {
		t.Errorf("rule's next charge date = %s, want the Pending Charge's date 2026-08-15", rule.NextChargeDate)
	}
	if !rule.Enabled {
		t.Error("rule disabled itself with no end date to reach")
	}
}

// TestGenerateCharges_IsIdempotentForTheSameDate is the property the background
// goroutine's whole safety rests on: it will call this repeatedly, at startup and
// on a ticker, and nothing may accumulate.
func TestGenerateCharges_IsIdempotentForTheSameDate(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	if _, err := svc.CreateRule(ctx, generatingRule()); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if _, err := svc.GenerateCharges(ctx, "2026-07-31"); err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}

	for run := 2; run <= 4; run++ {
		minted, err := svc.GenerateCharges(ctx, "2026-07-31")
		if err != nil {
			t.Fatalf("GenerateCharges run %d: %v", run, err)
		}
		if minted != 0 {
			t.Errorf("run %d minted %d charges, want 0 — the same date was already generated", run, minted)
		}
		assertDates(t, svc, "2026-08-15")
	}
}

// TestGenerateCharges_CatchesUpOnEveryMissedCharge is the spec's named case: a
// monthly rule last generated in January, run with an April date, backfills all
// four charges that came due on their own true dates and leaves exactly one
// pending.
func TestGenerateCharges_CatchesUpOnEveryMissedCharge(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	r := generatingRule()
	r.NextChargeDate = "2026-01-15"
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	minted, err := svc.GenerateCharges(ctx, "2026-04-20")
	if err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	// Four backfilled (Jan–Apr) plus the one now pending: every expense created.
	if minted != 5 {
		t.Errorf("minted %d charges, want 5 (four backfilled and one pending)", minted)
	}
	assertDates(t, svc, "2026-01-15", "2026-02-15", "2026-03-15", "2026-04-15", "2026-05-15")

	expenses, _ := svc.ExpensesInRange(ctx, "2026-04-21", "9999-12-31")
	if len(expenses) != 1 {
		t.Errorf("got %d charges after the supplied date, want exactly one pending: %+v", len(expenses), expenses)
	}
	rule, _ := svc.GetRule(ctx, id)
	if rule.NextChargeDate != "2026-05-15" {
		t.Errorf("rule's next charge date = %s, want it advanced past every backfilled charge to 2026-05-15", rule.NextChargeDate)
	}

	// And catching up is itself repeatable.
	minted, err = svc.GenerateCharges(ctx, "2026-04-20")
	if err != nil {
		t.Fatalf("second GenerateCharges: %v", err)
	}
	if minted != 0 {
		t.Errorf("re-running catch-up minted %d charges, want 0", minted)
	}
}

// TestGenerateCharges_CatchUpKeepsTheAnchor is the anchor canary on the path
// generation actually walks. A rule anchored on the 31st caught up from January
// must land on Mar 31, not the Mar 28 an implementation that accumulates onto the
// clamped February date would produce — and the drift, once started, never stops.
func TestGenerateCharges_CatchUpKeepsTheAnchor(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	r := generatingRule()
	r.AnchorDay = 31
	r.NextChargeDate = "2026-01-31"
	if _, err := svc.CreateRule(ctx, r); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	if _, err := svc.GenerateCharges(ctx, "2026-04-20"); err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	assertDates(t, svc, "2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30")
}

// TestGenerateCharges_EndDateBoundary pins the boundary exactly: the charge
// falling on the end date is generated, the one after it is not, and the rule
// comes back disabled rather than needing anyone to switch it off.
func TestGenerateCharges_EndDateBoundary(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	r := generatingRule()
	r.NextChargeDate = "2026-07-15"
	r.EndDate = "2026-08-15"
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	if _, err := svc.GenerateCharges(ctx, "2026-07-20"); err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	assertDates(t, svc, "2026-07-15", "2026-08-15") // the final charge lands on the end date
	rule, _ := svc.GetRule(ctx, id)
	if !rule.Enabled {
		t.Fatal("the rule disabled itself while it still owed the charge on its end date")
	}

	// Once that last charge is behind us, the next one would fall past the end
	// date, so the rule switches itself off instead of scheduling it.
	minted, err := svc.GenerateCharges(ctx, "2026-08-20")
	if err != nil {
		t.Fatalf("GenerateCharges past the end date: %v", err)
	}
	if minted != 0 {
		t.Errorf("minted %d charges past the end date, want 0", minted)
	}
	assertDates(t, svc, "2026-07-15", "2026-08-15")
	rule, _ = svc.GetRule(ctx, id)
	if rule.Enabled {
		t.Error("the rule stayed enabled with its end date behind it")
	}

	// A finished rule stays finished, however often generation runs.
	if _, err := svc.GenerateCharges(ctx, "2027-01-01"); err != nil {
		t.Fatalf("GenerateCharges long past the end date: %v", err)
	}
	assertDates(t, svc, "2026-07-15", "2026-08-15")
}

// TestGenerateCharges_DisabledRuleGeneratesNothing — the clock stops. No pending
// charge, no catch-up for the months it sat disabled, not even a date advance,
// which is what keeps re-enabling from dumping a paused stretch into the ledger.
func TestGenerateCharges_DisabledRuleGeneratesNothing(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	r := generatingRule()
	r.NextChargeDate = "2026-01-15"
	r.Enabled = false
	id, err := svc.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	minted, err := svc.GenerateCharges(ctx, "2026-04-20")
	if err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	if minted != 0 {
		t.Errorf("a disabled rule minted %d charges, want 0", minted)
	}
	assertDates(t, svc)

	rule, _ := svc.GetRule(ctx, id)
	if rule.NextChargeDate != "2026-01-15" {
		t.Errorf("a disabled rule's next charge date moved to %s, want it left at 2026-01-15", rule.NextChargeDate)
	}
	if rule.Enabled {
		t.Error("generation re-enabled a disabled rule")
	}
}

// TestGenerateCharges_CountsEveryRulesCharges: the count the goroutine reports
// spans all the rules, and one rule's pause does not stop another's schedule.
func TestGenerateCharges_CountsEveryRulesCharges(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	ctx := context.Background()

	behind := generatingRule()
	behind.Name = "Rent"
	behind.NextChargeDate = "2026-06-15"
	if _, err := svc.CreateRule(ctx, behind); err != nil {
		t.Fatalf("CreateRule(Rent): %v", err)
	}
	ahead := generatingRule()
	ahead.Name = "Gym"
	if _, err := svc.CreateRule(ctx, ahead); err != nil {
		t.Fatalf("CreateRule(Gym): %v", err)
	}
	paused := generatingRule()
	paused.Name = "Spotify"
	paused.Enabled = false
	if _, err := svc.CreateRule(ctx, paused); err != nil {
		t.Fatalf("CreateRule(Spotify): %v", err)
	}

	minted, err := svc.GenerateCharges(ctx, "2026-07-31")
	if err != nil {
		t.Fatalf("GenerateCharges: %v", err)
	}
	// Rent: June and July behind us, August pending. Gym: August pending.
	if minted != 4 {
		t.Errorf("minted %d charges across the rules, want 4", minted)
	}
	assertDates(t, svc, "2026-06-15", "2026-07-15", "2026-08-15", "2026-08-15")
}

func TestGenerateCharges_InvalidDate(t *testing.T) {
	svc, _, _ := newGeneratingService(t)
	if _, err := svc.GenerateCharges(context.Background(), "today"); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("got %v, want %v", err, ErrInvalidDate)
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
