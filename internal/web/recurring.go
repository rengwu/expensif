package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"expensif/internal/domain"
)

// RecurringPageData is the Recurring page's view model: the shared PageData
// (nav, currency, today) plus the rules it lists and the one figure above them.
type RecurringPageData struct {
	PageData
	Rules []RecurringRuleRow
	// MonthlyEquivalent is what every enabled rule costs per month, in the
	// Preferred Currency. ShowMonthlyEquivalent is false when it could not be
	// trusted, and the page then says so rather than printing a wrong number —
	// the same convention the List View's converted total already follows.
	MonthlyEquivalent     float64
	ShowMonthlyEquivalent bool
}

// RecurringRuleRow is one listed rule: the rule itself plus its schedule in
// words. The template reads the rest — amount, currency, dates — off the rule
// through the same formatting helpers every other page uses.
type RecurringRuleRow struct {
	domain.RecurringRule
	Interval string
}

// RecurringFormData is the new/edit form's view model. Rule carries the values
// the fields render: the stored rule when editing, and on a rejected save the
// values the human just submitted, so nothing typed is lost.
type RecurringFormData struct {
	PageData
	Rule   domain.RecurringRule
	IsEdit bool
}

// recurringFlashes maps a redirect's short code to the message the Recurring
// page shows. A whitelist, so a mutation can confirm itself across the redirect
// that follows it without any URL text ever reaching the page.
var recurringFlashes = map[string]string{
	"created": "Recurring rule created",
	"updated": "Recurring rule saved",
	"deleted": "Recurring rule deleted",
	"paused":  "Recurring rule paused",
	"resumed": "Recurring rule resumed",
}

// intervalLabel says a schedule in words — "Every month", "Every 2 weeks".
func intervalLabel(unit domain.IntervalUnit, count int) string {
	noun := "month"
	switch unit {
	case domain.IntervalWeeks:
		noun = "week"
	case domain.IntervalDays:
		noun = "day"
	}
	if count <= 1 {
		return "Every " + noun
	}
	return fmt.Sprintf("Every %d %ss", count, noun)
}

// recurringRows orders the rules for the page — soonest charge first, which is
// how the repository already returns them, with disabled rules after every
// enabled one. A disabled rule's stored date is a stopped clock (it can even be
// the date that overran its end date), so it sorts last and the template does
// not print it as a next charge.
func recurringRows(rules []domain.RecurringRule) []RecurringRuleRow {
	rows := make([]RecurringRuleRow, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, RecurringRuleRow{
			RecurringRule: rule,
			Interval:      intervalLabel(rule.IntervalUnit, rule.IntervalCount),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Enabled && !rows[j].Enabled
	})
	return rows
}

// renderRecurring draws the list page over rules the caller has already read,
// so a failed mutation can re-render it with its own flash rather than
// redirecting the human away from the error.
func (h *HTMLHandler) renderRecurring(ctx context.Context, w http.ResponseWriter, data PageData, rules []domain.RecurringRule) {
	monthly, ok := h.svc.MonthlyEquivalent(ctx, rules, data.Currency)

	// This page carries its own view model, so it takes the background notice
	// here — where h.render would otherwise do it for a plain PageData page.
	h.takeNotice(&data)
	page := RecurringPageData{
		PageData:              data,
		Rules:                 recurringRows(rules),
		MonthlyEquivalent:     monthly,
		ShowMonthlyEquivalent: ok,
	}
	if err := h.renderer.RenderPage(w, "recurring", page); err != nil {
		slog.Error("render failed", "template", "recurring", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// HandleRecurring lists every Recurring Rule with the Monthly Equivalent above
// them.
func (h *HTMLHandler) HandleRecurring(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "recurring")
	data.Flash = recurringFlashes[r.URL.Query().Get("flash")]

	rules, err := h.svc.ListRules(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.renderRecurring(ctx, w, data, rules)
}

// renderRecurringError re-renders the list with the error the operation returned.
// Toggling and deleting have no form of their own to send the human back to, so
// the list is where their failures have to appear.
func (h *HTMLHandler) renderRecurringError(ctx context.Context, w http.ResponseWriter, data PageData, cause error) {
	data.Flash = cause.Error()
	data.FlashError = true
	rules, err := h.svc.ListRules(ctx)
	if err != nil {
		slog.Error("recurring: list rules", "error", err)
	}
	h.renderRecurring(ctx, w, data, rules)
}

// renderRuleForm draws the new/edit form, on its own view model.
func (h *HTMLHandler) renderRuleForm(ctx context.Context, w http.ResponseWriter, data PageData, rule domain.RecurringRule, isEdit bool) {
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		slog.Error("recurring: list users", "error", err)
	}
	data.Users = users
	h.takeNotice(&data)
	form := RecurringFormData{PageData: data, Rule: rule, IsEdit: isEdit}
	if err := h.renderer.RenderPage(w, "recurring_form", form); err != nil {
		slog.Error("render failed", "template", "recurring_form", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// HandleRecurringNew serves the empty form: monthly from today, enabled, in the
// Preferred Currency, with the Preferences payer pre-selected the way the add
// form does it — the service deliberately defaults neither payer nor enabled.
func (h *HTMLHandler) HandleRecurringNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := h.basePageData(ctx, "recurring")
	rule := domain.RecurringRule{
		Currency:       data.Currency,
		PaidByID:       data.UserID,
		IntervalUnit:   domain.IntervalMonths,
		IntervalCount:  1,
		NextChargeDate: data.Today,
		Enabled:        true,
	}
	h.renderRuleForm(ctx, w, data, rule, false)
}

// parseRuleForm reads the rule the submitted form describes. Every field is
// carried, including Enabled and the Anchor Day: the service defaults neither,
// and an anchor silently re-derived from a clamped charge date is exactly the
// drift this effort exists to prevent.
func parseRuleForm(r *http.Request) domain.RecurringRule {
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	count, _ := strconv.Atoi(r.FormValue("interval_count"))
	anchorDay, _ := strconv.Atoi(r.FormValue("anchor_day"))
	paidByID, _ := strconv.ParseInt(r.FormValue("paid_by_id"), 10, 64)
	return domain.RecurringRule{
		Name:           r.FormValue("name"),
		Amount:         amount,
		Currency:       r.FormValue("currency"),
		Category:       r.FormValue("category"),
		PaidByID:       paidByID,
		IntervalUnit:   domain.IntervalUnit(r.FormValue("interval_unit")),
		IntervalCount:  count,
		AnchorDay:      anchorDay,
		NextChargeDate: r.FormValue("next_charge_date"),
		EndDate:        r.FormValue("end_date"),
		Enabled:        r.FormValue("enabled") != "",
	}
}

func (h *HTMLHandler) HandleRecurringCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rule := parseRuleForm(r)

	// A rule created here has no Pending Charge until generation's next pass
	// (at most an hour away): the background goroutine is the app's only
	// generator, and a handler that minted one would be a second.
	if _, err := h.svc.CreateRule(ctx, rule); err != nil {
		data := h.basePageData(ctx, "recurring")
		data.Flash = err.Error()
		data.FlashError = true
		h.renderRuleForm(ctx, w, data, rule, false)
		return
	}
	http.Redirect(w, r, "/recurring?flash=created", http.StatusSeeOther)
}

func (h *HTMLHandler) HandleRecurringEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rule, err := h.svc.GetRule(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	h.renderRuleForm(ctx, w, h.basePageData(ctx, "recurring"), *rule, true)
}

func (h *HTMLHandler) HandleRecurringUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data := h.basePageData(ctx, "recurring")
	rule := parseRuleForm(r)
	rule.ID, _ = strconv.ParseInt(r.PathValue("id"), 10, 64)

	// today in the configured Timezone: editing owns only the strictly future
	// charge, and where that boundary falls is a timezone question.
	if err := h.svc.UpdateRule(ctx, rule, data.Today); err != nil {
		data.Flash = err.Error()
		data.FlashError = true
		h.renderRuleForm(ctx, w, data, rule, true)
		return
	}
	http.Redirect(w, r, "/recurring?flash=updated", http.StatusSeeOther)
}

// HandleRecurringToggle pauses and resumes a rule. Pausing removes its Pending
// Charge; resuming picks the schedule up at today rather than backfilling the
// stretch the human deliberately paused.
func (h *HTMLHandler) HandleRecurringToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	enabled := r.FormValue("enabled") == "true"

	data := h.basePageData(ctx, "recurring")
	if err := h.svc.SetRuleEnabled(ctx, id, enabled, data.Today); err != nil {
		h.renderRecurringError(ctx, w, data, err)
		return
	}
	flash := "paused"
	if enabled {
		flash = "resumed"
	}
	http.Redirect(w, r, "/recurring?flash="+flash, http.StatusSeeOther)
}

// HandleRecurringDelete drops a rule and its Pending Charge. Every Expense it
// already minted survives as an ordinary unlinked one — deleting a schedule
// never destroys spending history.
func (h *HTMLHandler) HandleRecurringDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	data := h.basePageData(ctx, "recurring")
	if err := h.svc.DeleteRule(ctx, id, data.Today); err != nil {
		h.renderRecurringError(ctx, w, data, err)
		return
	}
	http.Redirect(w, r, "/recurring?flash=deleted", http.StatusSeeOther)
}
