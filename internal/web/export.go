package web

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"expensif/internal/domain"
	"expensif/internal/service"
)

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
// come out blank. It is injected so buildRecords stays pure and rate-free; this
// ticket passes a no-op resolver (converted columns blank), ticket 02 supplies
// the real historical-rate lookup.
type rateResolver func(date string) (rates map[string]float64, rateDate string)

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
// as a UTF-8 CSV the browser saves. Every status-bearing failure (400 on a bad
// or inverted range) happens before the first byte, so the stream itself never
// needs to unwind. Zero matches is a legitimate header-only 200, not an error.
// (Conversion columns are blank until ticket 02 wires the rate resolver; ticket
// 02 also adds the 503 rate-failure branch here.)
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

	// No conversion this ticket: the resolver always reports "no rate", so the
	// converted columns are blank. Ticket 02 replaces it with the real lookup.
	resolve := func(string) (map[string]float64, string) { return nil, "" }
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
