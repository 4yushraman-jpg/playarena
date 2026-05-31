package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/4yushraman-jpg/playarena/internal/bootstrap"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/database"
	"github.com/4yushraman-jpg/playarena/internal/platform/logger"
)

func main() {
	os.Exit(run())
}

// run is the real entry point. Returning an int code instead of calling
// os.Exit directly lets deferred cleanup functions execute before the
// process terminates.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	log := logger.New(cfg.AppEnv)

	db, err := database.New(cfg)
	if err != nil {
		log.Error("database connection failed", slog.Any("error", err))
		return 1
	}
	defer db.Close()

	log.Info("database connected",
		slog.String("env", cfg.AppEnv),
		slog.Int("port", cfg.AppPort),
	)

	app := &bootstrap.App{Config: cfg, DB: db, Log: log}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppPort),
		Handler:           app.Handler(),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	// Graceful shutdown on SIGINT (Ctrl-C) or SIGTERM (container stop).
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", slog.String("addr", srv.Addr))
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", slog.Any("error", err))
			return 1
		}
	case sig := <-shutdown:
		log.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", slog.Any("error", err))
		return 1
	}

	log.Info("server stopped cleanly")
	return 0
}
