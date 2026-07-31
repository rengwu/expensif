package web

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
	"expensif/internal/service"
)

// mockRepo is an in-memory implementation of repository.Repository for testing.
type mockRepo struct {
	mu         sync.RWMutex
	expenses   []domain.Expense
	nextID     int64
	prefs      domain.Preferences
	rates      map[string]map[string]map[string]float64 // base -> date -> target -> rate
	users      map[int64]string
	nextUserID int64
	rules      map[int64]domain.RecurringRule
	nextRuleID int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		expenses:   make([]domain.Expense, 0),
		nextID:     1,
		prefs:      domain.Preferences{Currency: "USD"},
		rates:      make(map[string]map[string]map[string]float64),
		users:      make(map[int64]string),
		nextUserID: 1,
		rules:      make(map[int64]domain.RecurringRule),
		nextRuleID: 1,
	}
}

func (r *mockRepo) CreateExpense(_ context.Context, e domain.Expense) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.ID = r.nextID
	e.CreatedAt = time.Now()
	r.nextID++
	r.expenses = append(r.expenses, e)
	return e.ID, nil
}

func (r *mockRepo) ListExpenses(_ context.Context, limit int) ([]domain.Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > len(r.expenses) {
		limit = len(r.expenses)
	}
	result := make([]domain.Expense, limit)
	copy(result, r.expenses[:limit])
	for i := range result {
		if result[i].PaidByID != 0 {
			if name, ok := r.users[result[i].PaidByID]; ok {
				result[i].PaidByName = name
			}
		}
	}
	return result, nil
}

func (r *mockRepo) ListExpensesInRange(_ context.Context, start, end string) ([]domain.Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []domain.Expense
	for _, e := range r.expenses {
		if e.Date >= start && e.Date <= end {
			if e.PaidByID != 0 {
				if name, ok := r.users[e.PaidByID]; ok {
					e.PaidByName = name
				}
			}
			result = append(result, e)
		}
	}
	return result, nil
}

func (r *mockRepo) ExpensesMatching(_ context.Context, f domain.ExpenseFilter) ([]domain.Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []domain.Expense
	for _, e := range r.expenses {
		if e.Date < f.Start || e.Date > f.End {
			continue
		}
		if f.Category != "" && e.Category != f.Category {
			continue
		}
		if f.PayerID != 0 && e.PaidByID != f.PayerID {
			continue
		}
		if e.PaidByID != 0 {
			if name, ok := r.users[e.PaidByID]; ok {
				e.PaidByName = name
			}
		}
		result = append(result, e)
	}
	// Newest first, matching the SQL ORDER BY e.date DESC, e.created_at DESC.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date > result[j].Date
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// CountMatching filters independently of ExpensesMatching (its own predicate
// copy), so a test asserting count == len(rows) proves the two paths agree rather
// than delegating the answer.
func (r *mockRepo) CountMatching(_ context.Context, f domain.ExpenseFilter) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, e := range r.expenses {
		if e.Date < f.Start || e.Date > f.End {
			continue
		}
		if f.Category != "" && e.Category != f.Category {
			continue
		}
		if f.PayerID != 0 && e.PaidByID != f.PayerID {
			continue
		}
		n++
	}
	return n, nil
}

func (r *mockRepo) SampleMatching(ctx context.Context, f domain.ExpenseFilter, limit int) ([]domain.Expense, error) {
	rows, err := r.ExpensesMatching(ctx, f)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (r *mockRepo) DistinctCategories(_ context.Context) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	var cats []string
	for _, e := range r.expenses {
		if e.Category != "" && !seen[e.Category] {
			seen[e.Category] = true
			cats = append(cats, e.Category)
		}
	}
	sort.Strings(cats)
	return cats, nil
}

func (r *mockRepo) GetExpense(_ context.Context, id int64) (*domain.Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.expenses {
		if r.expenses[i].ID == id {
			e := r.expenses[i]
			if e.PaidByID != 0 {
				if name, ok := r.users[e.PaidByID]; ok {
					e.PaidByName = name
				}
			}
			return &e, nil
		}
	}
	return nil, fmt.Errorf("no expense with id %d", id)
}

func (r *mockRepo) UpdateExpense(_ context.Context, e domain.Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.expenses {
		if r.expenses[i].ID == e.ID {
			r.expenses[i] = e
			return nil
		}
	}
	return fmt.Errorf("no expense with id %d", e.ID)
}

func (r *mockRepo) DeleteExpense(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.expenses {
		if r.expenses[i].ID == id {
			r.expenses = append(r.expenses[:i], r.expenses[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no expense with id %d", id)
}

func (r *mockRepo) ListCategories(_ context.Context) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	freq := make(map[string]int)
	cutoff := time.Now().AddDate(0, -3, 0).Format("2006-01-02")
	for _, e := range r.expenses {
		if e.Date >= cutoff {
			freq[e.Category]++
		}
	}
	cats := make([]string, 0, len(freq))
	for cat := range freq {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool {
		if freq[cats[i]] != freq[cats[j]] {
			return freq[cats[i]] > freq[cats[j]]
		}
		return cats[i] < cats[j]
	})
	return cats, nil
}

func (r *mockRepo) ListDescriptionsByCategory(_ context.Context, category string, limit int) ([]domain.DescriptionCount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	freq := make(map[string]int)
	for _, e := range r.expenses {
		if strings.EqualFold(e.Category, category) && e.Description != "" {
			freq[strings.ToLower(e.Description)]++
		}
	}
	var results []domain.DescriptionCount
	for desc, count := range freq {
		results = append(results, domain.DescriptionCount{Description: desc, Count: count})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Count != results[j].Count {
			return results[i].Count > results[j].Count
		}
		return results[i].Description < results[j].Description
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (r *mockRepo) SummaryByCategory(_ context.Context) (map[string]float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := make(map[string]float64)
	for _, e := range r.expenses {
		m[e.Category] += e.Amount
	}
	return m, nil
}

func (r *mockRepo) TotalExpenses(_ context.Context) (float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total float64
	for _, e := range r.expenses {
		total += e.Amount
	}
	return total, nil
}

func (r *mockRepo) GetEarliestExpenseDate(_ context.Context) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var earliest string
	for _, e := range r.expenses {
		if earliest == "" || e.Date < earliest {
			earliest = e.Date
		}
	}
	return earliest, nil
}

func (r *mockRepo) GetPreferences(_ context.Context) (*domain.Preferences, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p := r.prefs
	return &p, nil
}

func (r *mockRepo) SavePreferences(_ context.Context, p domain.Preferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefs = p
	return nil
}

func copyRates(rates map[string]float64) map[string]float64 {
	cp := make(map[string]float64, len(rates))
	for k, v := range rates {
		cp[k] = v
	}
	return cp
}

func (r *mockRepo) SaveRates(_ context.Context, base string, date string, rates map[string]float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rates[base] == nil {
		r.rates[base] = make(map[string]map[string]float64)
	}
	r.rates[base][date] = copyRates(rates)
	return nil
}

func (r *mockRepo) GetRates(_ context.Context, base string, date string) (map[string]float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if byDate, ok := r.rates[base]; ok {
		if rates, ok := byDate[date]; ok && len(rates) > 0 {
			return copyRates(rates), nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *mockRepo) GetLatestRates(_ context.Context, base string) (map[string]float64, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest string
	for d := range r.rates[base] {
		if d > latest {
			latest = d
		}
	}
	if latest == "" {
		return nil, "", sql.ErrNoRows
	}
	return copyRates(r.rates[base][latest]), latest, nil
}

func (r *mockRepo) RatesOnOrBefore(_ context.Context, base string, date string) (map[string]float64, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched string
	for d := range r.rates[base] {
		if d <= date && d > matched {
			matched = d
		}
	}
	if matched == "" {
		return nil, "", sql.ErrNoRows
	}
	return copyRates(r.rates[base][matched]), matched, nil
}

func (r *mockRepo) RateDatesInRange(_ context.Context, base string, from, to string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var dates []string
	for d := range r.rates[base] {
		if d >= from && d <= to {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	return dates, nil
}

func (r *mockRepo) ListUsers(_ context.Context) ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var users []domain.User
	for id, name := range r.users {
		users = append(users, domain.User{ID: id, Name: name})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Name < users[j].Name
	})
	return users, nil
}

func (r *mockRepo) CreateUser(_ context.Context, name string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// dedup check
	for _, n := range r.users {
		if n == name {
			return 0, fmt.Errorf("user already exists")
		}
	}
	id := r.nextUserID
	r.users[id] = name
	r.nextUserID++
	return id, nil
}

func (r *mockRepo) GetUser(_ context.Context, id int64) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name, ok := r.users[id]; ok {
		return &domain.User{ID: id, Name: name}, nil
	}
	return nil, fmt.Errorf("no user with id %d", id)
}

func (r *mockRepo) UpdateUser(_ context.Context, id int64, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return fmt.Errorf("no user with id %d", id)
	}
	r.users[id] = name
	return nil
}

func (r *mockRepo) DeleteUser(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return fmt.Errorf("no user with id %d", id)
	}
	delete(r.users, id)
	for i := range r.expenses {
		if r.expenses[i].PaidByID == id {
			r.expenses[i].PaidByID = 0
		}
	}
	return nil
}

func (r *mockRepo) CreateRule(_ context.Context, rule domain.RecurringRule) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule.ID = r.nextRuleID
	r.nextRuleID++
	r.rules[rule.ID] = rule
	return rule.ID, nil
}

func (r *mockRepo) GetRule(_ context.Context, id int64) (*domain.RecurringRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.rules[id]
	if !ok {
		return nil, fmt.Errorf("no recurring rule with id %d", id)
	}
	return &rule, nil
}

func (r *mockRepo) ListRules(_ context.Context) ([]domain.RecurringRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var rules []domain.RecurringRule
	for _, rule := range r.rules {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].NextChargeDate != rules[j].NextChargeDate {
			return rules[i].NextChargeDate < rules[j].NextChargeDate
		}
		return rules[i].Name < rules[j].Name
	})
	return rules, nil
}

func (r *mockRepo) UpdateRule(_ context.Context, rule domain.RecurringRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[rule.ID]; !ok {
		return fmt.Errorf("no recurring rule with id %d", rule.ID)
	}
	r.rules[rule.ID] = rule
	return nil
}

func (r *mockRepo) DeleteRule(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[id]; !ok {
		return fmt.Errorf("no recurring rule with id %d", id)
	}
	delete(r.rules, id)
	return nil
}

// seed adds a few expenses for testing convenience.
func (r *mockRepo) seed() {
	now := time.Now().Format("2006-01-02")
	r.CreateExpense(context.Background(), domain.Expense{Amount: 12.5, Category: "food", Description: "lunch", Date: now, Currency: "USD"})
	r.CreateExpense(context.Background(), domain.Expense{Amount: 45.0, Category: "transport", Description: "taxi", Date: now, Currency: "USD"})
	r.CreateExpense(context.Background(), domain.Expense{Amount: 99.99, Category: "food", Description: "groceries", Date: now, Currency: "EUR"})
}

// ensure mockRepo implements all repository interfaces at compile time.
// mockRateClient is a test double for service.RateFetcher.
type mockRateClient struct {
	rates map[string]float64
	date  string
	err   error

	tsErr  error
	tsFunc func(start, end string) map[string]map[string]float64
}

func (m *mockRateClient) Latest(ctx context.Context, base string) (map[string]float64, string, error) {
	return m.rates, m.date, m.err
}

func (m *mockRateClient) Timeseries(ctx context.Context, base, start, end string) (map[string]map[string]float64, error) {
	if m.tsErr != nil {
		return nil, m.tsErr
	}
	if m.tsFunc != nil {
		return m.tsFunc(start, end), nil
	}
	return map[string]map[string]float64{}, nil
}

var _ repository.Repository = (*mockRepo)(nil)
var _ repository.ExpenseRepository = (*mockRepo)(nil)
var _ repository.UserRepository = (*mockRepo)(nil)
var _ repository.PreferenceRepository = (*mockRepo)(nil)
var _ repository.RateRepository = (*mockRepo)(nil)
var _ repository.RecurringRuleRepository = (*mockRepo)(nil)
var _ service.RateFetcher = (*mockRateClient)(nil)
