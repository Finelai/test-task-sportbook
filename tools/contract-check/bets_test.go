package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBet(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status int
		fields map[string]string
	}{
		{"bet-1", http.StatusOK, map[string]string{"betId": "bet-1", "status": "pending", "amount": "3.00", "odds": "2.10"}},
		{"unknown", http.StatusNotFound, map[string]string{"code": "not_found", "message": "Bet not found"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/bets/"+tc.id, nil)
			req.SetPathValue("betId", tc.id)
			rec := httptest.NewRecorder()
			getBet(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("content type = %q", got)
			}
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.fields {
				if got[key] != want {
					t.Errorf("%s = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}
