package web

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"expensif/internal/domain"
	"expensif/internal/repository"
	"expensif/internal/service"
)

// TestBuildRecords_ColumnsFormattingAndBlanks is the leading red: it pins the
// ten-column contract, the exact-decimal money formatting, and the "absence is a
// blank cell" rule, with the rate lookup injected as a fake resolver so the pure
// record builder never touches the rate machinery.
func TestBuildRecords_ColumnsFormattingAndBlanks(t *testing.T) {
	rows := []domain.Expense{
		{ID: 1, Date: "2026-01-05", Category: "travel", Description: "flight", PaidByName: "Alice", Amount: 1.234, Currency: "KWD"},
		{ID: 2, Date: "2026-01-06", Category: "food", Description: "sushi", PaidByName: "", Amount: 100, Currency: "JPY"},
		{ID: 3, Date: "2026-01-07", Category: "misc", Description: "odd", PaidByName: "Bob", Amount: 9.99, Currency: "ZZZ"},
		{ID: 4, Date: "2026-01-08", Category: "office", Description: "pens", PaidByName: "", Amount: 12.5, Currency: "USD"},
	}
	// The resolver yields a fixed rate set for every date and reports the queried
	// date as the rate date. "ZZZ" is deliberately absent, so that row is
	// unconvertible even though the date's rates were obtained.
	resolve := func(date string) (map[string]float64, string) {
		return map[string]float64{"USD": 1.0, "KWD": 0.5, "JPY": 100.0}, date
	}

	got := buildRecords(rows, resolve, "USD")

	want := [][]string{
		{"id", "date", "category", "description", "paid_by", "amount", "currency", "amount_converted", "currency_converted", "rate_date"},
		{"1", "2026-01-05", "travel", "flight", "Alice", "1.234", "KWD", "2.468", "USD", "2026-01-05"},
		{"2", "2026-01-06", "food", "sushi", "", "100", "JPY", "1", "USD", "2026-01-06"},
		{"3", "2026-01-07", "misc", "odd", "Bob", "9.99", "ZZZ", "", "USD", ""},
		{"4", "2026-01-08", "office", "pens", "", "12.5", "USD", "12.5", "USD", "2026-01-08"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRecords mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// TestBuildRecords_ZeroRowsHeaderOnly proves a filter that matches nothing still
// yields a valid header-only file, not an empty one.
func TestBuildRecords_ZeroRowsHeaderOnly(t *testing.T) {
	got := buildRecords(nil, func(string) (map[string]float64, string) { return nil, "" }, "USD")
	if len(got) != 1 {
		t.Fatalf("expected header-only (1 record), got %d records", len(got))
	}
	if got[0][0] != "id" {
		t.Fatalf("expected header record first, got %v", got[0])
	}
}

func exportHandler(t *testing.T, seed func(*mockRepo)) *HTMLHandler {
	return exportHandlerWithRates(t, seed, &mockRateClient{})
}

func exportHandlerWithRates(t *testing.T, seed func(*mockRepo), client *mockRateClient) *HTMLHandler {
	t.Helper()
	repo := newMockRepo()
	if seed != nil {
		seed(repo)
	}
	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
	}, client)
	return NewHTMLHandler(svc, nil)
}

// exportDataRows runs the handler and returns the CSV data rows (header dropped),
// after stripping the BOM.
func exportDataRows(t *testing.T, h *HTMLHandler, query string) (int, [][]string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/export.csv"+query, nil)
	h.HandleExportCSV(rec, req)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	body := bytes.TrimPrefix(rec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF})
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	return rec.Code, records[1:]
}

// oneEUROnMonday seeds a single 80 EUR expense on Monday 2026-02-02.
func oneEUROnMonday(r *mockRepo) {
	r.CreateExpense(context.Background(), domain.Expense{
		Amount: 80, Category: "travel", Description: "hotel", Date: "2026-02-02", Currency: "EUR",
	})
}

// TestHandleExportCSV_ConvertsAtOwnDateRate proves a normal row carries an
// amount converted at its own date's rate, with the rate date surfaced.
func TestHandleExportCSV_ConvertsAtOwnDateRate(t *testing.T) {
	client := &mockRateClient{tsFunc: func(start, end string) map[string]map[string]float64 {
		// 0.8 EUR per USD on the expense's date, so 80 EUR -> 100 USD.
		return map[string]map[string]float64{"2026-02-02": {"USD": 1.0, "EUR": 0.8}}
	}}
	h := exportHandlerWithRates(t, oneEUROnMonday, client)

	code, rows := exportDataRows(t, h, "?from=2026-02-02&to=2026-02-02")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 data row, got %d", len(rows))
	}
	// Columns 7,8,9 are amount_converted, currency_converted, rate_date.
	if got := rows[0]; got[7] != "100" || got[8] != "USD" || got[9] != "2026-02-02" {
		t.Fatalf("converted columns = %q/%q/%q, want 100/USD/2026-02-02", got[7], got[8], got[9])
	}
}

// TestHandleExportCSV_UnconvertibleCurrencyBlankNot503 proves a row whose
// currency the obtained rate set doesn't cover leaves amount_converted and
// rate_date blank (currency_converted still present) — a clean 200, not a 503.
func TestHandleExportCSV_UnconvertibleCurrencyBlankNot503(t *testing.T) {
	client := &mockRateClient{tsFunc: func(start, end string) map[string]map[string]float64 {
		// The date's rates are present but don't cover ZZZ.
		return map[string]map[string]float64{"2026-02-02": {"USD": 1.0, "EUR": 0.8}}
	}}
	h := exportHandlerWithRates(t, func(r *mockRepo) {
		r.CreateExpense(context.Background(), domain.Expense{
			Amount: 9.99, Category: "misc", Description: "odd", Date: "2026-02-02", Currency: "ZZZ",
		})
	}, client)

	code, rows := exportDataRows(t, h, "?from=2026-02-02&to=2026-02-02")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unconvertible must not fail)", code)
	}
	if got := rows[0]; got[7] != "" || got[9] != "" {
		t.Fatalf("amount_converted/rate_date = %q/%q, want both blank", got[7], got[9])
	}
	if got := rows[0]; got[8] != "USD" {
		t.Fatalf("currency_converted = %q, want USD present on every row", got[8])
	}
}

// TestHandleExportCSV_RateProviderDown503 proves a down provider fails the export
// with a 503 and no file when a row actually needs converting.
func TestHandleExportCSV_RateProviderDown503(t *testing.T) {
	client := &mockRateClient{tsErr: errors.New("provider down")}
	h := exportHandlerWithRates(t, oneEUROnMonday, client)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/export.csv?from=2026-02-02&to=2026-02-02", nil)
	h.HandleExportCSV(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("a 503 must not stream a CSV (found the BOM)")
	}
}

func TestHandleExportCSV_StatusOrder(t *testing.T) {
	h := exportHandler(t, nil)
	cases := []struct {
		name, query string
		want        int
	}{
		{"unparseable from", "?from=banana&to=2026-01-01", http.StatusBadRequest},
		{"inverted range", "?from=2026-02-01&to=2026-01-01", http.StatusBadRequest},
		{"zero matches header-only", "?from=2020-01-01&to=2020-01-02", http.StatusOK},
		{"bare exports everything", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/export.csv"+tc.query, nil)
			h.HandleExportCSV(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body: %q)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestHandleExportCSV_Filename(t *testing.T) {
	h := exportHandler(t, nil)
	h.now = func() time.Time { return time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC) }

	cases := []struct{ query, want string }{
		{"?from=2026-01-01&to=2026-07-15", `attachment; filename="expensif-2026-01-01_2026-07-15.csv"`},
		{"", `attachment; filename="expensif-expenses-2026-07-16.csv"`},
		{"?from=2026-01-01", `attachment; filename="expensif-expenses-2026-07-16.csv"`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/export.csv"+tc.query, nil)
		h.HandleExportCSV(rec, req)
		if got := rec.Header().Get("Content-Disposition"); got != tc.want {
			t.Errorf("query %q: Content-Disposition = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestHandleExportCSV_ZeroMatchesHeaderOnly(t *testing.T) {
	h := exportHandler(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/export.csv?from=2020-01-01&to=2020-01-02", nil)
	h.HandleExportCSV(rec, req)

	body := bytes.TrimPrefix(rec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF})
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("zero matches should be header-only, got %d records: %v", len(records), records)
	}
}

// TestHandleExportCSV_HostileFieldRoundTrip proves the real bytes: BOM strip,
// CRLF terminators, and RFC-4180 quoting/escaping of a field carrying a comma, a
// double-quote, and an embedded newline.
func TestHandleExportCSV_HostileFieldRoundTrip(t *testing.T) {
	hostile := "He said \"hi\", then left\nand paid"
	h := exportHandler(t, func(r *mockRepo) {
		r.CreateExpense(context.Background(), domain.Expense{
			Amount: 5, Category: "misc", Description: hostile, Date: "2026-03-01", Currency: "USD",
		})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/export.csv?from=2026-03-01&to=2026-03-01", nil)
	h.HandleExportCSV(rec, req)

	raw := rec.Body.Bytes()
	if !bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("missing UTF-8 BOM")
	}
	if !bytes.Contains(raw, []byte("\r\n")) {
		t.Fatalf("expected CRLF row terminators")
	}

	body := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header + 1 row, got %d records", len(records))
	}
	// description is column index 3.
	if got := records[1][3]; got != hostile {
		t.Fatalf("hostile field did not round-trip:\n got: %q\nwant: %q", got, hostile)
	}
}

func TestHandleExportCSV_CategoryAndPayerFilter(t *testing.T) {
	h := exportHandler(t, func(r *mockRepo) {
		alice, _ := r.CreateUser(context.Background(), "Alice")
		bob, _ := r.CreateUser(context.Background(), "Bob")
		ctx := context.Background()
		r.CreateExpense(ctx, domain.Expense{Amount: 1, Category: "food", Date: "2026-04-01", Currency: "USD", PaidByID: alice})
		r.CreateExpense(ctx, domain.Expense{Amount: 2, Category: "travel", Date: "2026-04-02", Currency: "USD", PaidByID: alice})
		r.CreateExpense(ctx, domain.Expense{Amount: 3, Category: "food", Date: "2026-04-03", Currency: "USD", PaidByID: bob})
	})

	dataRows := func(query string) [][]string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/export.csv"+query, nil)
		h.HandleExportCSV(rec, req)
		body := bytes.TrimPrefix(rec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF})
		records, _ := csv.NewReader(bytes.NewReader(body)).ReadAll()
		return records[1:] // drop header
	}

	if got := dataRows("?category=food"); len(got) != 2 {
		t.Errorf("category=food: expected 2 rows, got %d", len(got))
	}
	// PayerID for Alice is 1 (first created user).
	if got := dataRows("?payer=1"); len(got) != 2 {
		t.Errorf("payer=1 (Alice): expected 2 rows, got %d", len(got))
	}
	if got := dataRows("?category=food&payer=1"); len(got) != 1 {
		t.Errorf("category=food&payer=1: expected 1 row, got %d", len(got))
	}
	if got := dataRows(""); len(got) != 3 {
		t.Errorf("bare: expected all 3 rows, got %d", len(got))
	}
}

// previewFixture builds an export handler over a real renderer (the preview
// endpoint renders a fragment) so the thin status/shape checks below run against
// the actual partial.
func previewFixture(t *testing.T, seed func(*mockRepo)) *HTMLHandler {
	t.Helper()
	repo := newMockRepo()
	if seed != nil {
		seed(repo)
	}
	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
	}, &mockRateClient{})
	renderer, err := NewRenderer("../../templates", false, nil)
	if err != nil {
		t.Fatalf("renderer init failed: %v", err)
	}
	return NewHTMLHandler(svc, renderer)
}

// TestHandleExportPreview asserts the preview endpoint's status/shape contract —
// not its markup: a valid filter returns a 200 fragment whose count reflects the
// matches, a zero-match filter is still a 200 (agreeing with the header-only
// download), and a bad or inverted range is the soft 400 the island shows as a
// hint. The preview does no rate work, so it can never 503.
func TestHandleExportPreview(t *testing.T) {
	h := previewFixture(t, func(r *mockRepo) {
		ctx := context.Background()
		r.CreateExpense(ctx, domain.Expense{Amount: 1, Category: "food", Description: "lunch", Date: "2026-04-01", Currency: "USD"})
		r.CreateExpense(ctx, domain.Expense{Amount: 2, Category: "travel", Description: "cab", Date: "2026-04-02", Currency: "USD"})
	})

	serve := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.HandleExportPreview(rec, httptest.NewRequest(http.MethodGet, "/export/preview"+query, nil))
		return rec
	}

	t.Run("valid filter returns fragment with the matches", func(t *testing.T) {
		rec := serve("?from=2026-04-01&to=2026-04-30")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "lunch") || !strings.Contains(body, "cab") {
			t.Errorf("expected both sample rows in fragment, got: %s", body)
		}
	})

	t.Run("zero matches is a 200", func(t *testing.T) {
		rec := serve("?from=2020-01-01&to=2020-12-31")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for zero matches, got %d", rec.Code)
		}
	})

	t.Run("bad and inverted ranges are a soft 400", func(t *testing.T) {
		for _, q := range []string{"?from=nonsense", "?from=2026-05-01&to=2026-04-01"} {
			if rec := serve(q); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d", q, rec.Code)
			}
		}
	})
}
