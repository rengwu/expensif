package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"

	"expensif/internal/assets"
	"expensif/internal/domain"
)

type PageData struct {
	Active              string
	Flash               string
	FlashError          bool
	Expenses            []domain.Expense
	Expense             *domain.Expense
	Total               float64
	Categories          []domain.CategorySummary
	Today               string
	DailyGroups         []domain.DailyGroup
	CalendarMonths      []domain.CalendarMonth
	CurrencySymbol      string
	Currency            string
	ConvertedTotal      float64
	RateDate            string
	ShowConverted       bool
	UserID              int64
	Users               []domain.User
	User                *domain.User
	PaidByID            int64
	Timezone            string
	FilterDate          string
	BackHref            string
	BackLabel           string
	NoExpensesEver      bool     // the account has never recorded an expense (ever, not "in this window")
	ReturnTo            string   // where delete sends the user next; validated as a local path before the redirect
	HiddenUpcoming      int      // upcoming days beyond the 3 nearest today, collapsed into the overflow row
	HiddenUpcomingTotal float64  // the collapsed days' combined total, in the Preferred Currency
	NextStart           string   // the scroll cursor: the window older than this one. Empty at the earliest
	NextEnd             string   // expense — that absence is the scroll's terminal, and the island reads no other stop condition
	PrevDate            string   // fragments only: the day directly above the seam, so the top divider knows whether it is a month break
	Islands             []string // Names of island entries to load on this page
}

// yearMonth is the YYYY-MM prefix of a date string. Rows written before date
// validation landed can carry garbage shorter than seven bytes, so slice
// defensively rather than panic.
func yearMonth(date string) string {
	if len(date) < 7 {
		return date
	}
	return date[:7]
}

// tzLocation resolves a Preferences timezone name, falling back to the
// server's local zone when it is empty or unknown.
func tzLocation(tz string) *time.Location {
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return time.Local
}

type Renderer struct {
	templates map[string]*template.Template
	fragments map[string]*template.Template
}

func NewRenderer(templatesDir string, dev bool, manifest assets.Manifest) (*Renderer, error) {
	helper := &assets.AssetHelper{Dev: dev, Manifest: manifest}

	funcMap := template.FuncMap{
		"default": func(def, v interface{}) interface{} {
			switch val := v.(type) {
			case string:
				if val == "" {
					return def
				}
			case nil:
				return def
			}
			return v
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"humanDate": func(dateStr, tz string) string {
			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return dateStr
			}
			now := time.Now().In(tzLocation(tz))
			todayStr := now.Format("2006-01-02")
			if dateStr == todayStr {
				return "Today"
			}
			yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
			if dateStr == yesterdayStr {
				return "Yesterday"
			}
			return humanize.Time(t)
		},
		"formatDate": func(dateStr, tz string) string {
			t, err := time.ParseInLocation("2006-01-02", dateStr, tzLocation(tz))
			if err != nil {
				return dateStr
			}
			return t.Format("Jan 2, Monday")
		},
		"shortDate": func(dateStr, tz string) string {
			t, err := time.ParseInLocation("2006-01-02", dateStr, tzLocation(tz))
			if err != nil {
				return dateStr
			}
			return t.Format("Jan 2 · Mon")
		},
		// monthBreak reports whether the divider between two adjacent ledger
		// days — prev drawn directly above cur — is a month boundary, and so
		// draws heavier. prev is empty at the top of the page; at the top of an
		// appended fragment it is the day above the seam, so one rule covers
		// both within a window and across two.
		"monthBreak": func(prev, cur string) bool {
			return prev != "" && yearMonth(prev) != yearMonth(cur)
		},
		"currencySymbol": domain.CurrencySymbol,
		"formatAmount":   domain.FormatAmount,
		"currencyDecimalsJSON": func() template.JS {
			b, err := json.Marshal(domain.AllCurrencyDecimals())
			if err != nil {
				return template.JS("{}")
			}
			return template.JS(b)
		},
		"script":    func(entry string) template.HTML { return helper.ScriptTag(entry) },
		"devClient": func() template.HTML { return helper.DevClient() },
		"safeHTML":  func(s string) template.HTML { return template.HTML(s) },
		"json": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"list": func(items ...interface{}) []interface{} {
			return items
		},
		"jsonSafe": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
	}

	partialFiles, _ := filepath.Glob(filepath.Join(templatesDir, "partials", "*.html"))

	parsePage := func(files ...string) *template.Template {
		t := template.New("").Funcs(funcMap)
		allFiles := append([]string{filepath.Join(templatesDir, "base.html")}, files...)
		allFiles = append(allFiles, partialFiles...)
		return template.Must(t.ParseFiles(allFiles...))
	}

	// A fragment is a page without the page: no base layout, since it is
	// appended into a document that already exists. It draws through the same
	// partials, which is the whole point — the ledger has one implementation.
	parseFragment := func(file string) *template.Template {
		t := template.New("").Funcs(funcMap)
		return template.Must(t.ParseFiles(append([]string{file}, partialFiles...)...))
	}

	templates := map[string]*template.Template{
		"list":      parsePage(filepath.Join(templatesDir, "list.html")),
		"daily":     parsePage(filepath.Join(templatesDir, "daily.html")),
		"add":       parsePage(filepath.Join(templatesDir, "form.html"), filepath.Join(templatesDir, "add.html")),
		"edit":      parsePage(filepath.Join(templatesDir, "form.html"), filepath.Join(templatesDir, "edit.html")),
		"prefs":     parsePage(filepath.Join(templatesDir, "preferences.html")),
		"users":     parsePage(filepath.Join(templatesDir, "users.html")),
		"user_form": parsePage(filepath.Join(templatesDir, "user_form.html")),
		"calendar":  parsePage(filepath.Join(templatesDir, "calendar.html")),
		"export":    parsePage(filepath.Join(templatesDir, "export.html")),
	}

	fragments := map[string]*template.Template{
		"daily-older": parseFragment(filepath.Join(templatesDir, "daily-older.html")),
		// The Export preview is the same partial the /export page renders on first
		// paint, served standalone so the input-driven island can re-fetch it.
		"export-preview": parseFragment(filepath.Join(templatesDir, "partials", "export-preview.html")),
	}

	return &Renderer{templates: templates, fragments: fragments}, nil
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data PageData) error {
	return r.RenderPage(w, name, data)
}

// RenderPage renders a page template through the base layout with an arbitrary
// data value, for pages (like Export) whose view model embeds PageData and adds
// its own fields. The base layout reads only the promoted PageData fields.
func (r *Renderer) RenderPage(w http.ResponseWriter, name string, data any) error {
	t, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return t.ExecuteTemplate(w, "base", data)
}

// RenderFragment writes a bare slice of HTML — no base layout — for a caller
// that appends it into a page already on screen. The data value is arbitrary so
// a fragment (like the Export preview) can carry its own view model rather than
// the shared PageData.
func (r *Renderer) RenderFragment(w http.ResponseWriter, name string, data any) error {
	t, ok := r.fragments[name]
	if !ok {
		return fmt.Errorf("fragment %q not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return t.ExecuteTemplate(w, name, data)
}
