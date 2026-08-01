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

	// Recurring rules. A rule's name is the Description of the expenses it
	// mints, so it has a name where an expense has a description.
	ErrMissingName          = errors.New("Name is required")
	ErrInvalidIntervalUnit  = errors.New("Interval must be months, weeks or days")
	ErrInvalidIntervalCount = errors.New("Interval count must be at least 1")
	ErrInvalidAnchorDay     = errors.New("Anchor day must be between 1 and 31")
)

type RateFetcher interface {
	Latest(ctx context.Context, base string) (map[string]float64, string, error)
	Timeseries(ctx context.Context, base, start, end string) (map[string]map[string]float64, error)
}

type Service struct {
	expenses   repository.ExpenseRepository
	users      repository.UserRepository
	prefs      repository.PreferenceRepository
	rates      repository.RateRepository
	rules      repository.RecurringRuleRepository
	rateClient RateFetcher
}

func New(repos repository.Repos, rateClient RateFetcher) *Service {
	return &Service{
		expenses:   repos.Expenses,
		users:      repos.Users,
		prefs:      repos.Preferences,
		rates:      repos.Rates,
		rules:      repos.Rules,
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
// leaving conversion to the caller. It is the single filtering seam — the Export
// and the List View both narrow through it.
func (s *Service) ExpensesMatching(ctx context.Context, f domain.ExpenseFilter) ([]domain.Expense, error) {
	return s.expenses.ExpensesMatching(ctx, f)
}

// CountMatching returns how many expenses match the filter — the Export preview's
// exact total. It reads the same filtering seam as ExpensesMatching, so the count
// equals the download's row count for the same filter.
func (s *Service) CountMatching(ctx context.Context, f domain.ExpenseFilter) (int, error) {
	return s.expenses.CountMatching(ctx, f)
}

// SampleMatching returns the first limit matching expenses (newest first) for the
// preview sample — the genuine head of the download.
func (s *Service) SampleMatching(ctx context.Context, f domain.ExpenseFilter, limit int) ([]domain.Expense, error) {
	return s.expenses.SampleMatching(ctx, f, limit)
}

// SumMatching returns the raw, unconverted total of the expenses matching the
// filter. It reads the same filtering seam as ExpensesMatching, so a caller that
// lists through one filter and totals through the same one cannot show a total
// that covers a different set of expenses than the rows beneath it.
func (s *Service) SumMatching(ctx context.Context, f domain.ExpenseFilter) (float64, error) {
	return s.expenses.SumMatching(ctx, f)
}

// DistinctCategories returns every category the account has used, for the Export
// page's category dropdown.
func (s *Service) DistinctCategories(ctx context.Context) ([]string, error) {
	return s.expenses.DistinctCategories(ctx)
}

func (s *Service) UpdateExpense(ctx context.Context, id int64, amount float64, category, description, date, currency string, paidByID int64) error {
	category, description, date, currency, err := validateExpenseInput(amount, category, description, date, currency)
	if err != nil {
		return err
	}
	existing, err := s.expenses.GetExpense(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("expense %d was not found", id)
	}

	// A Pending Charge is still an ordinary editable Expense. If that edit moves
	// its date, move the Rule's pointer with it so generation, Rule edits,
	// disabling and deletion continue to identify the same row. Historical
	// charges retain the rule reference too, so only the Expense currently named
	// by NextChargeDate is allowed to move the schedule.
	var movedRule *domain.RecurringRule
	if existing.RecurringRuleID != 0 && existing.Date != date {
		rule, err := s.rules.GetRule(ctx, existing.RecurringRuleID)
		if err != nil {
			return err
		}
		if rule.NextChargeDate == existing.Date {
			rule.NextChargeDate = date
			movedRule = rule
		}
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
	if err := s.expenses.UpdateExpense(ctx, e); err != nil {
		return err
	}
	if movedRule != nil {
		return s.rules.UpdateRule(ctx, *movedRule)
	}
	return nil
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

// rateBase is the single base currency the historical rate store is keyed on;
// every conversion is a cross-rate through it.
const rateBase = "USD"

// rateLookback widens a backfill (and the coverage probe) a week before a
// range's start, so a weekend or holiday expense at the very start of the range
// still finds a prior trading day's rate. It doubles as the freshness window in
// rangeCovered: a business day counts as covered when a stored rate sits within
// this many days on-or-before it, which is why an ordinary weekend gap (and a
// provider-omitted weekday holiday) reads as covered rather than forcing a
// re-fetch on every export.
const rateLookback = 7 * 24 * time.Hour

// EnsureRatesForRange backfills the historical rate store so every business day
// in [from, to] is covered, fetching Frankfurter's time-series endpoint exactly
// once — and only when the store lacks a business day the range needs. A second
// export over an already-covered range makes zero network calls. It errors only
// when the fetch itself fails (a hung or down provider), which the handler maps
// to a 503; a range the provider simply has no data for converts to blank cells
// downstream, not a failure.
func (s *Service) EnsureRatesForRange(ctx context.Context, from, to string) error {
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return ErrInvalidDate
	}
	toT, err := time.Parse("2006-01-02", to)
	if err != nil {
		return ErrInvalidDate
	}
	if fromT.After(toT) {
		return nil
	}

	covered, err := s.rangeCovered(ctx, fromT, toT)
	if err != nil {
		return err
	}
	if covered {
		return nil
	}

	fetchFrom := fromT.Add(-rateLookback).Format("2006-01-02")
	series, err := s.rateClient.Timeseries(ctx, rateBase, fetchFrom, to)
	if err != nil {
		return fmt.Errorf("fetch rates: %w", err)
	}
	for date, rates := range series {
		if err := s.rates.SaveRates(ctx, rateBase, date, rates); err != nil {
			return err
		}
	}
	return nil
}

// rangeCovered reports whether the store already holds, for every business day
// (Mon–Fri) in [from, to], a rate on-or-before it within rateLookback. Weekends
// are skipped — the provider never quotes them. A weekday whose nearest stored
// rate is older than the window (or absent) makes the range uncovered, so the
// backfill runs; a fully-fetched range — including one spanning a provider-
// omitted weekday holiday, covered by the prior trading day — reads as covered,
// so re-exports fetch nothing. The probe reads dates from rateLookback before
// `from` so a weekday at the very start still sees its prior trading day.
func (s *Service) rangeCovered(ctx context.Context, from, to time.Time) (bool, error) {
	probeFrom := from.Add(-rateLookback).Format("2006-01-02")
	dates, err := s.rates.RateDatesInRange(ctx, rateBase, probeFrom, to.Format("2006-01-02"))
	if err != nil {
		return false, err
	}
	stored := make([]time.Time, 0, len(dates))
	for _, d := range dates {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			stored = append(stored, t) // RateDatesInRange returns them ascending
		}
	}

	i := 0
	var last time.Time
	hasLast := false
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for i < len(stored) && !stored[i].After(d) {
			last, hasLast = stored[i], true
			i++
		}
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		if !hasLast || d.Sub(last) > rateLookback {
			return false, nil
		}
	}
	return true, nil
}

// RatesOnOrBefore returns the nearest stored rate set on-or-before date and the
// date it matched, so a non-trading-day expense resolves to the prior trading
// day's rate. The CSV export injects this as its per-row rate resolver.
func (s *Service) RatesOnOrBefore(ctx context.Context, date string) (map[string]float64, string, error) {
	return s.rates.RatesOnOrBefore(ctx, rateBase, date)
}

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
