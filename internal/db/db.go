package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func New() (*sql.DB, error) {
	dbDir := os.Getenv("DATA_DIR")
	if dbDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		dbDir = filepath.Join(home, ".expensif")
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dbPath := filepath.Join(dbDir, "expenses.db")

	db, err := sql.Open("sqlite", "file:"+dbPath+"?_fk=1")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Migrate creates the schema on a fresh database. Exported so tests can spin up
// an in-memory store with the real tables rather than a hand-copied subset.
func Migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS expenses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		amount REAL NOT NULL,
		category TEXT NOT NULL,
		description TEXT,
		date TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses(category);
	CREATE INDEX IF NOT EXISTS idx_expenses_created_at ON expenses(created_at);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	var hasDate int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expenses') WHERE name = 'date'`).Scan(&hasDate); err != nil {
		return err
	}
	if hasDate == 0 {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN date TEXT`); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE expenses SET date = date('now') WHERE date IS NULL`); err != nil {
			return err
		}
	}

	var hasCurrency int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expenses') WHERE name = 'currency'`).Scan(&hasCurrency); err != nil {
		return err
	}
	if hasCurrency == 0 {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN currency TEXT DEFAULT 'USD'`); err != nil {
			return err
		}
	}

	var hasPaidBy int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expenses') WHERE name = 'paid_by'`).Scan(&hasPaidBy); err != nil {
		return err
	}
	if hasPaidBy == 0 {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN paid_by INTEGER`); err != nil {
			return err
		}
	} else {
		// Column exists — ensure it's INTEGER (not old TEXT)
		var isInt int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expenses') WHERE name = 'paid_by' AND type = 'INTEGER'`).Scan(&isInt); err != nil {
			return err
		}
		if isInt == 0 {
			if _, err := db.Exec(`
				CREATE TABLE expenses_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					amount REAL NOT NULL,
					category TEXT NOT NULL,
					description TEXT,
					date TEXT,
					currency TEXT DEFAULT 'USD',
					paid_by INTEGER,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				INSERT INTO expenses_new (id, amount, category, description, date, currency, paid_by, created_at)
					SELECT id, amount, category, description, date, currency, paid_by, created_at FROM expenses;
				DROP TABLE expenses;
				ALTER TABLE expenses_new RENAME TO expenses;
				CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses(category);
				CREATE INDEX IF NOT EXISTS idx_expenses_created_at ON expenses(created_at);
			`); err != nil {
				return fmt.Errorf("migrate paid_by to integer: %w", err)
			}
		}
	}

	prefsSchema := `
	CREATE TABLE IF NOT EXISTS preferences (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		currency TEXT DEFAULT 'USD',
		user_id INTEGER
	);
	INSERT OR IGNORE INTO preferences (id, currency) VALUES (1, 'USD');
	`
	if _, err := db.Exec(prefsSchema); err != nil {
		return err
	}

	var hasUserID int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('preferences') WHERE name = 'user_id'`).Scan(&hasUserID); err != nil {
		return err
	}
	if hasUserID == 0 {
		if _, err := db.Exec(`ALTER TABLE preferences ADD COLUMN user_id INTEGER`); err != nil {
			return err
		}
	}

	var hasTimezone int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('preferences') WHERE name = 'timezone'`).Scan(&hasTimezone); err != nil {
		return err
	}
	if hasTimezone == 0 {
		if _, err := db.Exec(`ALTER TABLE preferences ADD COLUMN timezone TEXT DEFAULT ''`); err != nil {
			return err
		}
	}

	usersSchema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	);
	`
	if _, err := db.Exec(usersSchema); err != nil {
		return err
	}

	ratesSchema := `
	CREATE TABLE IF NOT EXISTS exchange_rates (
		base_currency TEXT NOT NULL,
		target_currency TEXT NOT NULL,
		rate REAL NOT NULL,
		date TEXT NOT NULL,
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (base_currency, target_currency, date)
	);
	`
	if _, err := db.Exec(ratesSchema); err != nil {
		return err
	}

	rulesSchema := `
	CREATE TABLE IF NOT EXISTS recurring_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		amount REAL NOT NULL,
		currency TEXT NOT NULL DEFAULT 'USD',
		category TEXT NOT NULL,
		paid_by INTEGER,
		interval_unit TEXT NOT NULL,
		interval_count INTEGER NOT NULL,
		anchor_day INTEGER NOT NULL,
		next_charge_date TEXT NOT NULL,
		end_date TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_rules_next_charge ON recurring_rules(enabled, next_charge_date);
	`
	if _, err := db.Exec(rulesSchema); err != nil {
		return err
	}

	var hasRuleRef int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expenses') WHERE name = 'recurring_rule_id'`).Scan(&hasRuleRef); err != nil {
		return err
	}
	if hasRuleRef == 0 {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN recurring_rule_id INTEGER`); err != nil {
			return err
		}
	}

	// One rule can never legitimately owe two charges on the same date — the
	// minimum interval is one day — so this makes double-generation impossible
	// by construction rather than merely unlikely. Partial, so hand-entered
	// expenses (which carry no rule) stay entirely unconstrained.
	if _, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_expenses_rule_date
		ON expenses(recurring_rule_id, date)
		WHERE recurring_rule_id IS NOT NULL
	`); err != nil {
		return err
	}

	slog.Info("database migrations completed")
	return nil
}
