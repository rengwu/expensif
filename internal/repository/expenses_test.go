package repository

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"expensif/internal/db"
	"expensif/internal/domain"
)

func newTestRepo(t *testing.T) (Repository, context.Context) {
	t.Helper()
	sqldb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewSQLite(sqldb), context.Background()
}

// TestCountMatchingAgreesWithExpensesMatching proves the by-construction
// guarantee at the SQL seam: because CountMatching and ExpensesMatching share
// filterWhere, the preview count equals the download's row count for every
// filter — the reassurance the preview promises isn't a lie.
func TestCountMatchingAgreesWithExpensesMatching(t *testing.T) {
	repo, ctx := newTestRepo(t)
	alice, _ := repo.CreateUser(ctx, "Alice")
	bob, _ := repo.CreateUser(ctx, "Bob")
	seed := []domain.Expense{
		{Amount: 1, Category: "food", Date: "2026-01-01", Currency: "USD", PaidByID: alice},
		{Amount: 2, Category: "travel", Date: "2026-01-02", Currency: "USD", PaidByID: alice},
		{Amount: 3, Category: "food", Date: "2026-01-03", Currency: "USD", PaidByID: bob},
		{Amount: 4, Category: "food", Date: "2026-02-01", Currency: "USD", PaidByID: bob},
	}
	for _, e := range seed {
		if _, err := repo.CreateExpense(ctx, e); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	filters := []domain.ExpenseFilter{
		{Start: "2026-01-01", End: "2026-12-31"},
		{Start: "2026-01-01", End: "2026-12-31", Category: "food"},
		{Start: "2026-01-01", End: "2026-12-31", PayerID: alice},
		{Start: "2026-01-01", End: "2026-12-31", Category: "food", PayerID: bob},
		{Start: "2026-01-01", End: "2026-01-31"},
		{Start: "2030-01-01", End: "2030-12-31"}, // matches nothing
	}
	for _, f := range filters {
		count, err := repo.CountMatching(ctx, f)
		if err != nil {
			t.Fatalf("CountMatching(%+v): %v", f, err)
		}
		rows, err := repo.ExpensesMatching(ctx, f)
		if err != nil {
			t.Fatalf("ExpensesMatching(%+v): %v", f, err)
		}
		if count != len(rows) {
			t.Fatalf("filter %+v: count %d != len(rows) %d", f, count, len(rows))
		}
	}
}

// TestSampleMatchingIsAPrefix proves the sample is the genuine head of the
// download: ordered identically, and truncated to the limit.
func TestSampleMatchingIsAPrefix(t *testing.T) {
	repo, ctx := newTestRepo(t)
	for _, d := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		if _, err := repo.CreateExpense(ctx, domain.Expense{Amount: 1, Category: "food", Date: d, Currency: "USD"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	f := domain.ExpenseFilter{Start: "2026-01-01", End: "2026-01-31"}
	full, err := repo.ExpensesMatching(ctx, f)
	if err != nil {
		t.Fatalf("ExpensesMatching: %v", err)
	}
	sample, err := repo.SampleMatching(ctx, f, 2)
	if err != nil {
		t.Fatalf("SampleMatching: %v", err)
	}
	if len(sample) != 2 {
		t.Fatalf("sample size = %d, want 2 (limit)", len(sample))
	}
	for i := range sample {
		if sample[i].ID != full[i].ID {
			t.Fatalf("sample[%d].ID = %d, not a prefix of the download (%d)", i, sample[i].ID, full[i].ID)
		}
	}
}

// TestDistinctCategories returns each used category once, sorted, ignoring blanks.
func TestDistinctCategories(t *testing.T) {
	repo, ctx := newTestRepo(t)
	for _, c := range []string{"travel", "food", "food", "", "office"} {
		if _, err := repo.CreateExpense(ctx, domain.Expense{Amount: 1, Category: c, Date: "2026-01-01", Currency: "USD"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := repo.DistinctCategories(ctx)
	if err != nil {
		t.Fatalf("DistinctCategories: %v", err)
	}
	want := []string{"food", "office", "travel"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("categories = %v, want %v", got, want)
	}
}
