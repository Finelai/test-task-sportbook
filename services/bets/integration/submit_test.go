//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/bets/internal/postgres"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestBetIdempotency(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "bets"}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	s, err := postgres.Open(ctx, "postgres://postgres:test@"+host+":"+port.Port()+"/bets?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(s)
	send := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/bets", bytes.NewBufferString(body))
		req.Header.Set("Idempotency-Key", "key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	const body = `{"offerId":"offer-1","odds":"2.10","amount":"3.00"}`
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 12)
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = send(body) }(i)
	}
	wg.Wait()
	var first string
	for _, r := range responses {
		var v struct {
			BetID string `json:"betId"`
		}
		if r.Code != 202 || json.Unmarshal(r.Body.Bytes(), &v) != nil || v.BetID == "" {
			t.Fatalf("response %d %s", r.Code, r.Body.String())
		}
		if first == "" {
			first = v.BetID
		}
		if v.BetID != first {
			t.Fatalf("different ids %s and %s", first, v.BetID)
		}
	}
	for _, b := range []string{`{"offerId":"offer-2","odds":"2.10","amount":"3.00"}`, `{"offerId":"offer-1","odds":"2.11","amount":"3.00"}`, `{"offerId":"offer-1","odds":"2.10","amount":"3.01"}`} {
		r := send(b)
		if r.Code != 409 {
			t.Fatalf("conflict: %d %s", r.Code, r.Body.String())
		}
	}
	firstLease, err := s.Claim(ctx, 30*time.Millisecond)
	if err != nil || firstLease == nil {
		t.Fatalf("first claim: %v %v", firstLease, err)
	}
	second, err := s.Claim(ctx, time.Second)
	if err != nil || second != nil {
		t.Fatalf("lease ignored: %v %v", second, err)
	}
	time.Sleep(60 * time.Millisecond)
	second, err = s.Claim(ctx, time.Second)
	if err != nil || second == nil || second.Token == firstLease.Token {
		t.Fatalf("second claim: %v %v", second, err)
	}
	firstLease.Status = "accepted"
	updated, err := s.Save(ctx, firstLease)
	if err != nil || updated {
		t.Fatalf("stale lease saved: %v %v", updated, err)
	}
	second.Status = "rejected"
	second.Reason = "market_closed"
	updated, err = s.Save(ctx, second)
	if err != nil || !updated {
		t.Fatalf("current lease not saved: %v %v", updated, err)
	}

}
