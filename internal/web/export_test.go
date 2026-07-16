package web

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	return NewHTMLHandler(svc, nil)
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
