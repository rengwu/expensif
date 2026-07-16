package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
)

var (
	ErrInvalidAmount      = errors.New("Amount must be greater than 0")
	ErrMissingCategory    = errors.New("Category is required")
	ErrMissingDescription = errors.New("Description is required")
	ErrInvalidDate        = errors.New("Date must be a valid YYYY-MM-DD")
	ErrInvalidRange       = errors.New("Start date must not be after end date")
	ErrNoRates            = errors.New("No exchange rates available")
)

type RateFetcher interface {
	Latest(ctx context.Context, base string) (map[string]float64, string, error)
}

type Service struct {
	expenses   repository.ExpenseRepository
	users      repository.UserRepository
	prefs      repository.PreferenceRepository
	rates      repository.RateRepository
	rateClient RateFetcher
}

func New(repos repository.Repos, rateClient RateFetcher) *Service {
	return &Service{
		expenses:   repos.Expenses,
		users:      repos.Users,
		prefs:      repos.Preferences,
		rates:      repos.Rates,
		rateClient: rateClient,
	}
}

// --- Expenses ---

func validateExpenseInput(amount float64, category, description, date, currency string) (string, string, string, string, error) {
	if amount <= 0 {
		return "", "", "", "", ErrInvalidAmount
	}
	category = strings.TrimSpace(category)
	description = strings.TrimSpace(description)
	if category == "" {
		return "", "", "", "", ErrMissingCategory
	}
	if description == "" {
		return "", "", "", "", ErrMissingDescription
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", "", "", "", ErrInvalidDate
	}
	if currency == "" {
		currency = "USD"
	}
	return category, description, date, currency, nil
}

func (s *Service) CreateExpense(ctx context.Context, amount float64, category, description, date, currency string, paidByID int64) (int64, error) {
	category, description, date, currency, err := validateExpenseInput(amount, category, description, date, currency)
	if err != nil {
		return 0, err
	}
	e := domain.Expense{
		Amount:      amount,
		Category:    category,
		Description: description,
		Date:        date,
		Currency:    currency,
		PaidByID:    paidByID,
	}
	return s.expenses.CreateExpense(ctx, e)
}

func (s *Service) ListExpenses(ctx context.Context, limit int) ([]domain.Expense, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.expenses.ListExpenses(ctx, limit)
}

func (s *Service) GetExpense(ctx context.Context, id int64) (*domain.Expense, error) {
	return s.expenses.GetExpense(ctx, id)
}

func (s *Service) ExpensesInRange(ctx context.Context, start, end string) ([]domain.Expense, error) {
	return s.expenses.ListExpensesInRange(ctx, start, end)
}

// ExpensesMatching returns the raw expenses matching the filter (newest first),
// leaving conversion to the caller. It is the single filtering seam; the List
// View is intentionally not wired to it yet.
func (s *Service) ExpensesMatching(ctx context.Context, f domain.ExpenseFilter) ([]domain.Expense, error) {
	return s.expenses.ExpensesMatching(ctx, f)
}

func (s *Service) UpdateExpense(ctx context.Context, id int64, amount float64, category, description, date, currency string, paidByID int64) error {
	category, description, date, currency, err := validateExpenseInput(amount, category, description, date, currency)
	if err != nil {
		return err
	}
	e := domain.Expense{
		ID:          id,
		Amount:      amount,
		Category:    category,
		Description: description,
		Date:        date,
		Currency:    currency,
		PaidByID:    paidByID,
	}
	return s.expenses.UpdateExpense(ctx, e)
}

func (s *Service) DeleteExpense(ctx context.Context, id int64) error {
	return s.expenses.DeleteExpense(ctx, id)
}

func (s *Service) ListCategories(ctx context.Context) ([]string, error) {
	return s.expenses.ListCategories(ctx)
}

func (s *Service) ListDescriptionsByCategory(ctx context.Context, category string, limit int) ([]domain.DescriptionCount, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.expenses.ListDescriptionsByCategory(ctx, category, limit)
}

func (s *Service) SummaryByCategory(ctx context.Context) (map[string]float64, error) {
	return s.expenses.SummaryByCategory(ctx)
}

func (s *Service) TotalExpenses(ctx context.Context) (float64, error) {
	return s.expenses.TotalExpenses(ctx)
}

func (s *Service) GetEarliestExpenseDate(ctx context.Context) (string, error) {
	return s.expenses.GetEarliestExpenseDate(ctx)
}

// DailyGroupsInRange returns one group per day in [start, end], newest
// first. Every day appears, empty days included; an empty day's Expenses
// is an empty slice, never nil. Dates are bare YYYY-MM-DD strings — the
// walk runs on UTC-parsed values, so the caller's timezone is never
// consulted here.
func (s *Service) DailyGroupsInRange(ctx context.Context, start, end string) ([]domain.DailyGroup, error) {
	startT, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, ErrInvalidDate
	}
	endT, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, ErrInvalidDate
	}
	if startT.After(endT) {
		return nil, ErrInvalidRange
	}
	expenses, err := s.expenses.ListExpensesInRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	byDate := groupByDate(expenses)
	var groups []domain.DailyGroup
	for d := endT; !d.Before(startT); d = d.AddDate(0, 0, -1) {
		date := d.Format("2006-01-02")
		exps := byDate[date]
		if exps == nil {
			exps = []domain.Expense{}
		}
		groups = append(groups, newDailyGroup(date, exps))
	}
	return groups, nil
}

// UpcomingGroups returns expenses dated strictly after `after`, grouped by
// day, newest first. Not gap-filled: only days that carry an expense appear.
func (s *Service) UpcomingGroups(ctx context.Context, after string) ([]domain.DailyGroup, error) {
	afterT, err := time.Parse("2006-01-02", after)
	if err != nil {
		return nil, ErrInvalidDate
	}
	from := afterT.AddDate(0, 0, 1).Format("2006-01-02")
	expenses, err := s.expenses.ListExpensesInRange(ctx, from, "9999-12-31")
	if err != nil {
		return nil, err
	}
	byDate := groupByDate(expenses)
	var groups []domain.DailyGroup
	for date, exps := range byDate {
		groups = append(groups, newDailyGroup(date, exps))
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Date > groups[j].Date
	})
	return groups, nil
}

func groupByDate(expenses []domain.Expense) map[string][]domain.Expense {
	byDate := make(map[string][]domain.Expense)
	for _, e := range expenses {
		byDate[e.Date] = append(byDate[e.Date], e)
	}
	return byDate
}

func newDailyGroup(date string, exps []domain.Expense) domain.DailyGroup {
	var total float64
	for _, e := range exps {
		total += e.Amount
	}
	return domain.DailyGroup{Date: date, Expenses: exps, Total: total}
}

// --- Preferences ---

func (s *Service) Preferences(ctx context.Context) (*domain.Preferences, error) {
	p, err := s.prefs.GetPreferences(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultPreferences(), nil
		}
		return nil, err
	}
	if p.Timezone == "" {
		p.Timezone = defaultTimezone()
	}
	return p, nil
}

func defaultPreferences() *domain.Preferences {
	return &domain.Preferences{Currency: "USD", Timezone: defaultTimezone()}
}

func defaultTimezone() string {
	if loc := time.Local; loc != nil {
		return loc.String()
	}
	return "UTC"
}

func (s *Service) SavePreferences(ctx context.Context, currency string, userID int64, timezone string) error {
	if currency == "" {
		currency = "USD"
	}
	if timezone == "" {
		timezone = defaultTimezone()
	}
	return s.prefs.SavePreferences(ctx, domain.Preferences{
		Currency: currency,
		UserID:   userID,
		Timezone: timezone,
	})
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.ListUsers(ctx)
}

func (s *Service) CreateUser(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("name is required")
	}
	return s.users.CreateUser(ctx, name)
}

func (s *Service) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.GetUser(ctx, id)
}

func (s *Service) UpdateUser(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	return s.users.UpdateUser(ctx, id, name)
}

func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	return s.users.DeleteUser(ctx, id)
}

// --- Exchange Rates ---

func (s *Service) RefreshRates(ctx context.Context) error {
	rates, date, err := s.rateClient.Latest(ctx, "USD")
	if err != nil {
		return fmt.Errorf("fetch rates: %w", err)
	}
	return s.rates.SaveRates(ctx, "USD", date, rates)
}

func (s *Service) GetRatesForConversion(ctx context.Context) (map[string]float64, string, error) {
	today := time.Now().Format("2006-01-02")
	rates, err := s.rates.GetRates(ctx, "USD", today)
	if err == nil && len(rates) > 0 {
		return rates, today, nil
	}
	rates, date, err := s.rates.GetLatestRates(ctx, "USD")
	if err != nil {
		return nil, "", ErrNoRates
	}
	if len(rates) == 0 {
		return nil, "", ErrNoRates
	}
	return rates, date, nil
}

func (s *Service) ConvertWithRates(amount float64, from, to string, rates map[string]float64) (float64, error) {
	return CrossRate(amount, from, to, rates)
}

// CrossRate converts amount from one currency to another via a single base's
// rate set (rates[X] is X per base). It is a pure function so callers outside a
// Service — such as the CSV record builder — can reuse the exact conversion the
// views use. Same-currency short-circuits; a currency the set doesn't cover is
// an error, which the caller renders as a blank cell rather than a zero.
func CrossRate(amount float64, from, to string, rates map[string]float64) (float64, error) {
	if from == to {
		return amount, nil
	}
	fromRate, ok := rates[from]
	if !ok {
		return 0, fmt.Errorf("no rate for %s", from)
	}
	toRate, ok := rates[to]
	if !ok {
		return 0, fmt.Errorf("no rate for %s", to)
	}
	return amount * toRate / fromRate, nil
}

func (s *Service) ConvertExpensesTotal(ctx context.Context, expenses []domain.Expense, target string) (float64, string, error) {
	rates, date, err := s.GetRatesForConversion(ctx)
	if err != nil {
		return 0, "", err
	}
	var total float64
	for _, e := range expenses {
		conv, err := s.ConvertWithRates(e.Amount, e.Currency, target, rates)
		if err != nil {
			continue // skip unconvertible currencies
		}
		total += conv
	}
	return total, date, nil
}
