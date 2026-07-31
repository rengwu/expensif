package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"expensif/internal/domain"
)

// --- Recurring Rules ---

// NextChargeDate answers "when does this rule charge next, after this date?" —
// the only scheduling primitive in the effort. The date to advance from is an
// argument, never a clock read, because charge dates are evaluated in the user's
// configured Timezone and only the caller knows what "today" means there.
//
// The result is always strictly after `after`: a monthly rule lands in a later
// month, and week/day intervals are at least one day long.
func (s *Service) NextChargeDate(rule domain.RecurringRule, after string) (string, error) {
	from, err := time.Parse("2006-01-02", after)
	if err != nil {
		return "", ErrInvalidDate
	}
	if err := validateSchedule(rule); err != nil {
		return "", err
	}
	return nextCharge(from, rule).Format("2006-01-02"), nil
}

// nextCharge is the anchor arithmetic, and the single most important function in
// this effort. For months it takes only the *month* from the date it advances
// from and takes the *day* from the stored anchor, clamped to the length of the
// target month. Nothing is ever accumulated from a previously clamped date, so a
// rule anchored on the 31st runs Jan 31 → Feb 28 → Mar 31 → Apr 30 and February
// never poisons the series.
//
// Weeks and days need no calendar reasoning at all: they are plain multiples of
// 24 hours, and having no clamping they have nothing to drift from.
func nextCharge(from time.Time, rule domain.RecurringRule) time.Time {
	return stepCharge(from, rule, 1)
}

// stepCharge is the one interval dispatch for moving a schedule in either
// direction. Recomputing an edited Pending Charge needs the prior occurrence;
// keeping both directions here prevents the reverse path from drifting away
// from the generation arithmetic above.
func stepCharge(from time.Time, rule domain.RecurringRule, direction int) time.Time {
	switch rule.IntervalUnit {
	case domain.IntervalWeeks:
		return from.AddDate(0, 0, direction*7*rule.IntervalCount)
	case domain.IntervalDays:
		return from.AddDate(0, 0, direction*rule.IntervalCount)
	default: // domain.IntervalMonths
		year, month := from.Year(), int(from.Month())+direction*rule.IntervalCount
		for month > 12 {
			year++
			month -= 12
		}
		for month < 1 {
			year--
			month += 12
		}
		return time.Date(year, time.Month(month), clampDay(year, month, rule.AnchorDay), 0, 0, 0, 0, time.UTC)
	}
}

// clampDay is min(anchor day, days in that month) — the 31st becoming the 28th
// in an ordinary February and the 29th in a leap one.
func clampDay(year, month, anchorDay int) int {
	if last := daysInMonth(year, month); anchorDay > last {
		return last
	}
	return anchorDay
}

// daysInMonth reads day zero of the following month, which normalizes to the
// last day of this one — including across a year boundary.
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// validateSchedule checks the three fields the arithmetic depends on. The anchor
// day only means something for months; weeks and days ignore it.
func validateSchedule(rule domain.RecurringRule) error {
	switch rule.IntervalUnit {
	case domain.IntervalMonths, domain.IntervalWeeks, domain.IntervalDays:
	default:
		return ErrInvalidIntervalUnit
	}
	if rule.IntervalCount < 1 {
		return ErrInvalidIntervalCount
	}
	if rule.IntervalUnit == domain.IntervalMonths && (rule.AnchorDay < 1 || rule.AnchorDay > 31) {
		return ErrInvalidAnchorDay
	}
	return nil
}

// normalizeRule validates a rule and fills in what the caller left off, mirroring
// validateExpenseInput's shape: amount, category and (here) name are required,
// and the currency falls back to the Preferred Currency. The payer is
// deliberately *not* defaulted — it is optional on a rule, so an unset payer
// stays unset rather than inheriting the Preferences one behind the caller's
// back; the add form pre-selects the default payer in its handler, and the
// Recurring form is expected to do the same.
func (s *Service) normalizeRule(ctx context.Context, rule domain.RecurringRule) (domain.RecurringRule, error) {
	if rule.Amount <= 0 {
		return rule, ErrInvalidAmount
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Category = strings.TrimSpace(rule.Category)
	if rule.Name == "" {
		return rule, ErrMissingName
	}
	if rule.Category == "" {
		return rule, ErrMissingCategory
	}

	next, err := time.Parse("2006-01-02", rule.NextChargeDate)
	if err != nil {
		return rule, ErrInvalidDate
	}
	// A rule created from a start date alone anchors on that date's day, which
	// is what a form asking only "when does it first charge?" implies.
	if rule.AnchorDay == 0 {
		rule.AnchorDay = next.Day()
	}
	if err := validateSchedule(rule); err != nil {
		return rule, err
	}

	if rule.EndDate != "" {
		if _, err := time.Parse("2006-01-02", rule.EndDate); err != nil {
			return rule, ErrInvalidDate
		}
	}

	if rule.Currency == "" {
		prefs, err := s.Preferences(ctx)
		if err != nil {
			return rule, err
		}
		rule.Currency = prefs.Currency
	}
	return rule, nil
}

func (s *Service) CreateRule(ctx context.Context, rule domain.RecurringRule) (int64, error) {
	rule, err := s.normalizeRule(ctx, rule)
	if err != nil {
		return 0, err
	}
	// An end date on the first charge is the degenerate one-charge rule and is
	// allowed; one before it could never charge at all. Edits do not use this
	// creation-only guard: moving an existing Rule's end before its Pending
	// Charge disables it below instead.
	if rule.EndDate != "" && rule.EndDate < rule.NextChargeDate {
		return 0, ErrInvalidRange
	}
	return s.rules.CreateRule(ctx, rule)
}

func (s *Service) GetRule(ctx context.Context, id int64) (*domain.RecurringRule, error) {
	return s.rules.GetRule(ctx, id)
}

// ListRules returns every rule, enabled or not — the Recurring page shows both.
func (s *Service) ListRules(ctx context.Context) ([]domain.RecurringRule, error) {
	return s.rules.ListRules(ctx)
}

func (s *Service) UpdateRule(ctx context.Context, rule domain.RecurringRule, today string) error {
	todayDate, err := time.Parse("2006-01-02", today)
	if err != nil {
		return ErrInvalidDate
	}
	existing, err := s.rules.GetRule(ctx, rule.ID)
	if err != nil {
		return err
	}

	// The stored date names the Pending Charge. It is owned by the scheduling
	// operation below, not trusted from a submitted edit form.
	rule.NextChargeDate = existing.NextChargeDate
	rule, err = s.normalizeRule(ctx, rule)
	if err != nil {
		return err
	}

	scheduleChanged := rule.IntervalUnit != existing.IntervalUnit ||
		rule.IntervalCount != existing.IntervalCount || rule.AnchorDay != existing.AnchorDay
	switch {
	case rule.Enabled && !existing.Enabled:
		rule.NextChargeDate, err = firstChargeOnOrAfter(rule, existing.NextChargeDate, todayDate)
	case rule.Enabled && scheduleChanged:
		rule.NextChargeDate, err = recomputePendingCharge(*existing, rule, todayDate)
	}
	if err != nil {
		return err
	}
	if rule.Enabled && rule.EndDate != "" && rule.NextChargeDate > rule.EndDate {
		rule.Enabled = false
	}

	if rule.Enabled {
		charge := chargeFor(rule)
		rewritten, err := s.expenses.RewritePendingCharge(ctx, rule.ID, today, charge)
		if err != nil {
			return err
		}
		if !rewritten {
			if _, err := s.expenses.MintCharge(ctx, charge); err != nil {
				return err
			}
		}
	} else if err := s.expenses.DeletePendingCharge(ctx, rule.ID, today); err != nil {
		return err
	}
	return s.rules.UpdateRule(ctx, rule)
}

// SetRuleEnabled applies the same ownership path as an edit. Disabling removes
// the Pending Charge; re-enabling advances the stopped schedule to its first
// occurrence on or after today before minting one charge, so the paused stretch
// is never mistaken for an outage that needs Catch-up.
func (s *Service) SetRuleEnabled(ctx context.Context, id int64, enabled bool, today string) error {
	rule, err := s.rules.GetRule(ctx, id)
	if err != nil {
		return err
	}
	rule.Enabled = enabled
	return s.UpdateRule(ctx, *rule, today)
}

func (s *Service) DeleteRule(ctx context.Context, id int64, today string) error {
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return ErrInvalidDate
	}
	return s.rules.DeleteRule(ctx, id, today)
}

// firstChargeOnOrAfter advances a stopped schedule without minting anything it
// stepped over. The stale stored occurrence is the phase anchor for week/day
// schedules and the stored Anchor Day is reapplied for month schedules.
func firstChargeOnOrAfter(rule domain.RecurringRule, from string, today time.Time) (string, error) {
	date, err := time.Parse("2006-01-02", from)
	if err != nil {
		return "", ErrInvalidDate
	}
	if err := validateSchedule(rule); err != nil {
		return "", err
	}
	if rule.IntervalUnit == domain.IntervalMonths {
		date = time.Date(date.Year(), date.Month(), clampDay(date.Year(), int(date.Month()), rule.AnchorDay), 0, 0, 0, 0, time.UTC)
	}
	for date.Before(today) {
		date = nextCharge(date, rule)
	}
	return date.Format("2006-01-02"), nil
}

// recomputePendingCharge treats the occurrence before the existing Pending
// Charge as the fixed point, then applies the edited schedule from there. This
// lets interval, count and Anchor Day edits move the future without ever
// deriving a new occurrence from a clamped day or touching history.
func recomputePendingCharge(existing, edited domain.RecurringRule, today time.Time) (string, error) {
	pending, err := time.Parse("2006-01-02", existing.NextChargeDate)
	if err != nil {
		return "", ErrInvalidDate
	}
	previous := previousCharge(pending, existing)
	candidate := nextCharge(previous, edited)
	for candidate.Before(today) {
		candidate = nextCharge(candidate, edited)
	}
	return candidate.Format("2006-01-02"), nil
}

func previousCharge(from time.Time, rule domain.RecurringRule) time.Time {
	return stepCharge(from, rule, -1)
}

// --- Generation ---

// GenerateCharges mints the expenses every enabled rule owes as of `today`, and
// returns how many it created. `today` is the current date in the user's
// configured Timezone and is always an argument, never a clock read: charge
// dates are Timezone-sensitive, the Timezone lives in Preferences, and only the
// caller knows what "today" means there.
//
// Afterwards each enabled rule has exactly one charge dated after `today` — its
// Pending Charge — and every charge it missed while the app was down exists on
// its own true date. Calling it again with the same date creates nothing, so the
// background goroutine may call it as often as it likes.
//
// A rule whose schedule or stored date is unusable is reported rather than
// silently skipped, but it never holds up the others: every rule is attempted
// and the first error is returned alongside the count of what was minted.
func (s *Service) GenerateCharges(ctx context.Context, today string) (int, error) {
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return 0, ErrInvalidDate
	}
	rules, err := s.rules.ListRules(ctx)
	if err != nil {
		return 0, err
	}

	minted := 0
	var firstErr error
	for _, rule := range rules {
		// A disabled rule's clock is stopped: no charge, no catch-up, not even a
		// date advance. Resuming one is the Recurring page's business, not this
		// function's.
		if !rule.Enabled {
			continue
		}
		n, err := s.generateForRule(ctx, rule, today)
		minted += n
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("recurring rule %d (%s): %w", rule.ID, rule.Name, err)
		}
	}
	return minted, firstErr
}

// generateForRule walks one rule forward from its stored next charge date,
// minting each charge on its own true date until it has laid down the one that
// falls after `today` — the Pending Charge — and then stops. Catch-up is not a
// separate mode: a rule that last charged in January simply takes several turns
// of this loop before it reaches a future date.
//
// The walk advances by recomputing from the anchor (see nextCharge), never by
// accumulating onto a clamped date, so a rule anchored on the 31st caught up
// across February lands on Mar 31 rather than drifting to Mar 28.
func (s *Service) generateForRule(ctx context.Context, rule domain.RecurringRule, today string) (int, error) {
	if err := validateSchedule(rule); err != nil {
		return 0, err
	}
	date, err := time.Parse("2006-01-02", rule.NextChargeDate)
	if err != nil {
		return 0, ErrInvalidDate
	}

	// The rule as it is stored, so it is only written back when the walk actually
	// moved it — a rule already one charge ahead is left completely untouched.
	stored := rule
	minted := 0
	for {
		// Past its end date the rule stops scheduling and switches itself off. A
		// charge falling exactly *on* the end date is still owed, so only a date
		// strictly after it ends the rule.
		if rule.EndDate != "" && rule.NextChargeDate > rule.EndDate {
			rule.Enabled = false
			break
		}
		created, err := s.expenses.MintCharge(ctx, chargeFor(rule))
		if err != nil {
			// Leave the rule where it stands: the dates already minted conflict
			// harmlessly on the next run, which then carries on from here.
			return minted, err
		}
		if created {
			minted++
		}
		if rule.NextChargeDate > today {
			break // that was the Pending Charge; one ahead is the whole horizon
		}
		date = nextCharge(date, rule)
		rule.NextChargeDate = date.Format("2006-01-02")
	}

	if rule != stored {
		if err := s.rules.UpdateRule(ctx, rule); err != nil {
			return minted, err
		}
	}
	return minted, nil
}

// chargeFor is the expense a rule mints for its current charge date: an ordinary
// one carrying the rule's amount, currency, category and payer, with the rule's
// Name as the Description. The rule reference is the only thing marking it as
// minted, which is why every existing view reads it without knowing rules exist.
func chargeFor(rule domain.RecurringRule) domain.Expense {
	return domain.Expense{
		Amount:          rule.Amount,
		Category:        rule.Category,
		Description:     rule.Name,
		Date:            rule.NextChargeDate,
		Currency:        rule.Currency,
		PaidByID:        rule.PaidByID,
		RecurringRuleID: rule.ID,
	}
}
