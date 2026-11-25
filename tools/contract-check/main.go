// Command contract-check serves fixed examples for local contract inspection.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/line", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"version": 1,
			"matches": []any{map[string]any{
				"matchId": "match-1", "homeTeam": "Home FC", "awayTeam": "Away FC", "open": true,
				"quotes": []any{map[string]string{"outcome": "HOME", "odds": "2.10"}, map[string]string{"outcome": "DRAW", "odds": "3.20"}, map[string]string{"outcome": "AWAY", "odds": "3.50"}},
			}},
		}); err != nil {
			log.Printf("write snapshot: %v", err)
		}
	})
	mux.HandleFunc("GET /api/v1/line/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		_, err := fmt.Fprint(w, "event: snapshot\ndata: {\"version\":1,\"matches\":[{\"matchId\":\"match-1\",\"homeTeam\":\"Home FC\",\"awayTeam\":\"Away FC\",\"open\":true,\"quotes\":[{\"outcome\":\"HOME\",\"odds\":\"2.10\"},{\"outcome\":\"DRAW\",\"odds\":\"3.20\"},{\"outcome\":\"AWAY\",\"odds\":\"3.50\"}]}]}\n\n")
		if err != nil {
			return
		}
		flusher.Flush()
		_, err = fmt.Fprint(w, "event: match_updated\ndata: {\"version\":2,\"match\":{\"matchId\":\"match-1\",\"homeTeam\":\"Home FC\",\"awayTeam\":\"Away FC\",\"open\":true,\"quotes\":[{\"outcome\":\"HOME\",\"odds\":\"2.20\"},{\"outcome\":\"DRAW\",\"odds\":\"3.20\"},{\"outcome\":\"AWAY\",\"odds\":\"3.50\"}]}}\n\n")
		if err != nil {
			return
		}
		flusher.Flush()
		<-r.Context().Done()
	})
	mux.HandleFunc("POST /api/v1/line/offers", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"offerId": "offer-1", "matchId": "match-1", "outcome": "HOME",
			"odds": "2.10", "expiresAt": "2030-01-01T00:01:00Z",
		}); err != nil {
			log.Printf("write offer: %v", err)
		}
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
