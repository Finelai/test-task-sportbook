package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
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
	mux.HandleFunc("GET /api/v1/line/events", func(w http.ResponseWriter, r *http.Request) {
		sub, err := service.Subscribe(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "unavailable", "message": "Line is temporarily unavailable"})
			return
		}
		defer sub.Close()
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("X-Accel-Buffering", "no")
		controller := http.NewResponseController(w)
		send := func(name string, value any) error {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		}
		if send("snapshot", sub.Snapshot) != nil {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-sub.Done:
				return
			default:
			}
			select {
			case <-r.Context().Done():
				return
			case <-sub.Done:
				return
			case event := <-sub.Events:
				if send("match_updated", event) != nil {
					return
				}
			}
		}
	})
	return mux
}
