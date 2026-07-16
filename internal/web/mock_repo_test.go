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
	rates      map[string]map[string]float64 // base -> target -> rate
	rateDates  map[string]string             // base -> date
	users      map[int64]string
	nextUserID int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		expenses:   make([]domain.Expense, 0),
		nextID:     1,
		prefs:      domain.Preferences{Currency: "USD"},
		rates:      make(map[string]map[string]float64),
		rateDates:  make(map[string]string),
		users:      make(map[int64]string),
		nextUserID: 1,
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

func (r *mockRepo) SaveRates(_ context.Context, base string, date string, rates map[string]float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rates[base] = make(map[string]float64, len(rates))
	for k, v := range rates {
		r.rates[base][k] = v
	}
	r.rateDates[base] = date
	return nil
}

func (r *mockRepo) GetRates(_ context.Context, base string, _ string) (map[string]float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rates, ok := r.rates[base]; ok && len(rates) > 0 {
		cp := make(map[string]float64, len(rates))
		for k, v := range rates {
			cp[k] = v
		}
		return cp, nil
	}
	return nil, sql.ErrNoRows
}

func (r *mockRepo) GetLatestRates(_ context.Context, base string) (map[string]float64, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rates, ok := r.rates[base]; ok && len(rates) > 0 {
		cp := make(map[string]float64, len(rates))
		for k, v := range rates {
			cp[k] = v
		}
		return cp, r.rateDates[base], nil
	}
	return nil, "", sql.ErrNoRows
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
}

func (m *mockRateClient) Latest(ctx context.Context, base string) (map[string]float64, string, error) {
	return m.rates, m.date, m.err
}

var _ repository.Repository = (*mockRepo)(nil)
var _ repository.ExpenseRepository = (*mockRepo)(nil)
var _ repository.UserRepository = (*mockRepo)(nil)
var _ repository.PreferenceRepository = (*mockRepo)(nil)
var _ repository.RateRepository = (*mockRepo)(nil)
var _ service.RateFetcher = (*mockRateClient)(nil)
