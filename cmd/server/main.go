// Command server is the entry point of the ticket reservation service.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			logger.Error("failed to write health response", "err", err)
		}
	})

	addr := ":8080"
	logger.Info("starting server", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
