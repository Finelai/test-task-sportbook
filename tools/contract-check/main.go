// Command contract-check serves fixed examples for local contract inspection.
package main

import (
	"encoding/json"
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
	log.Fatal(http.ListenAndServe(":8080", mux))
}
