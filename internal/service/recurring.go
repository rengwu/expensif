package service

import (
	"context"
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
	switch rule.IntervalUnit {
	case domain.IntervalWeeks:
		return from.AddDate(0, 0, 7*rule.IntervalCount)
	case domain.IntervalDays:
		return from.AddDate(0, 0, rule.IntervalCount)
	default: // domain.IntervalMonths
		year, month := from.Year(), int(from.Month())+rule.IntervalCount
		year += (month - 1) / 12
		month = (month-1)%12 + 1
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
		end, err := time.Parse("2006-01-02", rule.EndDate)
		if err != nil {
			return rule, ErrInvalidDate
		}
		// An end date on the first charge is the degenerate one-charge rule and
		// is allowed; one before it could never charge at all.
		if end.Before(next) {
			return rule, ErrInvalidRange
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
	return s.rules.CreateRule(ctx, rule)
}

func (s *Service) GetRule(ctx context.Context, id int64) (*domain.RecurringRule, error) {
	return s.rules.GetRule(ctx, id)
}

// ListRules returns every rule, enabled or not — the Recurring page shows both.
func (s *Service) ListRules(ctx context.Context) ([]domain.RecurringRule, error) {
	return s.rules.ListRules(ctx)
}

func (s *Service) UpdateRule(ctx context.Context, rule domain.RecurringRule) error {
	rule, err := s.normalizeRule(ctx, rule)
	if err != nil {
		return err
	}
	return s.rules.UpdateRule(ctx, rule)
}

// DeleteRule removes the rule itself. The transactional delete that also unlinks
// its past expenses and clears its Pending Charge is a later concern.
func (s *Service) DeleteRule(ctx context.Context, id int64) error {
	return s.rules.DeleteRule(ctx, id)
}
