package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrConflict = errors.New("idempotency conflict")
var ErrNotFound = errors.New("bet not found")

type Bet struct {
	ID, Status, Reason string
	Amount, Odds       int64
}
type Store interface {
	Ping(context.Context) error
	Get(context.Context, string) (Bet, error)
	Submit(context.Context, string, string, int64, int64) (Bet, error)
}

func New(store Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if store.Ping(ctx) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /api/v1/bets/{betId}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		bet, err := store.Get(ctx, r.PathValue("betId"))
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "Bet not found"})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "unavailable", "message": "Service is temporarily unavailable"})
			return
		}
		response := map[string]string{"betId": bet.ID, "status": bet.Status, "amount": fmt.Sprintf("%d.%02d", bet.Amount/100, bet.Amount%100), "odds": fmt.Sprintf("%d.%02d", bet.Odds/100, bet.Odds%100)}
		if bet.Status == "rejected" {
			response["reason"] = bet.Reason
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("POST /api/v1/bets", func(w http.ResponseWriter, r *http.Request) {
		fail := func(status int, code, message string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
		}
		invalid := func() { fail(400, "invalid_request", "Invalid request") }
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			invalid()
			return
		}
		var req struct {
			OfferID string `json:"offerId"`
			Amount  string `json:"amount"`
			Odds    string `json:"odds"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&req) != nil {
			invalid()
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			invalid()
			return
		}
		amount, err := parseDecimal(req.Amount)
		if err != nil {
			invalid()
			return
		}
		odds, err := parseDecimal(req.Odds)
		if err != nil || strings.TrimSpace(req.OfferID) == "" {
			invalid()
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		bet, err := store.Submit(ctx, key, req.OfferID, amount, odds)
		if errors.Is(err, ErrConflict) {
			fail(409, "idempotency_conflict", "Idempotency key conflict")
			return
		}
		if err != nil {
			fail(503, "unavailable", "Service is temporarily unavailable")
			return
		}
		response := map[string]string{"betId": bet.ID, "status": bet.Status, "amount": fmt.Sprintf("%d.%02d", bet.Amount/100, bet.Amount%100), "odds": fmt.Sprintf("%d.%02d", bet.Odds/100, bet.Odds%100)}
		if bet.Status == "rejected" {
			response["reason"] = bet.Reason
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(response)
	})
	return mux
}

func parseDecimal(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || len(parts[0]) == 0 {
		return 0, errors.New("invalid")
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if len(frac) == 0 || len(frac) > 2 {
			return 0, errors.New("invalid")
		}
	}
	for _, c := range parts[0] + frac {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid")
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	n, err := strconv.ParseInt(parts[0]+frac, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid")
	}
	return n, nil
}
