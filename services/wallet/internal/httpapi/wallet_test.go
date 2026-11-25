package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeStore struct{ err error }

func (s fakeStore) Balance(context.Context, string) (int64, int64, error) { return 0, 0, s.err }
func (s fakeStore) Ping(context.Context) error                            { return s.err }
func TestInitialBalance(t *testing.T) {
	h := New(fakeStore{})
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/wallet", nil))
	if r.Code != 200 || r.Body.String() != "{\"available\":\"0.00\",\"reserved\":\"0.00\"}\n" {
		t.Fatalf("response %d %s", r.Code, r.Body.String())
	}
}
func TestUnavailable(t *testing.T) {
	h := New(fakeStore{errors.New("database unavailable")})
	for _, path := range []string{"/api/v1/wallet", "/readyz"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != 503 {
			t.Fatalf("%s: %d", path, r.Code)
		}
	}
}
