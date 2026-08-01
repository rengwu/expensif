package repository

import (
	"context"

	"expensif/internal/domain"
)

type ExpenseRepository interface {
	CreateExpense(ctx context.Context, e domain.Expense) (int64, error)
	// MintCharge creates the expense a Recurring Rule owes for one of its dates
	// and reports whether this call was the one to create it. A charge the rule
	// has already minted for that date is left exactly as it stands — a human's
	// edit to it included — so generation can run as often as it likes and the
	// (rule, date) uniqueness constraint, not the caller's bookkeeping, is what
	// makes double-charging impossible.
	MintCharge(ctx context.Context, e domain.Expense) (bool, error)
	// RewritePendingCharge replaces every user-editable field on the Rule's one
	// future-dated charge, preserving its identity and CreatedAt. It reports
	// whether that Pending Charge existed to rewrite.
	RewritePendingCharge(ctx context.Context, ruleID int64, today string, e domain.Expense) (bool, error)
	// DeletePendingCharge removes the Rule's one future-dated charge. A charge
	// due today is already ledger history and must survive.
	DeletePendingCharge(ctx context.Context, ruleID int64, today string) error
	ListExpenses(ctx context.Context, limit int) ([]domain.Expense, error)
	ListExpensesInRange(ctx context.Context, start, end string) ([]domain.Expense, error)
	ExpensesMatching(ctx context.Context, filter domain.ExpenseFilter) ([]domain.Expense, error)
	// CountMatching returns how many expenses match the filter — a COUNT(*)
	// sibling of ExpensesMatching built from the same filter→WHERE helper, so the
	// preview count equals the download's row count by construction.
	CountMatching(ctx context.Context, filter domain.ExpenseFilter) (int, error)
	// SampleMatching returns the first limit expenses matching the filter, ordered
	// identically to ExpensesMatching — the genuine first rows of the download,
	// for the preview sample.
	SampleMatching(ctx context.Context, filter domain.ExpenseFilter, limit int) ([]domain.Expense, error)
	// SumMatching adds up the raw amounts of the expenses matching the filter — a
	// SUM(amount) sibling of CountMatching over the same filter→WHERE helper. The
	// amounts are not converted, so it is only meaningful where a raw total already
	// was; its purpose is to let a total be date-bounded by the same filter that
	// bounds the rows it summarises.
	SumMatching(ctx context.Context, filter domain.ExpenseFilter) (float64, error)
	// DistinctCategories returns every category the account has ever used, sorted,
	// for the Export page's category dropdown (category is free-text upstream).
	DistinctCategories(ctx context.Context) ([]string, error)
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
	// RatesOnOrBefore returns the nearest stored rate set on or before date and
	// the date it actually matched, so a weekend/holiday expense resolves to the
	// prior trading day's rate over the now-dense store. sql.ErrNoRows when the
	// store holds nothing at or before date.
	RatesOnOrBefore(ctx context.Context, base string, date string) (map[string]float64, string, error)
	// RateDatesInRange returns the distinct dates the store holds for base within
	// [from, to], ascending — the coverage probe EnsureRatesForRange reads to
	// decide whether a range still needs backfilling.
	RateDatesInRange(ctx context.Context, base string, from, to string) ([]string, error)
}

type UserRepository interface {
	ListUsers(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, name string) (int64, error)
	GetUser(ctx context.Context, id int64) (*domain.User, error)
	UpdateUser(ctx context.Context, id int64, name string) error
	DeleteUser(ctx context.Context, id int64) error
}

// RecurringRuleRepository stores the rules that mint expenses on a schedule.
// The sole cross-aggregate operation is DeleteRule: preserving history while
// removing the Pending Charge must be one storage transaction.
type RecurringRuleRepository interface {
	CreateRule(ctx context.Context, r domain.RecurringRule) (int64, error)
	GetRule(ctx context.Context, id int64) (*domain.RecurringRule, error)
	// ListRules returns every rule, enabled or not, soonest charge first.
	ListRules(ctx context.Context) ([]domain.RecurringRule, error)
	UpdateRule(ctx context.Context, r domain.RecurringRule) error
	DeleteRule(ctx context.Context, id int64, today string) error
}

// Repository is the full composition of all domain repositories.
// Useful when a single concrete implementation (e.g. SQLite) satisfies everything.
type Repository interface {
	ExpenseRepository
	PreferenceRepository
	RateRepository
	UserRepository
	RecurringRuleRepository
}

// Repos bundles the focused repository interfaces for dependency injection.
type Repos struct {
	Expenses    ExpenseRepository
	Users       UserRepository
	Preferences PreferenceRepository
	Rates       RateRepository
	Rules       RecurringRuleRepository
}
