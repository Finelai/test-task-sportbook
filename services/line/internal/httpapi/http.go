package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
)

type readiness interface{ Ready(context.Context) error }

func New(service *app.Service, db readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if db.Ready(ctx) != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/line", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := service.Snapshot(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "unavailable", "message": "Line is temporarily unavailable"})
			return
		}
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	return mux
}
