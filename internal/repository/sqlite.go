package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"expensif/internal/domain"
)

type sqliteRepo struct {
	db *sql.DB
}

func NewSQLite(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// nullableID maps an unset (zero) foreign key to SQL NULL, so "no payer" and "no
// rule" are stored as absence rather than as a reference to row 0.
func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}

// expenseColumns is the one column list every expense read shares, so an expense
// scanned by GetExpense and one scanned by ListExpenses are populated
// identically — and a column added to the table is added here once.
const expenseColumns = `e.id, e.amount, e.category, e.description, e.date, e.currency, e.paid_by, u.name, e.recurring_rule_id, e.created_at`

// scanExpense reads one expense row in expenseColumns order.
func scanExpense(scan func(dest ...any) error) (domain.Expense, error) {
	var e domain.Expense
	var paidBy, ruleID sql.NullInt64
	var paidByName sql.NullString
	var createdAt string
	if err := scan(&e.ID, &e.Amount, &e.Category, &e.Description, &e.Date, &e.Currency,
		&paidBy, &paidByName, &ruleID, &createdAt); err != nil {
		return domain.Expense{}, err
	}
	if paidBy.Valid {
		e.PaidByID = paidBy.Int64
	}
	if paidByName.Valid {
		e.PaidByName = paidByName.String
	}
	if ruleID.Valid {
		e.RecurringRuleID = ruleID.Int64
	}
	e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return e, nil
}

func (r *sqliteRepo) CreateExpense(ctx context.Context, e domain.Expense) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO expenses (amount, category, description, date, currency, paid_by, recurring_rule_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Amount, e.Category, e.Description, e.Date, e.Currency, nullableID(e.PaidByID), nullableID(e.RecurringRuleID),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MintCharge inserts a rule's charge, letting the partial unique index on
// (recurring_rule_id, date) decide whether it is new. A date the rule has
// already charged conflicts and the insert does nothing, so a repeated
// generation run neither duplicates the charge nor overwrites one a human has
// since edited; the boolean is how the caller counts what it actually minted.
func (r *sqliteRepo) MintCharge(ctx context.Context, e domain.Expense) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO expenses (amount, category, description, date, currency, paid_by, recurring_rule_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (recurring_rule_id, date) WHERE recurring_rule_id IS NOT NULL DO NOTHING`,
		e.Amount, e.Category, e.Description, e.Date, e.Currency, nullableID(e.PaidByID), nullableID(e.RecurringRuleID),
	)
	if err != nil {
		return false, fmt.Errorf("mint charge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// pendingChargePredicate is the storage definition of Rule-owned future: a
// linked Expense strictly after today. Keeping the boundary in one place makes
// due-today history impossible to rewrite or delete by accident.
const pendingChargePredicate = `recurring_rule_id = ? AND date > ?`

func (r *sqliteRepo) RewritePendingCharge(ctx context.Context, ruleID int64, today string, e domain.Expense) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE expenses SET
			amount = ?, category = ?, description = ?, date = ?, currency = ?, paid_by = ?
		WHERE id = (
			SELECT id FROM expenses
			WHERE `+pendingChargePredicate+`
			ORDER BY date ASC
			LIMIT 1
		)`,
		e.Amount, e.Category, e.Description, e.Date, e.Currency, nullableID(e.PaidByID), ruleID, today,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *sqliteRepo) DeletePendingCharge(ctx context.Context, ruleID int64, today string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM expenses WHERE `+pendingChargePredicate, ruleID, today)
	return err
}

func (r *sqliteRepo) ListExpenses(ctx context.Context, limit int) ([]domain.Expense, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+expenseColumns+`
		 FROM expenses e
		 LEFT JOIN users u ON e.paid_by = u.id
		 ORDER BY e.date DESC, e.created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expenses []domain.Expense
	for rows.Next() {
		e, err := scanExpense(rows.Scan)
		if err != nil {
			return nil, err
		}
		expenses = append(expenses, e)
	}
	return expenses, rows.Err()
}

// ListExpensesInRange is the whole-range case of ExpensesMatching, kept as a
// named seam for daily-timeline's callers.
func (r *sqliteRepo) ListExpensesInRange(ctx context.Context, start, end string) ([]domain.Expense, error) {
	return r.ExpensesMatching(ctx, domain.ExpenseFilter{Start: start, End: end})
}

// filterWhere builds the WHERE clause and args for an ExpenseFilter. The date
// range is always present; category and payer are appended only when set, so a
// zero-value field means "all". It is the single home of the filter→SQL mapping,
// so every reader of the filter (the download here, the preview count and sample
// later) narrows identically — the preview count equals the download by
// construction.
func filterWhere(f domain.ExpenseFilter) (string, []any) {
	clauses := []string{"e.date BETWEEN ? AND ?"}
	args := []any{f.Start, f.End}
	if f.Category != "" {
		clauses = append(clauses, "e.category = ?")
		args = append(args, f.Category)
	}
	if f.PayerID != 0 {
		clauses = append(clauses, "e.paid_by = ?")
		args = append(args, f.PayerID)
	}
	return strings.Join(clauses, " AND "), args
}

// ExpensesMatching returns the raw expenses matching the filter, newest first.
// ConvertedAmount is not populated — conversion is the handler's job — but
// PaidByName rides the existing LEFT JOIN.
func (r *sqliteRepo) ExpensesMatching(ctx context.Context, f domain.ExpenseFilter) ([]domain.Expense, error) {
	return r.queryExpenses(ctx, f, 0)
}

// SampleMatching returns the first limit matching expenses in the same order as
// ExpensesMatching, so the preview shows the genuine head of the download.
func (r *sqliteRepo) SampleMatching(ctx context.Context, f domain.ExpenseFilter, limit int) ([]domain.Expense, error) {
	return r.queryExpenses(ctx, f, limit)
}

// queryExpenses is the shared SELECT+scan behind ExpensesMatching and
// SampleMatching: one filter→WHERE mapping, one column list, one ORDER BY, with
// an optional LIMIT (limit <= 0 = unbounded) — so the sample is always a genuine
// prefix of the full download.
func (r *sqliteRepo) queryExpenses(ctx context.Context, f domain.ExpenseFilter, limit int) ([]domain.Expense, error) {
	where, args := filterWhere(f)
	query := `SELECT ` + expenseColumns + `
		 FROM expenses e
		 LEFT JOIN users u ON e.paid_by = u.id
		 WHERE ` + where + `
		 ORDER BY e.date DESC, e.created_at DESC`
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expenses []domain.Expense
	for rows.Next() {
		e, err := scanExpense(rows.Scan)
		if err != nil {
			return nil, err
		}
		expenses = append(expenses, e)
	}
	return expenses, rows.Err()
}

// CountMatching counts the expenses matching the filter, sharing filterWhere with
// ExpensesMatching so the preview count and the download's row count are equal by
// construction. No join is needed — the payer predicate is on e.paid_by.
func (r *sqliteRepo) CountMatching(ctx context.Context, f domain.ExpenseFilter) (int, error) {
	where, args := filterWhere(f)
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM expenses e WHERE `+where, args...,
	).Scan(&n)
	return n, err
}

// DistinctCategories returns every category the account has used, sorted, for the
// Export page's dropdown. Distinct from ListCategories, which is a recency- and
// frequency-ranked shortlist for the add form; here the export wants the full set.
func (r *sqliteRepo) DistinctCategories(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT category FROM expenses WHERE category != '' ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

func (r *sqliteRepo) GetExpense(ctx context.Context, id int64) (*domain.Expense, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+expenseColumns+`
		 FROM expenses e
		 LEFT JOIN users u ON e.paid_by = u.id
		 WHERE e.id = ?`, id)
	e, err := scanExpense(row.Scan)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no expense with id %d", id)
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// UpdateExpense writes the fields a human can edit. recurring_rule_id is
// deliberately not among them: editing a Pending Charge as an ordinary expense
// must not sever it from the rule that minted it.
func (r *sqliteRepo) UpdateExpense(ctx context.Context, e domain.Expense) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE expenses SET amount = ?, category = ?, description = ?, date = ?, currency = ?, paid_by = ? WHERE id = ?`,
		e.Amount, e.Category, e.Description, e.Date, e.Currency, nullableID(e.PaidByID), e.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no expense with id %d", e.ID)
	}
	return nil
}

func (r *sqliteRepo) DeleteExpense(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM expenses WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no expense with id %d", id)
	}
	return nil
}

func (r *sqliteRepo) ListCategories(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT category FROM expenses
		WHERE date >= date('now', '-3 months')
		GROUP BY category
		ORDER BY COUNT(*) DESC, category ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

func (r *sqliteRepo) ListDescriptionsByCategory(ctx context.Context, category string, limit int) ([]domain.DescriptionCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT description, COUNT(*) as c
		FROM expenses
		WHERE LOWER(category) = LOWER(?) AND description != ''
		GROUP BY LOWER(description)
		ORDER BY c DESC
		LIMIT ?
	`, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.DescriptionCount
	for rows.Next() {
		var dc domain.DescriptionCount
		if err := rows.Scan(&dc.Description, &dc.Count); err != nil {
			return nil, err
		}
		results = append(results, dc)
	}
	return results, rows.Err()
}

func (r *sqliteRepo) SummaryByCategory(ctx context.Context) (map[string]float64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT category, SUM(amount) FROM expenses GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]float64)
	for rows.Next() {
		var cat string
		var sum float64
		if err := rows.Scan(&cat, &sum); err != nil {
			return nil, err
		}
		m[cat] = sum
	}
	return m, rows.Err()
}

func (r *sqliteRepo) GetEarliestExpenseDate(ctx context.Context) (string, error) {
	var date sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT MIN(date) FROM expenses`).Scan(&date)
	if err != nil {
		return "", err
	}
	if !date.Valid {
		return "", nil
	}
	return date.String, nil
}

func (r *sqliteRepo) TotalExpenses(ctx context.Context) (float64, error) {
	var total sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `SELECT SUM(amount) FROM expenses`).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}

func (r *sqliteRepo) GetPreferences(ctx context.Context) (*domain.Preferences, error) {
	var p domain.Preferences
	var userID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT currency, user_id, timezone FROM preferences WHERE id = 1`).Scan(&p.Currency, &userID, &p.Timezone)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("get preferences: %w", err)
	}
	if userID.Valid {
		p.UserID = userID.Int64
	}
	return &p, nil
}

func (r *sqliteRepo) SavePreferences(ctx context.Context, p domain.Preferences) error {
	var userID interface{}
	if p.UserID != 0 {
		userID = p.UserID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO preferences (id, currency, user_id, timezone) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET currency = excluded.currency, user_id = excluded.user_id, timezone = excluded.timezone
	`, p.Currency, userID, p.Timezone)
	if err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}

func (r *sqliteRepo) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *sqliteRepo) CreateUser(ctx context.Context, name string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO users (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return res.LastInsertId()
}

func (r *sqliteRepo) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx, `SELECT id, name FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Name)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no user with id %d", id)
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *sqliteRepo) UpdateUser(ctx context.Context, id int64, name string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no user with id %d", id)
	}
	return nil
}

func (r *sqliteRepo) DeleteUser(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no user with id %d", id)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE expenses SET paid_by = NULL WHERE paid_by = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

// --- Recurring Rules ---

// ruleColumns is the one column list every rule read shares, so a rule scanned
// by GetRule and one scanned by ListRules are populated identically. The payer's
// name rides the same LEFT JOIN the expense reads use.
const ruleColumns = `r.id, r.name, r.amount, r.currency, r.category, r.paid_by, u.name,
	r.interval_unit, r.interval_count, r.anchor_day, r.next_charge_date, r.end_date, r.enabled, r.created_at`

// scanRule reads one rule row in ruleColumns order.
func scanRule(scan func(dest ...any) error) (domain.RecurringRule, error) {
	var r domain.RecurringRule
	var paidBy sql.NullInt64
	var paidByName, endDate sql.NullString
	var createdAt string
	err := scan(&r.ID, &r.Name, &r.Amount, &r.Currency, &r.Category, &paidBy, &paidByName,
		&r.IntervalUnit, &r.IntervalCount, &r.AnchorDay, &r.NextChargeDate, &endDate, &r.Enabled, &createdAt)
	if err != nil {
		return domain.RecurringRule{}, err
	}
	if paidBy.Valid {
		r.PaidByID = paidBy.Int64
	}
	if paidByName.Valid {
		r.PaidByName = paidByName.String
	}
	if endDate.Valid {
		r.EndDate = endDate.String
	}
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return r, nil
}

func (r *sqliteRepo) CreateRule(ctx context.Context, rule domain.RecurringRule) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO recurring_rules
			(name, amount, currency, category, paid_by, interval_unit, interval_count, anchor_day, next_charge_date, end_date, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Name, rule.Amount, rule.Currency, rule.Category, nullableID(rule.PaidByID),
		string(rule.IntervalUnit), rule.IntervalCount, rule.AnchorDay, rule.NextChargeDate,
		nullableDate(rule.EndDate), rule.Enabled,
	)
	if err != nil {
		return 0, fmt.Errorf("create recurring rule: %w", err)
	}
	return res.LastInsertId()
}

// nullableDate maps an unset optional date to SQL NULL, so an open-ended rule
// stores absence rather than an empty string.
func nullableDate(date string) interface{} {
	if date == "" {
		return nil
	}
	return date
}

func (r *sqliteRepo) GetRule(ctx context.Context, id int64) (*domain.RecurringRule, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+ruleColumns+`
		FROM recurring_rules r
		LEFT JOIN users u ON r.paid_by = u.id
		WHERE r.id = ?`, id)
	rule, err := scanRule(row.Scan)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no recurring rule with id %d", id)
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListRules returns every rule, enabled or not, soonest charge first — the order
// the Recurring page reads in — with ties broken by name so the list is stable.
func (r *sqliteRepo) ListRules(ctx context.Context) ([]domain.RecurringRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+ruleColumns+`
		FROM recurring_rules r
		LEFT JOIN users u ON r.paid_by = u.id
		ORDER BY r.next_charge_date ASC, r.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []domain.RecurringRule
	for rows.Next() {
		rule, err := scanRule(rows.Scan)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *sqliteRepo) UpdateRule(ctx context.Context, rule domain.RecurringRule) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE recurring_rules SET
			name = ?, amount = ?, currency = ?, category = ?, paid_by = ?,
			interval_unit = ?, interval_count = ?, anchor_day = ?,
			next_charge_date = ?, end_date = ?, enabled = ?
		WHERE id = ?`,
		rule.Name, rule.Amount, rule.Currency, rule.Category, nullableID(rule.PaidByID),
		string(rule.IntervalUnit), rule.IntervalCount, rule.AnchorDay, rule.NextChargeDate,
		nullableDate(rule.EndDate), rule.Enabled, rule.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no recurring rule with id %d", rule.ID)
	}
	return nil
}

// DeleteRule follows user deletion's transaction shape: the Rule's future is
// removed, its history is detached into ordinary Expenses, and the Rule itself
// is dropped as one indivisible operation.
func (r *sqliteRepo) DeleteRule(ctx context.Context, id int64, today string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM expenses WHERE `+pendingChargePredicate, id, today); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE expenses SET recurring_rule_id = NULL WHERE recurring_rule_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM recurring_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no recurring rule with id %d", id)
	}
	return tx.Commit()
}

// --- Exchange Rates ---

func (r *sqliteRepo) SaveRates(ctx context.Context, base string, date string, rates map[string]float64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO exchange_rates (base_currency, target_currency, rate, date)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for target, rate := range rates {
		if _, err := stmt.ExecContext(ctx, base, target, rate, date); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *sqliteRepo) GetRates(ctx context.Context, base string, date string) (map[string]float64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_currency, rate FROM exchange_rates WHERE base_currency = ? AND date = ?`,
		base, date,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rates := make(map[string]float64)
	for rows.Next() {
		var target string
		var rate float64
		if err := rows.Scan(&target, &rate); err != nil {
			return nil, err
		}
		rates[target] = rate
	}
	return rates, rows.Err()
}

func (r *sqliteRepo) RatesOnOrBefore(ctx context.Context, base string, date string) (map[string]float64, string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT target_currency, rate, date
		FROM exchange_rates
		WHERE base_currency = ?
		  AND date = (
		    SELECT MAX(date) FROM exchange_rates
		    WHERE base_currency = ? AND date <= ?
		  )
	`, base, base, date)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	rates := make(map[string]float64)
	var matched string
	for rows.Next() {
		var target string
		var rate float64
		if err := rows.Scan(&target, &rate, &matched); err != nil {
			return nil, "", err
		}
		rates[target] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(rates) == 0 {
		return nil, "", sql.ErrNoRows
	}
	return rates, matched, nil
}

func (r *sqliteRepo) RateDatesInRange(ctx context.Context, base string, from, to string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT date FROM exchange_rates
		WHERE base_currency = ? AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, base, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

func (r *sqliteRepo) GetLatestRates(ctx context.Context, base string) (map[string]float64, string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT target_currency, rate, date
		FROM exchange_rates
		WHERE base_currency = ?
		  AND date = (SELECT MAX(date) FROM exchange_rates WHERE base_currency = ?)
	`, base, base)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	rates := make(map[string]float64)
	var date string
	for rows.Next() {
		var target string
		var rate float64
		if err := rows.Scan(&target, &rate, &date); err != nil {
			return nil, "", err
		}
		rates[target] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(rates) == 0 {
		return nil, "", sql.ErrNoRows
	}
	return rates, date, nil
}
