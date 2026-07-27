package web

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"expensif/internal/domain"
	"expensif/internal/service"
)

// exportPreviewSampleSize is how many sample rows the preview shows — the head of
// the download, ordered identically.
const exportPreviewSampleSize = 10

// exportPreviewIsland is the Export page's live-preview script. Like
// daily-scroll, it names its extension because it is not a React island — plain
// TypeScript that fetches the preview fragment and swaps it into place.
const exportPreviewIsland = "export-preview.ts"

// ExportPageData is the Export page's view model: the shared PageData (nav,
// currency, today) plus the page's own controls and the first-paint preview. Its
// Categories field ([]string of the account's distinct categories) deliberately
// shadows PageData.Categories.
type ExportPageData struct {
	PageData
	Preset       string // the highlighted preset link: "month" | "year" | "all" | "" (custom)
	CustomRange  bool   // reveal the custom date inputs
	From         string // custom range bounds, echoed into the date inputs and the download form
	To           string
	Category     string // the selected category ("" = all)
	PayerID      int64  // the selected payer (0 = all)
	Categories   []string
	PrefCurrency string
	PreviewCount int               // exact number of matches — equals the download's row count
	PreviewRows  []ExportSampleRow // up to exportPreviewSampleSize raw sample rows
}

// ExportSampleRow is one preview row, pre-formatted in Go so the template — and
// the island that fetches it — render strings without any formatting logic.
// Amount uses the same FormatFloat(v,'f',-1,64) rule the CSV uses; these are the
// entered (raw) figures, never the converted ones.
type ExportSampleRow struct {
	Date        string
	Category    string
	Description string
	PaidByName  string
	Amount      string
	Currency    string
}

// exportSampleRows pre-formats the sample expenses for the preview partial.
func exportSampleRows(rows []domain.Expense) []ExportSampleRow {
	out := make([]ExportSampleRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, ExportSampleRow{
			Date:        e.Date,
			Category:    e.Category,
			Description: e.Description,
			PaidByName:  e.PaidByName,
			Amount:      formatMoney(e.Amount),
			Currency:    e.Currency,
		})
	}
	return out
}

// exportRange resolves the Export page's range controls into effective from/to
// bounds and the view state the template needs. A custom range (either bound
// present) shows the date inputs and highlights no preset; otherwise the preset
// links drive the bounds, defaulting to "all time" when none is given. today is
// the account's today (already in its timezone), used only to fill the relative
// presets by string surgery on the YYYY-MM-DD form.
func exportRange(preset, from, to, today string) (outPreset, outFrom, outTo string, custom bool) {
	if from != "" || to != "" {
		return "", from, to, true
	}
	switch preset {
	case "month":
		return "month", today[:8] + "01", today, false
	case "year":
		return "year", today[:4] + "-01-01", today, false
	default: // "all" and anything unrecognised default to the open range
		return "all", "", "", false
	}
}

// HandleExportPage renders the guided Export page: the range/category/payer
// controls and a server-rendered first-paint preview (count + raw sample) for the
// initial filters. Because the first paint is server-rendered, the export-preview
// island only fetches /export/preview once a filter actually changes.
func (h *HTMLHandler) HandleExportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	data := h.basePageData(ctx, "export")
	data.Islands = append(data.Islands, exportPreviewIsland)

	preset, from, to, custom := exportRange(q.Get("preset"), q.Get("from"), q.Get("to"), data.Today)

	start, end, err := parseExportRange(from, to)
	if err != nil {
		// A hand-typed bad or inverted range still renders the page chrome; the
		// preview simply matches nothing. (Rejecting a bad range with a 400 is the
		// download route's job, not the page's — the page stays forgiving so a
		// mid-edit URL isn't hostile.)
		start, end = "9999-12-31", "0001-01-01"
	}
	filter := domain.ExpenseFilter{
		Start:    start,
		End:      end,
		Category: q.Get("category"),
		PayerID:  parsePayerID(q.Get("payer")),
	}

	categories, err := h.svc.DistinctCategories(ctx)
	if err != nil {
		slog.Error("export: distinct categories", "error", err)
	}
	users, err := h.svc.ListUsers(ctx)
	if err != nil {
		slog.Error("export: list users", "error", err)
	}
	data.Users = users

	count, err := h.svc.CountMatching(ctx, filter)
	if err != nil {
		http.Error(w, "Failed to count expenses", http.StatusInternalServerError)
		return
	}
	sample, err := h.svc.SampleMatching(ctx, filter, exportPreviewSampleSize)
	if err != nil {
		http.Error(w, "Failed to read expenses", http.StatusInternalServerError)
		return
	}

	epd := ExportPageData{
		PageData:     data,
		Preset:       preset,
		CustomRange:  custom,
		From:         from,
		To:           to,
		Category:     filter.Category,
		PayerID:      filter.PayerID,
		Categories:   categories,
		PrefCurrency: data.Currency,
		PreviewCount: count,
		PreviewRows:  exportSampleRows(sample),
	}
	if err := h.renderer.RenderPage(w, "export", epd); err != nil {
		slog.Error("render failed", "template", "export", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// exportPreviewData is the preview fragment's view model — just the two fields
// the export-preview partial reads. The /export page's first paint fills these
// from ExportPageData; the /export/preview endpoint serves the same partial with
// this smaller shape as filters change.
type exportPreviewData struct {
	PreviewCount int
	PreviewRows  []ExportSampleRow
}

// HandleExportPreview serves the live-preview fragment: the same partial the
// Export page renders on first paint (a count line plus up to ten raw sample
// rows), re-rendered for the current filters as the input-driven island refetches
// it. It parses filters identically to /export.csv and does zero rate work, so it
// can never 503 — a bad or inverted range is a soft 400 (via httpDateRangeError)
// the island shows as an inline hint, and zero matches is a header-only 200.
func (h *HTMLHandler) HandleExportPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	start, end, err := parseExportRange(q.Get("from"), q.Get("to"))
	if err != nil {
		httpDateRangeError(w, err)
		return
	}
	filter := domain.ExpenseFilter{
		Start:    start,
		End:      end,
		Category: q.Get("category"),
		PayerID:  parsePayerID(q.Get("payer")),
	}

	count, err := h.svc.CountMatching(ctx, filter)
	if err != nil {
		http.Error(w, "Failed to count expenses", http.StatusInternalServerError)
		return
	}
	sample, err := h.svc.SampleMatching(ctx, filter, exportPreviewSampleSize)
	if err != nil {
		http.Error(w, "Failed to read expenses", http.StatusInternalServerError)
		return
	}

	data := exportPreviewData{PreviewCount: count, PreviewRows: exportSampleRows(sample)}
	if err := h.renderer.RenderFragment(w, "export-preview", data); err != nil {
		slog.Error("render failed", "fragment", "export-preview", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// utf8BOM is prepended once before the CSV so Excel on Windows detects UTF-8 and
// renders accented text and currency symbols correctly. Readers (and tests) must
// strip it, as Go's csv.Reader does not.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// exportColumns is the fixed, snake_case header row (record 0 of every export).
// created_at is deliberately omitted as internal audit metadata.
var exportColumns = []string{
	"id", "date", "category", "description", "paid_by",
	"amount", "currency", "amount_converted", "currency_converted", "rate_date",
}

// rateResolver yields the rate set and rate date to use for an expense's date.
// An empty rate date means no rates were available, so the converted columns
// come out blank. It is injected so buildRecords stays pure and rate-free: the
// handler backs it with the historical-rate lookup (RatesOnOrBefore + CrossRate),
// while tests pass a canned fake.
type rateResolver func(date string) (rates map[string]float64, rateDate string)

// resolvedRate memoises one date's on-or-before lookup for the export handler's
// resolver, so rows sharing a date read the store once.
type resolvedRate struct {
	rates map[string]float64
	date  string
}

// buildRecords turns raw expenses into CSV records — the header as record 0,
// then one record per expense in the ten-column order. Money is written with
// FormatFloat(v,'f',-1,64): the shortest decimal that round-trips the stored
// float64, so KWD's 1.234 and JPY's 100 keep their true precision. Absence is an
// empty cell: paid_by blank when no payer, amount_converted and rate_date blank
// when the row can't be converted, while currency_converted is present on every
// row.
func buildRecords(rows []domain.Expense, resolve rateResolver, pref string) [][]string {
	records := make([][]string, 0, len(rows)+1)
	records = append(records, exportColumns)

	for _, e := range rows {
		converted, rateDate := "", ""
		if rates, rd := resolve(e.Date); rd != "" {
			if v, err := service.CrossRate(e.Amount, e.Currency, pref, rates); err == nil {
				converted = formatMoney(v)
				rateDate = rd
			}
		}
		records = append(records, []string{
			strconv.FormatInt(e.ID, 10),
			e.Date,
			e.Category,
			e.Description,
			e.PaidByName,
			formatMoney(e.Amount),
			e.Currency,
			converted,
			pref,
			rateDate,
		})
	}
	return records
}

func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// HandleExportCSV streams the account's expenses, narrowed by the query filter,
// as a UTF-8 CSV the browser saves. Every status-bearing failure happens before
// the first byte, so the stream itself never needs to unwind: 400 on a bad or
// inverted range, then 503 when the historical rates the rows need can't be
// obtained. Zero matches is a legitimate header-only 200, not an error.
func (h *HTMLHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")

	start, end, err := parseExportRange(from, to)
	if err != nil {
		httpDateRangeError(w, err)
		return
	}

	filter := domain.ExpenseFilter{
		Start:    start,
		End:      end,
		Category: q.Get("category"),
		PayerID:  parsePayerID(q.Get("payer")),
	}
	rows, err := h.svc.ExpensesMatching(ctx, filter)
	if err != nil {
		http.Error(w, "Failed to read expenses", http.StatusInternalServerError)
		return
	}

	pref := "USD"
	if prefs, err := h.svc.Preferences(ctx); err == nil && prefs.Currency != "" {
		pref = prefs.Currency
	}

	// Backfill the historical rates the matched rows actually need — the span of
	// their own dates, not the (possibly open-ended) filter range — then convert
	// each row at its own-date rate. A provider that can't be reached is a hard
	// 503 with no file, decided here before the first byte, rather than shipping
	// today's-rate guesses. The fetch is skipped when nothing needs converting
	// (no rows, or every row already in the preferred currency), so a
	// single-currency export never fails on a down provider it doesn't need.
	if lo, hi, ok := conversionSpan(rows, pref); ok {
		if err := h.svc.EnsureRatesForRange(ctx, lo, hi); err != nil {
			http.Error(w, "Exchange rates are temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
	}

	// Resolve each expense's date to its on-or-before rate set, memoised so rows
	// sharing a date hit the store once. A date with no stored rate yields "",
	// which buildRecords renders as blank converted columns.
	rateCache := make(map[string]resolvedRate)
	resolve := func(date string) (map[string]float64, string) {
		if hit, ok := rateCache[date]; ok {
			return hit.rates, hit.date
		}
		rates, rateDate, err := h.svc.RatesOnOrBefore(ctx, date)
		if err != nil {
			rates, rateDate = nil, ""
		}
		rateCache[date] = resolvedRate{rates, rateDate}
		return rates, rateDate
	}
	records := buildRecords(rows, resolve, pref)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+h.exportFilename(from, to)+`"`)

	w.Write(utf8BOM)
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	for _, rec := range records {
		if err := cw.Write(rec); err != nil {
			// The status line is already sent; a mid-stream fault truncates the
			// file. Rare, and there is nothing left to say on the wire.
			return
		}
	}
	cw.Flush()
}

// parseExportRange resolves the from/to query params into an effective inclusive
// range. An absent bound widens to the open end (so a bare request exports
// everything); a present but unparseable bound is a 400, as is an inverted range.
func parseExportRange(from, to string) (start, end string, err error) {
	start, end = "0001-01-01", "9999-12-31"
	if from != "" {
		if _, e := time.Parse("2006-01-02", from); e != nil {
			return "", "", service.ErrInvalidDate
		}
		start = from
	}
	if to != "" {
		if _, e := time.Parse("2006-01-02", to); e != nil {
			return "", "", service.ErrInvalidDate
		}
		end = to
	}
	if start > end { // ISO dates compare lexically
		return "", "", service.ErrInvalidRange
	}
	return start, end, nil
}

// conversionSpan returns the earliest and latest dates among the rows that
// actually need converting (currency other than pref), so rate backfill covers
// exactly the dates in play rather than the filter's possibly open-ended range.
// ok is false when no row needs conversion — nothing to convert, so nothing to
// fetch and no way to 503.
func conversionSpan(rows []domain.Expense, pref string) (lo, hi string, ok bool) {
	for _, e := range rows {
		if e.Currency == pref {
			continue
		}
		if !ok {
			lo, hi, ok = e.Date, e.Date, true
			continue
		}
		if e.Date < lo {
			lo = e.Date
		}
		if e.Date > hi {
			hi = e.Date
		}
	}
	return lo, hi, ok
}

func parsePayerID(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id // 0 on empty/invalid = all payers
}

// exportFilename names the download for its range when both bounds are given,
// else for today — so open-ended exports still get a distinct, dated name.
func (h *HTMLHandler) exportFilename(from, to string) string {
	if from != "" && to != "" {
		return "expensif-" + from + "_" + to + ".csv"
	}
	return "expensif-expenses-" + h.now().Format("2006-01-02") + ".csv"
}
