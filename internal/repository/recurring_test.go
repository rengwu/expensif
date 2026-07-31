package repository

import (
	"context"
	"database/sql"
	"testing"

	"expensif/internal/db"
	"expensif/internal/domain"
)

func sampleRule() domain.RecurringRule {
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

func TestRuleCRUDRoundTrip(t *testing.T) {
	repo, ctx := newTestRepo(t)
	alice, err := repo.CreateUser(ctx, "Alice")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	r := sampleRule()
	r.PaidByID = alice
	r.EndDate = "2027-08-15"
	id, err := repo.CreateRule(ctx, r)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	got, err := repo.GetRule(ctx, id)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	want := r
	want.ID = id
	want.PaidByName = "Alice"
	want.CreatedAt = got.CreatedAt
	if *got != want {
		t.Errorf("GetRule returned %+v, want %+v", *got, want)
	}

	got.Amount = 19.99
	got.Name = "Netflix Premium"
	got.Enabled = false
	got.EndDate = ""
	got.PaidByID = 0
	if err := repo.UpdateRule(ctx, *got); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	after, err := repo.GetRule(ctx, id)
	if err != nil {
		t.Fatalf("GetRule after update: %v", err)
	}
	if after.Amount != 19.99 || after.Name != "Netflix Premium" || after.Enabled {
		t.Errorf("update did not round-trip: %+v", *after)
	}
	if after.EndDate != "" || after.PaidByID != 0 || after.PaidByName != "" {
		t.Errorf("cleared optional fields did not round-trip as empty: %+v", *after)
	}

	if err := repo.DeleteRule(ctx, id); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if _, err := repo.GetRule(ctx, id); err == nil {
		t.Error("GetRule succeeded after DeleteRule")
	}
	if err := repo.DeleteRule(ctx, id); err == nil {
		t.Error("DeleteRule of a missing rule succeeded")
	}
}

func TestListRulesOrdersByNextChargeDate(t *testing.T) {
	repo, ctx := newTestRepo(t)
	for _, spec := range []struct{ name, next string }{
		{"Rent", "2026-09-01"},
		{"Netflix", "2026-08-15"},
		{"Gym", "2026-08-15"},
	} {
		r := sampleRule()
		r.Name = spec.name
		r.NextChargeDate = spec.next
		if _, err := repo.CreateRule(ctx, r); err != nil {
			t.Fatalf("CreateRule(%s): %v", spec.name, err)
		}
	}

	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	var names []string
	for _, r := range rules {
		names = append(names, r.Name)
	}
	want := []string{"Gym", "Netflix", "Rent"} // soonest first, then by name
	if len(names) != len(want) {
		t.Fatalf("ListRules returned %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ListRules returned %v, want %v", names, want)
		}
	}
}

// TestRuleDateUniquenessRejectsDuplicates proves the constraint that makes
// double-generation structurally impossible rather than merely unlikely. It is
// safe because the minimum interval is one day, so a rule can never legitimately
// owe two charges on the same date — and it must not constrain hand-entered
// expenses, which carry no rule at all.
func TestRuleDateUniquenessRejectsDuplicates(t *testing.T) {
	repo, ctx := newTestRepo(t)
	ruleID, err := repo.CreateRule(ctx, sampleRule())
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	charge := domain.Expense{
		Amount: 15.99, Category: "subscriptions", Description: "Netflix",
		Date: "2026-08-15", Currency: "USD", RecurringRuleID: ruleID,
	}
	if _, err := repo.CreateExpense(ctx, charge); err != nil {
		t.Fatalf("first charge: %v", err)
	}
	if _, err := repo.CreateExpense(ctx, charge); err == nil {
		t.Fatal("a second charge for the same (rule, date) was accepted")
	}

	// The same rule on a different date is legitimate.
	next := charge
	next.Date = "2026-09-15"
	if _, err := repo.CreateExpense(ctx, next); err != nil {
		t.Fatalf("next month's charge: %v", err)
	}

	// A second rule may charge on the same date as the first.
	otherRule := sampleRule()
	otherRule.Name = "Spotify"
	otherID, err := repo.CreateRule(ctx, otherRule)
	if err != nil {
		t.Fatalf("CreateRule(Spotify): %v", err)
	}
	other := charge
	other.RecurringRuleID = otherID
	other.Description = "Spotify"
	if _, err := repo.CreateExpense(ctx, other); err != nil {
		t.Fatalf("a second rule charging on the same date: %v", err)
	}

	// Hand-entered expenses are unconstrained: two coffees on one day are fine.
	coffee := domain.Expense{Amount: 4, Category: "food", Description: "coffee", Date: "2026-08-15", Currency: "USD"}
	if _, err := repo.CreateExpense(ctx, coffee); err != nil {
		t.Fatalf("first coffee: %v", err)
	}
	if _, err := repo.CreateExpense(ctx, coffee); err != nil {
		t.Fatalf("second coffee on the same day was rejected: %v", err)
	}
}

func TestExpenseCarriesItsRuleReference(t *testing.T) {
	repo, ctx := newTestRepo(t)
	ruleID, err := repo.CreateRule(ctx, sampleRule())
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	minted, err := repo.CreateExpense(ctx, domain.Expense{
		Amount: 15.99, Category: "subscriptions", Description: "Netflix",
		Date: "2026-08-15", Currency: "USD", RecurringRuleID: ruleID,
	})
	if err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}
	handEntered, err := repo.CreateExpense(ctx, domain.Expense{
		Amount: 4, Category: "food", Description: "coffee", Date: "2026-08-15", Currency: "USD",
	})
	if err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}

	got, err := repo.GetExpense(ctx, minted)
	if err != nil {
		t.Fatalf("GetExpense: %v", err)
	}
	if got.RecurringRuleID != ruleID {
		t.Errorf("minted expense has RecurringRuleID %d, want %d", got.RecurringRuleID, ruleID)
	}
	got, err = repo.GetExpense(ctx, handEntered)
	if err != nil {
		t.Fatalf("GetExpense: %v", err)
	}
	if got.RecurringRuleID != 0 {
		t.Errorf("hand-entered expense has RecurringRuleID %d, want 0", got.RecurringRuleID)
	}

	listed, err := repo.ListExpensesInRange(ctx, "2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatalf("ListExpensesInRange: %v", err)
	}
	for _, e := range listed {
		if e.ID == minted && e.RecurringRuleID != ruleID {
			t.Errorf("listed minted expense lost its rule reference: %+v", e)
		}
	}
}

// TestMigrateOverPopulatedDatabase runs the migration over a database shaped as
// it was before this effort, with expenses already in it — the real upgrade
// path. The additive column and the rule table must arrive without disturbing a
// single recorded expense.
func TestMigrateOverPopulatedDatabase(t *testing.T) {
	sqldb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	// The schema as it stood before recurring rules existed.
	legacy := `
	CREATE TABLE expenses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		amount REAL NOT NULL,
		category TEXT NOT NULL,
		description TEXT,
		date TEXT,
		currency TEXT DEFAULT 'USD',
		paid_by INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
	INSERT INTO users (id, name) VALUES (1, 'Alice');
	INSERT INTO expenses (amount, category, description, date, currency, paid_by) VALUES
		(12.50, 'food', 'lunch', '2026-07-01', 'USD', 1),
		(4.00,  'food', 'coffee', '2026-07-01', 'MYR', NULL),
		(900.00,'rent', 'July rent', '2026-07-05', 'USD', 1);
	`
	if _, err := sqldb.Exec(legacy); err != nil {
		t.Fatalf("build legacy schema: %v", err)
	}

	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate over a populated database: %v", err)
	}

	repo, ctx := NewSQLite(sqldb), context.Background()
	expenses, err := repo.ListExpenses(ctx, 100)
	if err != nil {
		t.Fatalf("ListExpenses: %v", err)
	}
	if len(expenses) != 3 {
		t.Fatalf("got %d expenses after migrating, want the 3 that were already there", len(expenses))
	}
	for _, e := range expenses {
		if e.RecurringRuleID != 0 {
			t.Errorf("pre-existing expense %d gained a rule reference: %+v", e.ID, e)
		}
	}
	total, err := repo.TotalExpenses(ctx)
	if err != nil {
		t.Fatalf("TotalExpenses: %v", err)
	}
	if total != 916.50 {
		t.Errorf("total = %v, want the 916.50 that was recorded before migrating", total)
	}

	// The new table works on the upgraded database, and the constraint is live.
	ruleID, err := repo.CreateRule(ctx, sampleRule())
	if err != nil {
		t.Fatalf("CreateRule on an upgraded database: %v", err)
	}
	charge := domain.Expense{
		Amount: 15.99, Category: "subscriptions", Description: "Netflix",
		Date: "2026-08-15", Currency: "USD", RecurringRuleID: ruleID,
	}
	if _, err := repo.CreateExpense(ctx, charge); err != nil {
		t.Fatalf("first charge on an upgraded database: %v", err)
	}
	if _, err := repo.CreateExpense(ctx, charge); err == nil {
		t.Error("the (rule, date) constraint is missing on an upgraded database")
	}

	// Migrating again changes nothing — every startup re-runs this.
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules after re-migrating: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("got %d rules after re-migrating, want 1", len(rules))
	}
	expenses, err = repo.ListExpenses(ctx, 100)
	if err != nil {
		t.Fatalf("ListExpenses after re-migrating: %v", err)
	}
	if len(expenses) != 4 {
		t.Errorf("got %d expenses after re-migrating, want 4", len(expenses))
	}
}
