package service

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
)

// mockRateClient is a test double for RateFetcher.
type mockRateClient struct {
	rates map[string]float64
	date  string
	err   error

	// Timeseries doubles: tsCalls counts invocations (to prove caching skips the
	// fetch), tsErr forces a provider failure, and tsFunc supplies the returned
	// series for a requested [start, end].
	tsCalls int
	tsErr   error
	tsFunc  func(start, end string) map[string]map[string]float64
}

func (m *mockRateClient) Latest(ctx context.Context, base string) (map[string]float64, string, error) {
	return m.rates, m.date, m.err
}

func (m *mockRateClient) Timeseries(ctx context.Context, base, start, end string) (map[string]map[string]float64, error) {
	m.tsCalls++
	if m.tsErr != nil {
		return nil, m.tsErr
	}
	if m.tsFunc != nil {
		return m.tsFunc(start, end), nil
	}
	return map[string]map[string]float64{}, nil
}

// mockRateRepo implements only the RateRepository interface for testing.
type mockRateRepo struct {
	saved map[string]map[string]float64 // base -> target -> rate
	date  string
}

func (r *mockRateRepo) SaveRates(ctx context.Context, base string, date string, rates map[string]float64) error {
	if r.saved == nil {
		r.saved = make(map[string]map[string]float64)
	}
	r.saved[base] = make(map[string]float64, len(rates))
	for k, v := range rates {
		r.saved[base][k] = v
	}
	r.date = date
	return nil
}

func (r *mockRateRepo) GetRates(ctx context.Context, base string, date string) (map[string]float64, error) {
	return nil, errors.New("no rates for date")
}

func (r *mockRateRepo) GetLatestRates(ctx context.Context, base string) (map[string]float64, string, error) {
	if r.saved == nil || r.saved[base] == nil {
		return nil, "", errors.New("no rates")
	}
	cp := make(map[string]float64, len(r.saved[base]))
	for k, v := range r.saved[base] {
		cp[k] = v
	}
	return cp, r.date, nil
}

func (r *mockRateRepo) RatesOnOrBefore(ctx context.Context, base, date string) (map[string]float64, string, error) {
	return r.GetLatestRates(ctx, base)
}

func (r *mockRateRepo) RateDatesInRange(ctx context.Context, base, from, to string) ([]string, error) {
	if r.saved != nil && r.saved[base] != nil && r.date >= from && r.date <= to {
		return []string{r.date}, nil
	}
	return nil, nil
}

// Compile-time interface checks.
var _ RateFetcher = (*mockRateClient)(nil)
var _ repository.RateRepository = (*mockRateRepo)(nil)

// --- RefreshRates ---

func TestRefreshRates_SavesToRepository(t *testing.T) {
	mockRates := &mockRateClient{
		rates: map[string]float64{"EUR": 0.85, "JPY": 150.0},
		date:  "2024-06-01",
	}
	repo := &mockRateRepo{}

	svc := New(repository.Repos{Rates: repo}, mockRates)

	if err := svc.RefreshRates(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.date != "2024-06-01" {
		t.Fatalf("expected date 2024-06-01, got %s", repo.date)
	}
	if repo.saved["USD"]["EUR"] != 0.85 {
		t.Fatalf("expected EUR rate 0.85, got %v", repo.saved["USD"]["EUR"])
	}
	if repo.saved["USD"]["JPY"] != 150.0 {
		t.Fatalf("expected JPY rate 150.0, got %v", repo.saved["USD"]["JPY"])
	}
}

func TestRefreshRates_FetchError_NoSave(t *testing.T) {
	mockRates := &mockRateClient{
		rates: nil,
		date:  "",
		err:   errors.New("network timeout"),
	}
	repo := &mockRateRepo{}

	svc := New(repository.Repos{Rates: repo}, mockRates)

	err := svc.RefreshRates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if repo.saved != nil {
		t.Fatal("expected no save on fetch error")
	}
}

// --- EnsureRatesForRange ---

// dateRateStore is a date-aware in-memory RateRepository for the backfill tests:
// it keys rates by date as the real store does, so coverage probing and
// on-or-before resolution are exercised honestly rather than flattened.
type dateRateStore struct {
	byDate map[string]map[string]float64 // date -> target -> rate
}

func newDateRateStore() *dateRateStore {
	return &dateRateStore{byDate: map[string]map[string]float64{}}
}

func (s *dateRateStore) SaveRates(_ context.Context, base, date string, rates map[string]float64) error {
	cp := make(map[string]float64, len(rates))
	for k, v := range rates {
		cp[k] = v
	}
	s.byDate[date] = cp
	return nil
}

func (s *dateRateStore) GetRates(_ context.Context, base, date string) (map[string]float64, error) {
	if r, ok := s.byDate[date]; ok {
		return r, nil
	}
	return nil, sql.ErrNoRows
}

func (s *dateRateStore) GetLatestRates(_ context.Context, base string) (map[string]float64, string, error) {
	var latest string
	for d := range s.byDate {
		if d > latest {
			latest = d
		}
	}
	if latest == "" {
		return nil, "", sql.ErrNoRows
	}
	return s.byDate[latest], latest, nil
}

func (s *dateRateStore) RatesOnOrBefore(_ context.Context, base, date string) (map[string]float64, string, error) {
	var matched string
	for d := range s.byDate {
		if d <= date && d > matched {
			matched = d
		}
	}
	if matched == "" {
		return nil, "", sql.ErrNoRows
	}
	return s.byDate[matched], matched, nil
}

func (s *dateRateStore) RateDatesInRange(_ context.Context, base, from, to string) ([]string, error) {
	var dates []string
	for d := range s.byDate {
		if d >= from && d <= to {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	return dates, nil
}

var _ repository.RateRepository = (*dateRateStore)(nil)

// businessDayRates returns a USD-based series with one rate set per weekday in
// [start, end], mimicking Frankfurter's omission of weekends.
func businessDayRates(start, end string) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	s, _ := time.Parse("2006-01-02", start)
	e, _ := time.Parse("2006-01-02", end)
	for d := s; !d.After(e); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		out[d.Format("2006-01-02")] = map[string]float64{"USD": 1.0, "EUR": 0.9}
	}
	return out
}

func TestEnsureRatesForRange_FetchesOnceThenCaches(t *testing.T) {
	store := newDateRateStore()
	client := &mockRateClient{tsFunc: businessDayRates}
	svc := New(repository.Repos{Rates: store}, client)

	// Cold range (Mon–Fri): the store lacks every business day, so one fetch runs.
	if err := svc.EnsureRatesForRange(context.Background(), "2026-01-05", "2026-01-09"); err != nil {
		t.Fatalf("cold range: unexpected error: %v", err)
	}
	if client.tsCalls != 1 {
		t.Fatalf("cold range: expected exactly 1 Timeseries call, got %d", client.tsCalls)
	}

	// Second export over the same range: fully covered, so zero further calls.
	if err := svc.EnsureRatesForRange(context.Background(), "2026-01-05", "2026-01-09"); err != nil {
		t.Fatalf("cached range: unexpected error: %v", err)
	}
	if client.tsCalls != 1 {
		t.Fatalf("cached range: expected no further calls, got %d total", client.tsCalls)
	}
}

func TestEnsureRatesForRange_ProviderError(t *testing.T) {
	store := newDateRateStore()
	client := &mockRateClient{tsErr: errors.New("provider down")}
	svc := New(repository.Repos{Rates: store}, client)

	if err := svc.EnsureRatesForRange(context.Background(), "2026-01-05", "2026-01-09"); err == nil {
		t.Fatal("expected an error when Timeseries fails, got nil")
	}
}

// --- ConvertWithRates ---

func TestConvertWithRates_SameCurrency(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	result, err := svc.ConvertWithRates(100.0, "USD", "USD", map[string]float64{"USD": 1.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 100.0 {
		t.Fatalf("expected 100.0, got %f", result)
	}
}

func TestConvertWithRates_CrossCurrency(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	// Rates relative to USD: EUR=0.85, GBP=0.79
	rates := map[string]float64{"USD": 1.0, "EUR": 0.85, "GBP": 0.79}

	// 100 USD -> EUR = 100 * 0.85 / 1.0 = 85
	result, err := svc.ConvertWithRates(100.0, "USD", "EUR", rates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 85.0 {
		t.Fatalf("expected 85.0 USD->EUR, got %f", result)
	}

	// 100 GBP -> EUR = 100 * 0.85 / 0.79 = ~107.59
	result, err = svc.ConvertWithRates(100.0, "GBP", "EUR", rates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 100.0 * 0.85 / 0.79
	if result != expected {
		t.Fatalf("expected %f GBP->EUR, got %f", expected, result)
	}
}

func TestConvertWithRates_MissingRate(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	rates := map[string]float64{"USD": 1.0}

	_, err := svc.ConvertWithRates(100.0, "USD", "EUR", rates)
	if err == nil {
		t.Fatal("expected error for missing rate, got nil")
	}
}

// --- ConvertExpensesTotal ---

func TestConvertExpensesTotal_ConvertsAndSums(t *testing.T) {
	repo := &mockRateRepo{
		saved: map[string]map[string]float64{
			"USD": {"EUR": 0.85, "JPY": 150.0, "USD": 1.0},
		},
		date: "2024-06-01",
	}
	svc := New(repository.Repos{Rates: repo}, &mockRateClient{})

	expenses := []domain.Expense{
		{Amount: 100.0, Currency: "USD"},
		{Amount: 100.0, Currency: "EUR"}, // 100 EUR -> USD = 100 * 1.0 / 0.85 = ~117.65
		{Amount: 150.0, Currency: "JPY"}, // 150 JPY -> USD = 150 * 1.0 / 150.0 = 1.0
	}

	total, date, err := svc.ConvertExpensesTotal(context.Background(), expenses, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if date != "2024-06-01" {
		t.Fatalf("expected date 2024-06-01, got %s", date)
	}
	expected := 100.0 + (100.0 / 0.85) + 1.0
	if total != expected {
		t.Fatalf("expected total %f, got %f", expected, total)
	}
}

func TestConvertExpensesTotal_NoRates(t *testing.T) {
	repo := &mockRateRepo{}
	svc := New(repository.Repos{Rates: repo}, &mockRateClient{})

	_, _, err := svc.ConvertExpensesTotal(context.Background(), []domain.Expense{
		{Amount: 100.0, Currency: "USD"},
	}, "USD")
	if !errors.Is(err, ErrNoRates) {
		t.Fatalf("expected ErrNoRates, got %v", err)
	}
}

func TestConvertExpensesTotal_SkipsUnconvertible(t *testing.T) {
	repo := &mockRateRepo{
		saved: map[string]map[string]float64{
			"USD": {"USD": 1.0},
		},
		date: "2024-06-01",
	}
	svc := New(repository.Repos{Rates: repo}, &mockRateClient{})

	expenses := []domain.Expense{
		{Amount: 100.0, Currency: "USD"},
		{Amount: 50.0, Currency: "XXX"}, // no rate for XXX — should be skipped
	}

	total, _, err := svc.ConvertExpensesTotal(context.Background(), expenses, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 100.0 {
		t.Fatalf("expected 100.0 (skipping unconvertible), got %f", total)
	}
}

// --- CreateExpense validation ---

func TestCreateExpense_Defaults(t *testing.T) {
	// We need a minimal expense repository to capture the created expense.
	var captured domain.Expense
	mockExpenses := &expenseRepoStub{create: func(e domain.Expense) (int64, error) {
		captured = e
		return 1, nil
	}}

	svc := New(repository.Repos{Expenses: mockExpenses}, &mockRateClient{})
	ctx := context.Background()

	id, err := svc.CreateExpense(ctx, 10.0, "food", "lunch", "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
	if captured.Date == "" {
		t.Fatal("expected default date")
	}
	if captured.Currency != "USD" {
		t.Fatalf("expected default currency USD, got %s", captured.Currency)
	}
}

func TestCreateExpense_InvalidAmount(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	_, err := svc.CreateExpense(context.Background(), -5, "food", "lunch", "2024-06-01", "USD", 0)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestCreateExpense_MissingCategory(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	_, err := svc.CreateExpense(context.Background(), 10.0, "", "lunch", "2024-06-01", "USD", 0)
	if !errors.Is(err, ErrMissingCategory) {
		t.Fatalf("expected ErrMissingCategory, got %v", err)
	}
}

func TestCreateExpense_MissingDescription(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	_, err := svc.CreateExpense(context.Background(), 10.0, "food", "", "2024-06-01", "USD", 0)
	if !errors.Is(err, ErrMissingDescription) {
		t.Fatalf("expected ErrMissingDescription, got %v", err)
	}
}

func TestCreateExpense_InvalidDate(t *testing.T) {
	svc := New(repository.Repos{}, &mockRateClient{})
	_, err := svc.CreateExpense(context.Background(), 10.0, "food", "lunch", "banana", "USD", 0)
	if !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("expected ErrInvalidDate, got %v", err)
	}
}

func TestCreateExpense_ValidAndFutureDates(t *testing.T) {
	tests := []struct {
		name string
		date string
	}{
		{"past date", "2020-12-31"},
		{"today default", ""},
		{"future date", "2031-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured domain.Expense
			mockExpenses := &expenseRepoStub{create: func(e domain.Expense) (int64, error) {
				captured = e
				return 1, nil
			}}
			svc := New(repository.Repos{Expenses: mockExpenses}, &mockRateClient{})
			_, err := svc.CreateExpense(context.Background(), 10.0, "food", "lunch", tt.date, "USD", 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.date == "" {
				if captured.Date == "" {
					t.Fatal("expected default date")
				}
				return
			}
			if captured.Date != tt.date {
				t.Fatalf("expected date %q, got %q", tt.date, captured.Date)
			}
		})
	}
}

// --- Date-indexed daily groups ---

// rangeStub returns a stub whose ListExpensesInRange filters the given
// expenses by bare-string date comparison, like the real SQL does.
func rangeStub(expenses ...domain.Expense) *expenseRepoStub {
	return &expenseRepoStub{listInRange: func(start, end string) ([]domain.Expense, error) {
		var out []domain.Expense
		for _, e := range expenses {
			if e.Date >= start && e.Date <= end {
				out = append(out, e)
			}
		}
		return out, nil
	}}
}

func TestDailyGroupsInRange_GapFillsEmptyDays(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub(
		domain.Expense{ID: 1, Amount: 10, Category: "food", Description: "a", Date: "2026-06-01", Currency: "USD"},
		domain.Expense{ID: 2, Amount: 7, Category: "food", Description: "b", Date: "2026-06-05", Currency: "USD"},
		domain.Expense{ID: 3, Amount: 5, Category: "food", Description: "c", Date: "2026-06-05", Currency: "USD"},
	)}, &mockRateClient{})

	groups, err := svc.DailyGroupsInRange(context.Background(), "2026-06-01", "2026-06-05")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups (every day in range), got %d", len(groups))
	}
	wantDates := []string{"2026-06-05", "2026-06-04", "2026-06-03", "2026-06-02", "2026-06-01"}
	for i, want := range wantDates {
		if groups[i].Date != want {
			t.Fatalf("group %d: expected date %s (newest first), got %s", i, want, groups[i].Date)
		}
	}
	if len(groups[0].Expenses) != 2 || groups[0].Total != 12 {
		t.Fatalf("expected 2 expenses totalling 12 on 2026-06-05, got %d totalling %v", len(groups[0].Expenses), groups[0].Total)
	}
	if len(groups[4].Expenses) != 1 || groups[4].Total != 10 {
		t.Fatalf("expected 1 expense totalling 10 on 2026-06-01, got %d totalling %v", len(groups[4].Expenses), groups[4].Total)
	}
	for _, i := range []int{1, 2, 3} {
		if len(groups[i].Expenses) != 0 {
			t.Fatalf("expected %s to be an empty day, got %d expenses", groups[i].Date, len(groups[i].Expenses))
		}
	}
}

// TestDailyGroupsInRange_ExpensesNeverNil is the one home of the never-nil
// invariant: a gap-filled empty day carries an empty slice, never nil.
func TestDailyGroupsInRange_ExpensesNeverNil(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub()}, &mockRateClient{})

	groups, err := svc.DailyGroupsInRange(context.Background(), "2026-06-01", "2026-06-03")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups for an empty window, got %d", len(groups))
	}
	for _, g := range groups {
		if g.Expenses == nil {
			t.Fatalf("Expenses is nil on %s; the invariant is an empty slice", g.Date)
		}
	}
}

func TestDailyGroupsInRange_SingleDay(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub(
		domain.Expense{ID: 1, Amount: 3, Category: "food", Description: "a", Date: "2026-06-02", Currency: "USD"},
	)}, &mockRateClient{})

	groups, err := svc.DailyGroupsInRange(context.Background(), "2026-06-02", "2026-06-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || groups[0].Date != "2026-06-02" || len(groups[0].Expenses) != 1 {
		t.Fatalf("expected one group for 2026-06-02 with one expense, got %+v", groups)
	}
}

func TestDailyGroupsInRange_InvertedRangeIsError(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub()}, &mockRateClient{})

	_, err := svc.DailyGroupsInRange(context.Background(), "2026-06-05", "2026-06-01")
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange, got %v", err)
	}
}

func TestDailyGroupsInRange_UnparseableEndpointIsError(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub()}, &mockRateClient{})

	for _, r := range [][2]string{{"banana", "2026-06-05"}, {"2026-06-01", "banana"}} {
		if _, err := svc.DailyGroupsInRange(context.Background(), r[0], r[1]); !errors.Is(err, ErrInvalidDate) {
			t.Fatalf("range %v: expected ErrInvalidDate, got %v", r, err)
		}
	}
}

// TestTodayUpcomingPartition asserts the boundary from both sides in one
// test: an expense dated exactly today belongs to the window and not to
// Upcoming; tomorrow's is the reverse. The bug hunted is an expense landing
// in both sections at once.
func TestTodayUpcomingPartition(t *testing.T) {
	const today = "2026-07-12"
	svc := New(repository.Repos{Expenses: rangeStub(
		domain.Expense{ID: 1, Amount: 5, Category: "food", Description: "today", Date: "2026-07-12", Currency: "USD"},
		domain.Expense{ID: 2, Amount: 9, Category: "food", Description: "tomorrow", Date: "2026-07-13", Currency: "USD"},
	)}, &mockRateClient{})
	ctx := context.Background()

	window, err := svc.DailyGroupsInRange(ctx, "2026-06-13", today)
	if err != nil {
		t.Fatalf("window: unexpected error: %v", err)
	}
	upcoming, err := svc.UpcomingGroups(ctx, today)
	if err != nil {
		t.Fatalf("upcoming: unexpected error: %v", err)
	}

	inWindow := map[int64]bool{}
	for _, g := range window {
		for _, e := range g.Expenses {
			inWindow[e.ID] = true
		}
	}
	inUpcoming := map[int64]bool{}
	for _, g := range upcoming {
		for _, e := range g.Expenses {
			inUpcoming[e.ID] = true
		}
	}

	if !inWindow[1] || inUpcoming[1] {
		t.Fatalf("today's expense must be in the window and only the window (window=%v upcoming=%v)", inWindow[1], inUpcoming[1])
	}
	if inWindow[2] || !inUpcoming[2] {
		t.Fatalf("tomorrow's expense must be in Upcoming and only Upcoming (window=%v upcoming=%v)", inWindow[2], inUpcoming[2])
	}
	if window[0].Date != today {
		t.Fatalf("expected the window's newest group to be today (%s), got %s", today, window[0].Date)
	}
}

func TestUpcomingGroups_UngappedAndDescending(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub(
		domain.Expense{ID: 1, Amount: 5, Category: "rent", Description: "near", Date: "2026-07-15", Currency: "USD"},
		domain.Expense{ID: 2, Amount: 9, Category: "rent", Description: "far", Date: "2026-07-22", Currency: "USD"},
	)}, &mockRateClient{})

	groups, err := svc.UpcomingGroups(context.Background(), "2026-07-12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("Upcoming is not gap-filled: expected exactly 2 groups, got %d", len(groups))
	}
	if groups[0].Date != "2026-07-22" || groups[1].Date != "2026-07-15" {
		t.Fatalf("expected descending order (furthest future first), got %s then %s", groups[0].Date, groups[1].Date)
	}
}

func TestUpcomingGroups_UnparseableAfterIsError(t *testing.T) {
	svc := New(repository.Repos{Expenses: rangeStub()}, &mockRateClient{})

	if _, err := svc.UpcomingGroups(context.Background(), "banana"); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("expected ErrInvalidDate, got %v", err)
	}
}

// expenseRepoStub is a minimal stub for expense repository tests.
func TestExpensesMatching(t *testing.T) {
	// A fixed dataset the stub filters as a reference implementation, so each
	// case pins the ExpenseFilter contract: unset (zero-value) means "all",
	// while a set field is an equality narrow.
	all := []domain.Expense{
		{ID: 1, Category: "food", PaidByID: 10, Date: "2026-01-01"},
		{ID: 2, Category: "travel", PaidByID: 10, Date: "2026-01-02"},
		{ID: 3, Category: "food", PaidByID: 20, Date: "2026-01-03"},
	}
	var gotFilter domain.ExpenseFilter
	stub := &expenseRepoStub{matching: func(f domain.ExpenseFilter) ([]domain.Expense, error) {
		gotFilter = f
		var out []domain.Expense
		for _, e := range all {
			if e.Date < f.Start || e.Date > f.End {
				continue
			}
			if f.Category != "" && e.Category != f.Category {
				continue
			}
			if f.PayerID != 0 && e.PaidByID != f.PayerID {
				continue
			}
			out = append(out, e)
		}
		return out, nil
	}}
	svc := New(repository.Repos{Expenses: stub}, &mockRateClient{})

	cases := []struct {
		name   string
		filter domain.ExpenseFilter
		wantID []int64
	}{
		{"unset = all", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31"}, []int64{1, 2, 3}},
		{"category", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", Category: "food"}, []int64{1, 3}},
		{"payer", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", PayerID: 10}, []int64{1, 2}},
		{"category and payer", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", Category: "food", PayerID: 10}, []int64{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := svc.ExpensesMatching(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("ExpensesMatching: %v", err)
			}
			if gotFilter != tc.filter {
				t.Fatalf("service altered the filter: got %+v, want %+v", gotFilter, tc.filter)
			}
			var ids []int64
			for _, e := range rows {
				ids = append(ids, e.ID)
			}
			if !reflect.DeepEqual(ids, tc.wantID) {
				t.Fatalf("ids = %v, want %v", ids, tc.wantID)
			}
		})
	}
}

// TestCountMatching mirrors TestExpensesMatching: the same reference dataset and
// the same filter cases, asserting CountMatching returns the number of rows the
// filter selects — including the sharp unset (zero-value) = all case.
func TestCountMatching(t *testing.T) {
	all := []domain.Expense{
		{ID: 1, Category: "food", PaidByID: 10, Date: "2026-01-01"},
		{ID: 2, Category: "travel", PaidByID: 10, Date: "2026-01-02"},
		{ID: 3, Category: "food", PaidByID: 20, Date: "2026-01-03"},
	}
	matches := func(f domain.ExpenseFilter) []domain.Expense {
		var out []domain.Expense
		for _, e := range all {
			if e.Date < f.Start || e.Date > f.End {
				continue
			}
			if f.Category != "" && e.Category != f.Category {
				continue
			}
			if f.PayerID != 0 && e.PaidByID != f.PayerID {
				continue
			}
			out = append(out, e)
		}
		return out
	}
	// count and matching share the reference predicate but are independent code
	// paths — mirroring production, where both read the same filterWhere.
	stub := &expenseRepoStub{
		count:    func(f domain.ExpenseFilter) (int, error) { return len(matches(f)), nil },
		matching: func(f domain.ExpenseFilter) ([]domain.Expense, error) { return matches(f), nil },
	}
	svc := New(repository.Repos{Expenses: stub}, &mockRateClient{})

	cases := []struct {
		name   string
		filter domain.ExpenseFilter
		want   int
	}{
		{"unset = all", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31"}, 3},
		{"category", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", Category: "food"}, 2},
		{"payer", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", PayerID: 10}, 2},
		{"category and payer", domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31", Category: "food", PayerID: 10}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.CountMatching(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("CountMatching: %v", err)
			}
			if got != tc.want {
				t.Fatalf("count = %d, want %d", got, tc.want)
			}
			// The invariant that reassures the preview: the count equals the
			// download's row count for the same filter.
			rows, err := svc.ExpensesMatching(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("ExpensesMatching: %v", err)
			}
			if got != len(rows) {
				t.Fatalf("count %d != download row count %d for the same filter", got, len(rows))
			}
		})
	}
}

type expenseRepoStub struct {
	create      func(e domain.Expense) (int64, error)
	listInRange func(start, end string) ([]domain.Expense, error)
	matching    func(f domain.ExpenseFilter) ([]domain.Expense, error)
	count       func(f domain.ExpenseFilter) (int, error)
	sample      func(f domain.ExpenseFilter, limit int) ([]domain.Expense, error)
	categories  func() ([]string, error)
}

func (s *expenseRepoStub) CreateExpense(ctx context.Context, e domain.Expense) (int64, error) {
	if s.create != nil {
		return s.create(e)
	}
	return 0, nil
}

func (s *expenseRepoStub) MintCharge(ctx context.Context, e domain.Expense) (bool, error) {
	if s.create != nil {
		_, err := s.create(e)
		return err == nil, err
	}
	return false, nil
}

func (s *expenseRepoStub) RewritePendingCharge(ctx context.Context, ruleID int64, today string, e domain.Expense) (bool, error) {
	return false, nil
}

func (s *expenseRepoStub) DeletePendingCharge(ctx context.Context, ruleID int64, today string) error {
	return nil
}

func (s *expenseRepoStub) ListExpenses(ctx context.Context, limit int) ([]domain.Expense, error) {
	return nil, nil
}

func (s *expenseRepoStub) ListExpensesInRange(ctx context.Context, start, end string) ([]domain.Expense, error) {
	if s.listInRange != nil {
		return s.listInRange(start, end)
	}
	return nil, nil
}

func (s *expenseRepoStub) ExpensesMatching(ctx context.Context, f domain.ExpenseFilter) ([]domain.Expense, error) {
	if s.matching != nil {
		return s.matching(f)
	}
	return nil, nil
}

func (s *expenseRepoStub) CountMatching(ctx context.Context, f domain.ExpenseFilter) (int, error) {
	if s.count != nil {
		return s.count(f)
	}
	return 0, nil
}

func (s *expenseRepoStub) SampleMatching(ctx context.Context, f domain.ExpenseFilter, limit int) ([]domain.Expense, error) {
	if s.sample != nil {
		return s.sample(f, limit)
	}
	return nil, nil
}

func (s *expenseRepoStub) SumMatching(ctx context.Context, f domain.ExpenseFilter) (float64, error) {
	if s.matching == nil {
		return 0, nil
	}
	rows, err := s.matching(f)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, e := range rows {
		total += e.Amount
	}
	return total, nil
}

func (s *expenseRepoStub) DistinctCategories(ctx context.Context) ([]string, error) {
	if s.categories != nil {
		return s.categories()
	}
	return nil, nil
}

func (s *expenseRepoStub) GetExpense(ctx context.Context, id int64) (*domain.Expense, error) {
	return nil, nil
}

func (s *expenseRepoStub) UpdateExpense(ctx context.Context, e domain.Expense) error {
	return nil
}

func (s *expenseRepoStub) DeleteExpense(ctx context.Context, id int64) error {
	return nil
}

func (s *expenseRepoStub) ListCategories(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (s *expenseRepoStub) SummaryByCategory(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}

func (s *expenseRepoStub) TotalExpenses(ctx context.Context) (float64, error) {
	return 0, nil
}

func (s *expenseRepoStub) GetEarliestExpenseDate(ctx context.Context) (string, error) {
	return "", nil
}

func (s *expenseRepoStub) ListDescriptionsByCategory(ctx context.Context, category string, limit int) ([]domain.DescriptionCount, error) {
	return nil, nil
}
