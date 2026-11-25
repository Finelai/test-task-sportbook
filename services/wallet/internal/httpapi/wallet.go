package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Store interface {
	Balance(context.Context, string) (int64, int64, error)
	Ping(context.Context) error
}

func New(store Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if store.Ping(ctx) != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/wallet", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		a, b, err := store.Balance(ctx, "player-1")
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{"unavailable", "Wallet is temporarily unavailable"})
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Available string `json:"available"`
			Reserved  string `json:"reserved"`
		}{format(a), format(b)})
	})
	return mux
}
func format(n int64) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }
