package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/app"
	"io"
	"net/http"
	"strings"
	"time"
)

type Store interface {
	Balance(context.Context, string) (int64, int64, error)
	Ping(context.Context) error
	Credit(context.Context, string, string, int64) (app.CreditResult, error)
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

	mux.HandleFunc("POST /api/v1/wallet/credits", func(w http.ResponseWriter, r *http.Request) {
		fail := func(status int, code, message string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{code, message})
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			fail(400, "invalid_request", "Invalid request")
			return
		}
		var req struct {
			Amount string `json:"amount"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			fail(400, "invalid_request", "Invalid request")
			return
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			fail(400, "invalid_request", "Invalid request")
			return
		}
		amount, err := app.ParseAmount(req.Amount)
		if err != nil {
			fail(400, "invalid_request", "Invalid request")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		result, err := store.Credit(ctx, "player-1", key, amount)
		if errors.Is(err, app.ErrInvalidAmount) {
			fail(400, "invalid_request", "Invalid request")
			return
		}
		if errors.Is(err, app.ErrConflict) {
			fail(409, "idempotency_conflict", "Idempotency key conflict")
			return
		}
		if err != nil {
			fail(503, "unavailable", "Wallet is temporarily unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			OperationID string `json:"operationId"`
			Available   string `json:"available"`
			Reserved    string `json:"reserved"`
		}{result.OperationID, format(result.Available), format(result.Reserved)})
	})
	return mux
}
func format(n int64) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }
