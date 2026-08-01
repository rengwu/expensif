package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

type Server struct {
	srv *http.Server
}

func NewServer(api *APIHandler, html *HTMLHandler, port string) *Server {
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: RecoverPanic(RequestLog(routes(api, html))),
	}
	return &Server{srv: srv}
}

// routes is the app's routing table, apart from the server that listens on it,
// so a test can drive real requests — path values and method matching included —
// through the same mux production runs.
func routes(api *APIHandler, html *HTMLHandler) http.Handler {
	mux := http.NewServeMux()

	// Static assets (Vite build output)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// HTML routes
	mux.HandleFunc("GET /{$}", html.HandleDaily)
	mux.HandleFunc("GET /daily/older", html.HandleDailyOlder)
	mux.HandleFunc("GET /calendar", html.HandleCalendar)
	mux.HandleFunc("GET /expenses", html.HandleList)
	mux.HandleFunc("GET /export", html.HandleExportPage)
	mux.HandleFunc("GET /export.csv", html.HandleExportCSV)
	mux.HandleFunc("GET /export/preview", html.HandleExportPreview)
	mux.HandleFunc("GET /preferences", html.HandlePreferences)
	mux.HandleFunc("POST /preferences", html.HandleSavePreferences)
	mux.HandleFunc("GET /expenses/new", html.HandleAdd)
	mux.HandleFunc("POST /expenses/new", html.HandleCreate)
	mux.HandleFunc("GET /expenses/edit/{id}", html.HandleEdit)
	mux.HandleFunc("POST /expenses/edit/{id}", html.HandleUpdate)
	mux.HandleFunc("POST /expenses/delete/{id}", html.HandleDelete)

	mux.HandleFunc("GET /recurring", html.HandleRecurring)
	mux.HandleFunc("GET /recurring/new", html.HandleRecurringNew)
	mux.HandleFunc("POST /recurring/new", html.HandleRecurringCreate)
	mux.HandleFunc("GET /recurring/edit/{id}", html.HandleRecurringEdit)
	mux.HandleFunc("POST /recurring/edit/{id}", html.HandleRecurringUpdate)
	mux.HandleFunc("POST /recurring/toggle/{id}", html.HandleRecurringToggle)
	mux.HandleFunc("POST /recurring/delete/{id}", html.HandleRecurringDelete)

	mux.HandleFunc("GET /users", html.HandleUsers)
	mux.HandleFunc("GET /users/new", html.HandleUserNew)
	mux.HandleFunc("POST /users/new", html.HandleUserCreate)
	mux.HandleFunc("GET /users/edit/{id}", html.HandleUserEdit)
	mux.HandleFunc("POST /users/edit/{id}", html.HandleUserUpdate)
	mux.HandleFunc("POST /users/delete/{id}", html.HandleUserDelete)

	// JSON API routes
	mux.HandleFunc("GET /api/expenses", api.HandleList)
	mux.HandleFunc("POST /api/expenses", api.HandleCreate)
	mux.HandleFunc("GET /api/expenses/{id}", api.HandleGet)
	mux.HandleFunc("PUT /api/expenses/{id}", api.HandleUpdate)
	mux.HandleFunc("DELETE /api/expenses/{id}", api.HandleDelete)
	mux.HandleFunc("GET /api/categories", api.HandleCategories)
	mux.HandleFunc("GET /api/expenses/descriptions", api.HandleDescriptions)
	mux.HandleFunc("GET /api/summary", api.HandleSummary)

	return mux
}

func (s *Server) Run() error {
	slog.Info("server running", "url", fmt.Sprintf("http://localhost%s", s.srv.Addr))
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
