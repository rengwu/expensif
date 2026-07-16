package repository

import (
	"context"

	"expensif/internal/domain"
)

type ExpenseRepository interface {
	CreateExpense(ctx context.Context, e domain.Expense) (int64, error)
	ListExpenses(ctx context.Context, limit int) ([]domain.Expense, error)
	ListExpensesInRange(ctx context.Context, start, end string) ([]domain.Expense, error)
	ExpensesMatching(ctx context.Context, filter domain.ExpenseFilter) ([]domain.Expense, error)
	GetExpense(ctx context.Context, id int64) (*domain.Expense, error)
	UpdateExpense(ctx context.Context, e domain.Expense) error
	DeleteExpense(ctx context.Context, id int64) error
	ListCategories(ctx context.Context) ([]string, error)
	SummaryByCategory(ctx context.Context) (map[string]float64, error)
	TotalExpenses(ctx context.Context) (float64, error)
	GetEarliestExpenseDate(ctx context.Context) (string, error)
	ListDescriptionsByCategory(ctx context.Context, category string, limit int) ([]domain.DescriptionCount, error)
}

type PreferenceRepository interface {
	GetPreferences(ctx context.Context) (*domain.Preferences, error)
	SavePreferences(ctx context.Context, p domain.Preferences) error
}

type RateRepository interface {
	SaveRates(ctx context.Context, base string, date string, rates map[string]float64) error
	GetRates(ctx context.Context, base string, date string) (map[string]float64, error)
	GetLatestRates(ctx context.Context, base string) (map[string]float64, string, error)
}

type UserRepository interface {
	ListUsers(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, name string) (int64, error)
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	UpdateUser(ctx context.Context, id int64, name string) error
	DeleteUser(ctx context.Context, id int64) error
}

// Repository is the full composition of all domain repositories.
// Useful when a single concrete implementation (e.g. SQLite) satisfies everything.
type Repository interface {
	ExpenseRepository
	PreferenceRepository
	RateRepository
	UserRepository
}

// Repos bundles the focused repository interfaces for dependency injection.
type Repos struct {
	Expenses    ExpenseRepository
	Users       UserRepository
	Preferences PreferenceRepository
	Rates       RateRepository
}
