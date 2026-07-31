package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"expensif/internal/domain"
	"expensif/internal/service"
)

// heatRampColors are the green→yellow→orange→red blob swatches for the
// calendar spending heatmap, indexed by quintile 1-5 (index 0 is unused —
// zero-spend days render no blob). Magnitude is double-encoded via heatDiameters
// so it isn't color-alone.
var heatRampColors = [6]string{"", "#4ade80", "#a3e635", "#facc15", "#fb923c", "#dc2626"}

// heatDiameters are the blob diameters in px per quintile level.
var heatDiameters = [6]int{0, 20, 34, 48, 64, 82}

// heatLevel buckets total into a 1-5 quintile rank among sortedSpendTotals
// (ascending, one entry per day that had any spend). Rank-based so outliers
// can't stretch the scale — a single huge day just occupies the top bucket.
func heatLevel(total float64, sortedSpendTotals []float64) int {
	n := len(sortedSpendTotals)
	if n == 0 {
		return 0
	}
	pos := sort.SearchFloat64s(sortedSpendTotals, total)
	bucket := pos * 5 / n
	if bucket > 4 {
		bucket = 4
	}
	return bucket + 1
}

type HTMLHandler struct {
	svc      *service.Service
	renderer *Renderer
	now      func() time.Time // injectable so tests can pin "today"
	notice   *Notice          // background messages waiting for a page to land on
}

func NewHTMLHandler(svc *service.Service, renderer *Renderer) *HTMLHandler {
	return &HTMLHandler{svc: svc, renderer: renderer, now: time.Now, notice: &Notice{}}
}

// Notice is where a background worker leaves a message for the next page a
// human loads. The rendered page is the only place a Flash can appear, so the
// handler owns the notice and hands it out rather than being handed one.
func (h *HTMLHandler) Notice() *Notice { return h.notice }

// takeNotice moves a pending background message into this page's Flash. A flash
// the page already set outranks it — an error the human just caused is the more
// urgent of the two — and the notice simply waits for the page after.
//
// Only whole pages consume it. A fragment draws no base layout and so has
// nowhere to show a Flash; consuming one there would swallow it silently.
func (h *HTMLHandler) takeNotice(data *PageData) {
	if data.Flash == "" {
		data.Flash = h.notice.Take()
	}
}

func (h *HTMLHandler) nowInTZ(tz string) time.Time {
	loc := time.Local
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	return h.now().In(loc)
}

func (h *HTMLHandler) basePageData(ctx context.Context, active string) PageData {
	prefs, err := h.svc.Preferences(ctx)
	if err != nil {
		slog.Error("failed to load preferences", "error", err)
		prefs = &domain.Preferences{Currency: "USD"}
	}
	return PageData{
		Active:         active,
		CurrencySymbol: domain.CurrencySymbol(prefs.Currency),
		Currency:       prefs.Currency,
		UserID:         prefs.UserID,
		Timezone:       prefs.Timezone,
		Today:          h.nowInTZ(prefs.Timezone).Format("2006-01-02"),
		Islands:        []string{"mobile-nav"},
	}
}

func (h *HTMLHandler) render(w http.ResponseWriter, name string, data PageData) {
	h.takeNotice(&data)
	if err := h.renderer.Render(w, name, data); err != nil {
		slog.Error("render failed", "template", name, "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (h *HTMLHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	expenses, err := h.svc.ListExpenses(ctx, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := h.basePageData(ctx, "list")

	// Convert per-row and totals to preference currency
	convertedTotal, rateDate, _ := h.svc.ConvertExpensesTotal(ctx, expenses, data.Currency)
	data.ConvertedTotal = convertedTotal
	data.RateDate = rateDate
	data.ShowConverted = convertedTotal > 0

	catConverted := make(map[string]float64)
	rates, _, rateErr := h.svc.GetRatesForConversion(ctx)
	if rateErr == nil {
		for i := range expenses {
			conv, err := h.svc.ConvertWithRates(expenses[i].Amount, expenses[i].Currency, data.Currency, rates)
			if err == nil {
				expenses[i].ConvertedAmount = conv
				catConverted[expenses[i].Category] += conv
			}
		}
	}

	data.Expenses = expenses

	var categories []domain.CategorySummary
	for name, amount := range catConverted {
		categories = append(categories, domain.CategorySummary{Name: name, Amount: amount})
	}
	data.Categories = categories

	total, _ := h.svc.TotalExpenses(ctx)
	data.Total = total
	data.Islands = append(data.Islands, "data-table")

	h.render(w, "list", data)
}

func (h *HTMLHandler) HandleCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "calendar")

	earliest, _ := h.svc.GetEarliestExpenseDate(ctx)
	loc := time.Local
	if data.Timezone != "" {
		if l, err := time.LoadLocation(data.Timezone); err == nil {
			loc = l
		}
	}
	today := h.nowInTZ(data.Timezone)

	// Start = earliest expense or 1 year before today, whichever is earlier
	var start time.Time
	if earliest != "" {
		start, _ = time.Parse("2006-01-02", earliest)
	}
	oneYearAgo := today.AddDate(-1, 0, 0)
	if start.IsZero() || start.After(oneYearAgo) {
		start = oneYearAgo
	}
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc)

	// End = today + 1 year
	end := today.AddDate(1, 0, 0)
	end = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, loc)

	// Aggregate converted daily totals across the full range in one pass.
	lastDay := end.AddDate(0, 0, -1)
	dayTotals := make(map[string]struct {
		Total float64
		Count int
	})
	expenses, err := h.svc.ExpensesInRange(ctx, start.Format("2006-01-02"), lastDay.Format("2006-01-02"))
	if err != nil {
		slog.Error("failed to load expenses for calendar", "error", err)
	}
	rates, _, rateErr := h.svc.GetRatesForConversion(ctx)
	for _, e := range expenses {
		amount := e.Amount
		if rateErr == nil {
			if conv, err := h.svc.ConvertWithRates(e.Amount, e.Currency, data.Currency, rates); err == nil {
				amount = conv
			}
		}
		entry := dayTotals[e.Date]
		entry.Total += amount
		entry.Count++
		dayTotals[e.Date] = entry
	}

	// Quintile buckets computed globally over the whole range, ranked only
	// among days that had any spend (zero-spend days stay untinted).
	var spendTotals []float64
	for _, dt := range dayTotals {
		if dt.Count > 0 {
			spendTotals = append(spendTotals, dt.Total)
		}
	}
	sort.Float64s(spendTotals)

	var months []domain.CalendarMonth
	for m := start; !m.After(end); m = m.AddDate(0, 1, 0) {
		var month domain.CalendarMonth
		month.Label = m.Format("Jan 2006")

		first := m
		last := m.AddDate(0, 1, -1)

		// Padding before (Sunday → first day)
		for d := 0; d < int(first.Weekday()); d++ {
			month.Cells = append(month.Cells, domain.CalendarCell{})
		}

		// Days
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			isToday := dateStr == data.Today
			if isToday {
				month.HasToday = true
			}
			totals := dayTotals[dateStr]
			level := 0
			if totals.Count > 0 {
				level = heatLevel(totals.Total, spendTotals)
			}
			month.Cells = append(month.Cells, domain.CalendarCell{
				Day:          d.Day(),
				Date:         dateStr,
				IsToday:      isToday,
				Total:        totals.Total,
				Count:        totals.Count,
				HeatLevel:    level,
				HeatColor:    heatRampColors[level],
				HeatDiameter: heatDiameters[level],
			})
		}

		// Padding after (last day → Saturday)
		for d := int(last.Weekday()) + 1; d <= 6; d++ {
			month.Cells = append(month.Cells, domain.CalendarCell{})
		}

		months = append(months, month)
	}

	data.CalendarMonths = months
	h.render(w, "calendar", data)
}

// httpDateRangeError maps the service's date-validation errors to 400,
// matching the write path's precedent; anything else is a 500.
func httpDateRangeError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrInvalidDate) || errors.Is(err, service.ErrInvalidRange) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// dailyWindowDays is one window of the timeline: the page opens on the 30 days
// ending today, and the scroll walks back a window at a time.
const dailyWindowDays = 30

// dailyScrollIsland is the timeline's only script. It names its extension
// because it is not a React island — plain TypeScript, no components, no JSX.
const dailyScrollIsland = "daily-scroll.ts"

// olderWindow computes the cursor the scroll island fetches next: the window
// immediately older than the one starting at start, clamped so the last window
// stops on the earliest expense rather than running back through empty
// pre-history. ok is false when nothing older than start exists — the terminal.
//
// This is the scroll's entire stop condition, and it lives here rather than in
// the island, which does no date arithmetic and reads only the cursor's
// presence. Dates are YYYY-MM-DD, so they compare lexicographically.
func olderWindow(start, earliest string) (nextStart, nextEnd string, ok bool) {
	if earliest == "" || earliest >= start {
		return "", "", false
	}
	startT, err := time.Parse("2006-01-02", start)
	if err != nil {
		return "", "", false
	}
	nextEnd = startT.AddDate(0, 0, -1).Format("2006-01-02")
	nextStart = startT.AddDate(0, 0, -dailyWindowDays).Format("2006-01-02")
	if nextStart < earliest {
		nextStart = earliest
	}
	return nextStart, nextEnd, true
}

// convertDailyGroups converts every expense and day total into the Preferred
// Currency, in place. When rates are unavailable the groups keep their own
// currencies — the ledger's own fallback for a missing conversion.
func (h *HTMLHandler) convertDailyGroups(ctx context.Context, groups []domain.DailyGroup, currency string) {
	rates, _, err := h.svc.GetRatesForConversion(ctx)
	if err != nil {
		return
	}
	for i := range groups {
		var convTotal float64
		for j := range groups[i].Expenses {
			conv, convErr := h.svc.ConvertWithRates(groups[i].Expenses[j].Amount, groups[i].Expenses[j].Currency, currency, rates)
			if convErr == nil {
				groups[i].Expenses[j].ConvertedAmount = conv
				convTotal += conv
			}
		}
		groups[i].ConvertedTotal = convTotal
	}
}

// HandleDailyOlder serves one older window of the ledger as an HTML fragment,
// drawn by the same partial as the first window — so the ledger has exactly one
// implementation, and the scroll cannot drift from the page it extends.
func (h *HTMLHandler) HandleDailyOlder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	startT, startErr := time.Parse("2006-01-02", start)
	endT, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil {
		httpDateRangeError(w, service.ErrInvalidDate)
		return
	}
	// Cursors are server-issued and never span more than a window, so a longer
	// range is a hand-crafted request — and the gap-fill would answer it with a
	// row per day for as far back as it asked.
	if endT.Sub(startT) >= dailyWindowDays*24*time.Hour {
		http.Error(w, "Range must not exceed one window", http.StatusBadRequest)
		return
	}

	// An inverted range is rejected here (400) rather than answered with an
	// empty window, which would reach the island as an empty fragment,
	// indistinguishable from the terminal.
	groups, err := h.svc.DailyGroupsInRange(ctx, start, end)
	if err != nil {
		httpDateRangeError(w, err)
		return
	}

	data := h.basePageData(ctx, "daily")
	// The fragment lands in the timeline, so its rows' edit links must lead
	// back there — never to this endpoint, which serves a bare fragment.
	data.ReturnTo = "/"
	h.convertDailyGroups(ctx, groups, data.Currency)
	data.DailyGroups = groups
	// The day drawn directly above this window's first row is the day after its
	// end: the timeline is gap-filled and contiguous, so that is the whole seam,
	// and the top divider knows whether it is a month break.
	data.PrevDate = endT.AddDate(0, 0, 1).Format("2006-01-02")

	earliest, _ := h.svc.GetEarliestExpenseDate(ctx)
	data.NextStart, data.NextEnd, _ = olderWindow(start, earliest)

	if err := h.renderer.RenderFragment(w, "daily-older", data); err != nil {
		slog.Error("render failed", "fragment", "daily-older", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (h *HTMLHandler) HandleDaily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "daily")

	filterDate := r.URL.Query().Get("date")
	// Every ledger row's edit link carries where it came from, so delete's
	// danger zone can send the user back. Only the redirect that finally
	// consumes it validates it.
	data.ReturnTo = r.URL.RequestURI()
	var groups []domain.DailyGroup
	upcomingCount := 0
	if filterDate != "" {
		data.FilterDate = filterDate
		if r.URL.Query().Get("from") == "calendar" {
			data.BackHref = "/calendar?date=" + filterDate
			data.BackLabel = "Back to calendar"
		} else {
			data.BackHref = "/"
			data.BackLabel = "Back to daily view"
		}
		expenses, rangeErr := h.svc.ExpensesInRange(ctx, filterDate, filterDate)
		if rangeErr != nil {
			http.Error(w, rangeErr.Error(), http.StatusInternalServerError)
			return
		}
		var dayTotal float64
		for _, e := range expenses {
			dayTotal += e.Amount
		}
		groups = []domain.DailyGroup{{Date: filterDate, Expenses: expenses, Total: dayTotal}}
	} else {
		// The timeline: a rolling 30-day window anchored at today, with
		// future-dated expenses (ungapped) above it. The timezone names
		// today once, here; the service walk is timezone-free.
		endT, parseErr := time.Parse("2006-01-02", data.Today)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusInternalServerError)
			return
		}
		start := endT.AddDate(0, 0, -(dailyWindowDays - 1)).Format("2006-01-02")
		window, rangeErr := h.svc.DailyGroupsInRange(ctx, start, data.Today)
		if rangeErr != nil {
			httpDateRangeError(w, rangeErr)
			return
		}
		upcoming, upErr := h.svc.UpcomingGroups(ctx, data.Today)
		if upErr != nil {
			httpDateRangeError(w, upErr)
			return
		}
		upcomingCount = len(upcoming)
		groups = append(upcoming, window...)

		// One fetch answers two questions: whether the account has ever
		// recorded an expense, and whether anything sits older than the window
		// for the scroll to reach.
		earliest, _ := h.svc.GetEarliestExpenseDate(ctx)
		data.NoExpensesEver = earliest == ""
		if !data.NoExpensesEver {
			data.NextStart, data.NextEnd, _ = olderWindow(start, earliest)
			data.Islands = append(data.Islands, dailyScrollIsland)
		}
	}

	h.convertDailyGroups(ctx, groups, data.Currency)

	// Cap upcoming at the 3 days nearest today (the slice is newest-first, so
	// those are its last elements); the rest collapse into the overflow row.
	// After the conversion loop, so the row's total is in the page currency.
	if upcomingCount > 3 {
		hidden := groups[:upcomingCount-3]
		data.HiddenUpcoming = len(hidden)
		for _, g := range hidden {
			if g.ConvertedTotal > 0 {
				data.HiddenUpcomingTotal += g.ConvertedTotal
			} else {
				data.HiddenUpcomingTotal += g.Total
			}
		}
		groups = groups[upcomingCount-3:]
	}

	data.DailyGroups = groups
	h.render(w, "daily", data)
}

func (h *HTMLHandler) HandleAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	date := r.URL.Query().Get("date")
	data := h.basePageData(ctx, "add")
	if date == "" {
		date = data.Today
	}
	data.Today = date
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		slog.Error("failed to list users", "error", err)
	}
	data.Users = users
	// Pre-select current user from preferences
	data.PaidByID = data.UserID
	data.Islands = append(data.Islands, "category-pills", "description-pills")
	h.render(w, "add", data)
}

func (h *HTMLHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	date := r.FormValue("date")
	category := r.FormValue("category")
	description := r.FormValue("description")
	currency := r.FormValue("currency")
	paidByID, _ := strconv.ParseInt(r.FormValue("paid_by_id"), 10, 64)

	_, err := h.svc.CreateExpense(ctx, amount, category, description, date, currency, paidByID)
	if err != nil {
		data := h.basePageData(ctx, "add")
		data.Flash = err.Error()
		data.FlashError = true
		data.Today = date
		data.Expense = &domain.Expense{
			Amount:      amount,
			Category:    category,
			Description: description,
			Date:        date,
			Currency:    currency,
			PaidByID:    paidByID,
		}
		users, _ := h.svc.ListUsers(ctx)
		data.Users = users
		data.Islands = append(data.Islands, "category-pills", "description-pills")
		h.render(w, "add", data)
		return
	}

	if r.FormValue("action") == "another" {
		http.Redirect(w, r, "/expenses/new", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (h *HTMLHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	expense, err := h.svc.GetExpense(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data := h.basePageData(ctx, "edit")
	data.Expense = expense
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		slog.Error("failed to list users", "error", err)
	}
	data.Users = users
	// Passed through to the danger zone's hidden field, unvalidated: the edit
	// page is reachable from the timeline, the single-day view and the List
	// View, and only the redirect that consumes this needs to trust it.
	data.ReturnTo = r.URL.Query().Get("return")
	data.Islands = append(data.Islands, "category-pills", "description-pills")
	h.render(w, "edit", data)
}

func (h *HTMLHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	date := r.FormValue("date")
	category := r.FormValue("category")
	description := r.FormValue("description")
	currency := r.FormValue("currency")
	paidByID, _ := strconv.ParseInt(r.FormValue("paid_by_id"), 10, 64)

	err := h.svc.UpdateExpense(ctx, id, amount, category, description, date, currency, paidByID)
	if err != nil {
		data := h.basePageData(ctx, "edit")
		data.Expense = &domain.Expense{
			ID:          id,
			Amount:      amount,
			Category:    category,
			Description: description,
			Date:        date,
			Currency:    currency,
			PaidByID:    paidByID,
		}
		data.Flash = err.Error()
		data.FlashError = true
		// The form posts the return path back, so a failed save's re-render
		// keeps the danger zone's destination intact.
		data.ReturnTo = r.FormValue("return")
		users, _ := h.svc.ListUsers(ctx)
		data.Users = users
		data.Islands = append(data.Islands, "category-pills", "description-pills")
		h.render(w, "edit", data)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *HTMLHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.DeleteExpense(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Redirect(w, r, localPath(r.FormValue("return")), http.StatusSeeOther)
}

// localPath keeps a redirect target inside this app. Anything that could send
// the browser to another origin — an absolute URL, a protocol-relative "//host",
// a backslash the browser folds into one — falls back to "/". The value reaches
// us through a hidden form field, so it is attacker-supplied in principle even
// though the app only ever issues local paths.
func localPath(raw string) string {
	const fallback = "/"
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, `/\`) {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return fallback
	}
	return raw
}

func (h *HTMLHandler) HandlePreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "prefs")
	users, _ := h.svc.ListUsers(ctx)
	data.Users = users
	h.render(w, "prefs", data)
}

func (h *HTMLHandler) HandleSavePreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	currency := r.FormValue("currency")
	if currency == "" {
		currency = "USD"
	}
	userID, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	timezone := r.FormValue("timezone")

	if err := h.svc.SavePreferences(ctx, currency, userID, timezone); err != nil {
		data := h.basePageData(ctx, "prefs")
		data.Flash = "Failed to save preferences"
		data.FlashError = true
		users, _ := h.svc.ListUsers(ctx)
		data.Users = users
		h.render(w, "prefs", data)
		return
	}

	data := h.basePageData(ctx, "prefs")
	data.Flash = "Preferences saved"
	users, _ := h.svc.ListUsers(ctx)
	data.Users = users
	h.render(w, "prefs", data)
}

// --- User Management ---

func (h *HTMLHandler) HandleUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := h.basePageData(ctx, "users")
	data.Users = users
	data.Islands = append(data.Islands, "data-table")
	h.render(w, "users", data)
}

func (h *HTMLHandler) HandleUserNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "users")
	h.render(w, "user_form", data)
}

func (h *HTMLHandler) HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	_, err := h.svc.CreateUser(ctx, name)
	if err != nil {
		data := h.basePageData(ctx, "users")
		data.User = &domain.User{Name: name}
		data.Flash = err.Error()
		data.FlashError = true
		h.render(w, "user_form", data)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

func (h *HTMLHandler) HandleUserEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	user, err := h.svc.GetUser(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data := h.basePageData(ctx, "users")
	data.User = user
	h.render(w, "user_form", data)
}

func (h *HTMLHandler) HandleUserUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	name := r.FormValue("name")
	err := h.svc.UpdateUser(ctx, id, name)
	if err != nil {
		data := h.basePageData(ctx, "users")
		data.User = &domain.User{ID: id, Name: name}
		data.Flash = err.Error()
		data.FlashError = true
		h.render(w, "user_form", data)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

func (h *HTMLHandler) HandleUserDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.svc.DeleteUser(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}
