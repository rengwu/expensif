package domain

import "time"

type Expense struct {
	ID          int64   `json:"id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"` // YYYY-MM-DD
	Currency    string  `json:"currency"`
	PaidByID    int64   `json:"paidById,omitempty"`
	PaidByName  string  `json:"paidByName,omitempty"` // computed at render time
	// RecurringRuleID links an expense to the Recurring Rule that minted it, and
	// is 0 for a hand-entered one. It is the only thing that distinguishes the
	// two: a Pending Charge is an ordinary future-dated expense, with no status
	// of its own, so every existing view reads it without knowing rules exist.
	RecurringRuleID int64     `json:"recurringRuleId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	ConvertedAmount float64   `json:"convertedAmount"` // computed at render time
}

// ExpenseFilter narrows a set of expenses to a required date range and,
// optionally, a single category and/or payer. Zero-values mean "all": an empty
// Category matches every category and a PayerID of 0 matches every payer, so a
// bare range exports everything within it.
type ExpenseFilter struct {
	Start, End string // required date range (YYYY-MM-DD, inclusive)
	Category   string // "" = all categories
	PayerID    int64  // 0  = all payers
}

// IntervalUnit is the unit of a Recurring Rule's schedule. Only months need
// calendar reasoning; weeks and days are plain multiples of 24 hours.
type IntervalUnit string

const (
	IntervalMonths IntervalUnit = "months"
	IntervalWeeks  IntervalUnit = "weeks"
	IntervalDays   IntervalUnit = "days"
)

// RecurringRule is a thing that charges on a schedule — rent, a subscription, an
// installment. It mints ordinary Expenses on its own dates; its Name *is* their
// Description, so there is deliberately no separate description field.
//
// AnchorDay is stored apart from NextChargeDate and is what the schedule is
// computed from. NextChargeDate is a cheap index for "which rules are due" and
// is never authoritative: each monthly occurrence is min(AnchorDay, days in that
// month), recomputed from the anchor rather than accumulated from the last
// (possibly clamped) date. That is what keeps a rule anchored on the 31st
// running Jan 31 → Feb 28 → Mar 31 instead of walking backwards forever.
type RecurringRule struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	Category   string  `json:"category"`
	PaidByID   int64   `json:"paidById,omitempty"`
	PaidByName string  `json:"paidByName,omitempty"` // computed at render time

	IntervalUnit   IntervalUnit `json:"intervalUnit"`
	IntervalCount  int          `json:"intervalCount"`
	AnchorDay      int          `json:"anchorDay"`         // 1-31, the day of month a monthly rule aims for
	NextChargeDate string       `json:"nextChargeDate"`    // YYYY-MM-DD
	EndDate        string       `json:"endDate,omitempty"` // YYYY-MM-DD; "" = open-ended
	Enabled        bool         `json:"enabled"`

	CreatedAt time.Time `json:"createdAt"`
}

type Preferences struct {
	Currency string `json:"currency"`
	UserID   int64  `json:"userId"`
	Timezone string `json:"timezone"`
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type CategorySummary struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type DailyGroup struct {
	Date           string    `json:"date"`
	Expenses       []Expense `json:"expenses"`
	Total          float64   `json:"total"`
	ConvertedTotal float64   `json:"convertedTotal"`
}

type CalendarCell struct {
	Day          int     `json:"day"`
	Date         string  `json:"date"`
	IsToday      bool    `json:"isToday"`
	Total        float64 `json:"total"`
	Count        int     `json:"count"`
	HeatLevel    int     `json:"heatLevel"`    // 0 = no spend, 1-5 = quintile of spend among days with any spend
	HeatColor    string  `json:"heatColor"`    // blob hex color for HeatLevel, "" when HeatLevel is 0
	HeatDiameter int     `json:"heatDiameter"` // blob diameter in px for HeatLevel, 0 when HeatLevel is 0
}

type CalendarMonth struct {
	Label    string         `json:"label"`
	HasToday bool           `json:"hasToday"`
	Cells    []CalendarCell `json:"cells"`
}

type DescriptionCount struct {
	Description string `json:"description"`
	Count       int    `json:"count"`
}
