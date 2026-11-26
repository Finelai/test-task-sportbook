package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type testStore struct{ calls int }

func (s *testStore) Get(context.Context, string) (Bet, error) { return Bet{}, ErrNotFound }
func (s *testStore) Ping(context.Context) error               { return nil }
func (s *testStore) Submit(_ context.Context, key, offer string, amount, odds int64) (Bet, error) {
	s.calls++
	return Bet{ID: "bet-1", Status: "pending", Amount: amount, Odds: odds}, nil
}

func TestInvalidRequestsDoNotReachStore(t *testing.T) {
	cases := []struct{ key, body string }{
		{"", `{"offerId":"offer-1","odds":"2.10","amount":"3.00"}`},
		{"key", `{"offerId":"","odds":"2.10","amount":"3.00"}`},
		{"key", `{"offerId":"offer-1","odds":"0","amount":"3.00"}`},
		{"key", `{"offerId":"offer-1","odds":"2.10","amount":"-1"}`},
		{"key", `{"offerId":"offer-1","odds":"2.101","amount":"3.00"}`},
		{"key", `{"offerId":"offer-1","odds":"2.10","amount":"92233720368547758.08"}`},
		{"key", `{"offerId":"offer-1","odds":"2.10","amount":"3.00"} {}`},
		{"key", `{"offerId":"offer-1","odds":2.1,"amount":"3.00"}`},
	}
	for _, tc := range cases {
		t.Run(tc.key+tc.body, func(t *testing.T) {
			s := &testStore{}
			req := httptest.NewRequest("POST", "/api/v1/bets", strings.NewReader(tc.body))
			req.Header.Set("Idempotency-Key", tc.key)
			w := httptest.NewRecorder()
			New(s).ServeHTTP(w, req)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) || s.calls != 0 {
				t.Fatalf("status=%d body=%s calls=%d", w.Code, w.Body.String(), s.calls)
			}
		})
	}
}

func TestSubmitReturnsCanonicalAmountAndOdds(t *testing.T) {
	s := &testStore{}
	req := httptest.NewRequest("POST", "/api/v1/bets", strings.NewReader(`{"offerId":"offer-1","odds":"2.1","amount":"3"}`))
	req.Header.Set("Idempotency-Key", "key")
	w := httptest.NewRecorder()
	New(s).ServeHTTP(w, req)
	if w.Code != 202 || !strings.Contains(w.Body.String(), `"amount":"3.00"`) || !strings.Contains(w.Body.String(), `"odds":"2.10"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
