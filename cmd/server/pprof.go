package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"ticket-reservation/internal/config"
)

// startPprof serves the profiler on its own address, or does nothing when none
// is configured.
//
// A separate server rather than a route on the main one, so that the profiler
// can listen on 127.0.0.1 while the API faces the world. Nothing in front of it
// checks anything, because the point is that it is not reachable at all unless
// somebody is already on the machine.
func startPprof(ctx context.Context, cfg config.Config, logger *slog.Logger) func() {
	if cfg.PprofAddr == "" {
		return func() {}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              cfg.PprofAddr,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	logger.WarnContext(ctx, "the profiler is listening",
		"addr", cfg.PprofAddr,
		"why", "it will hand out heap dumps, so keep it off anything public",
	)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the profiler failed", "err", err)
		}
	}()

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = srv.Shutdown(shutdownCtx)
	}
}
