package repository

import (
	"context"
	"database/sql"
	"testing"

	"expensif/internal/db"
)

// TestRatesOnOrBefore_WeekendResolvesToPriorTradingDay is the codebase's first
// repository test: over an in-memory SQLite store it proves the weekend→Friday
// rule at the seam where the SQL lives. A Saturday expense resolves to Friday's
// rate — the nearest stored rate on-or-before its date — and a date earlier than
// anything stored reports sql.ErrNoRows.
func TestRatesOnOrBefore_WeekendResolvesToPriorTradingDay(t *testing.T) {
	sqldb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer sqldb.Close()
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewSQLite(sqldb)
	ctx := context.Background()

	// Friday's rate set is stored; Saturday and Sunday are not (the provider
	// omits non-trading days).
	if err := repo.SaveRates(ctx, "USD", "2026-01-02", map[string]float64{"USD": 1.0, "EUR": 0.9}); err != nil {
		t.Fatalf("save Friday rate: %v", err)
	}

	rates, matched, err := repo.RatesOnOrBefore(ctx, "USD", "2026-01-03") // Saturday
	if err != nil {
		t.Fatalf("RatesOnOrBefore(Saturday): %v", err)
	}
	if matched != "2026-01-02" {
		t.Fatalf("matched date = %q, want 2026-01-02 (the prior Friday)", matched)
	}
	if rates["EUR"] != 0.9 {
		t.Fatalf("EUR rate = %v, want 0.9 from Friday's set", rates["EUR"])
	}

	// Nothing is stored on-or-before an earlier date.
	if _, _, err := repo.RatesOnOrBefore(ctx, "USD", "2026-01-01"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows before any stored rate, got %v", err)
	}
}
