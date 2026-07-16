package domain

import "time"

type Expense struct {
	ID              int64     `json:"id"`
	Amount          float64   `json:"amount"`
	Category        string    `json:"category"`
	Description     string    `json:"description"`
	Date            string    `json:"date"` // YYYY-MM-DD
	Currency        string    `json:"currency"`
	PaidByID        int64     `json:"paidById,omitempty"`
	PaidByName      string    `json:"paidByName,omitempty"` // computed at render time
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
