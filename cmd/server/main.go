package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"expensif/internal/assets"
	"expensif/internal/db"
	"expensif/internal/rate"
	"expensif/internal/repository"
	"expensif/internal/service"
	"expensif/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	database, err := db.New()
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	defer database.Close()

	repo := repository.NewSQLite(database)
	svc := service.New(repository.Repos{
		Expenses:    repo,
		Users:       repo,
		Preferences: repo,
		Rates:       repo,
		Rules:       repo,
	}, rate.NewClient())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background rate refresh: fetch immediately, then every 6 hours
	go func() {
		for {
			if err := svc.RefreshRates(ctx); err != nil {
				slog.Error("rate refresh failed", "error", err)
			} else {
				slog.Info("exchange rates refreshed")
			}
			select {
			case <-time.After(6 * time.Hour):
			case <-ctx.Done():
				return
			}
		}
	}()

	dev := os.Getenv("DEV") == "true"

	var manifest assets.Manifest
	if !dev {
		var err error
		manifest, err = assets.LoadManifest("static/.vite/manifest.json")
		if err != nil {
			log.Fatalf("failed to load asset manifest: %v", err)
		}
	}

	renderer, err := web.NewRenderer("templates", dev, manifest)
	if err != nil {
		log.Fatalf("template init failed: %v", err)
	}

	apiHandler := web.NewAPIHandler(svc)
	htmlHandler := web.NewHTMLHandler(svc, renderer)

	// Recurring generation: mint what is owed now — closing any gap this process
	// was down for — then on the generator's ticker. It is the only caller of
	// generation in the running app, so no two runs can ever overlap, and it
	// leaves its count on the handler's notice for the next page to flash.
	go web.NewGenerator(svc, htmlHandler.Notice()).Run(ctx)

	port := os.Getenv("PORT")
	server := web.NewServer(apiHandler, htmlHandler, port)

	srvErr := make(chan error, 1)
	go func() {
		if err := server.Run(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		slog.Info("shutting down server...")
	case err := <-srvErr:
		log.Fatalf("server error: %v", err)
	}

	cancel() // stop background goroutines

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
	slog.Info("server stopped")
}
