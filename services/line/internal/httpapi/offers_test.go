package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
)

func TestOfferHTTP(t *testing.T) {
	db := &store{}
	service := app.New(db)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := service.Generate(context.Background(), i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	h := New(service, db)
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/api/v1/line/offers", bytes.NewBufferString(body)))
		return r
	}
	a := request(`{"matchId":"match-1","outcome":"HOME"}`)
	if a.Code != 200 {
		t.Fatalf("issue: %d %s", a.Code, a.Body.String())
	}
	var first map[string]any
	if err := json.Unmarshal(a.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first["offerId"] == "" || first["odds"] == "" || first["expiresAt"] == "" || first["matchId"] != "match-1" || first["outcome"] != "HOME" {
		t.Fatalf("response: %v", first)
	}
	b := request(`{"matchId":"match-1","outcome":"HOME"}`)
	var repeat map[string]any
	_ = json.Unmarshal(b.Body.Bytes(), &repeat)
	if b.Code != 200 || repeat["offerId"] != first["offerId"] || repeat["expiresAt"] != first["expiresAt"] {
		t.Fatalf("repeat: %d %v", b.Code, repeat)
	}
	for _, body := range []string{`{"matchId":"match-1","outcome":"BAD"}`, `{"matchId":"unknown","outcome":"HOME"}`, `{`, `{"matchId":"match-1","outcome":"HOME"} {}`} {
		r := request(body)
		if r.Code != 400 {
			t.Fatalf("invalid %q: %d", body, r.Code)
		}
	}
	if err := service.Refresh(context.Background(), now.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	r := request(`{"matchId":"match-1","outcome":"HOME"}`)
	if r.Code != 409 {
		t.Fatalf("closed: %d %s", r.Code, r.Body.String())
	}
}
