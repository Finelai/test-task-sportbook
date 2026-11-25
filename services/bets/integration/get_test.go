//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/bets/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestGetBet(t *testing.T) {
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
	url := "postgres://postgres:test@" + host + ":" + port.Port() + "/bets?sslmode=disable"
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	bet, err := store.Submit(ctx, "get-key", "offer", 400, 395)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(store)
	get := func(id string) (int, map[string]string) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bets/"+id, nil))
		var result map[string]string
		if e := json.Unmarshal(rec.Body.Bytes(), &result); e != nil {
			t.Fatalf("decode %d %q: %v", rec.Code, rec.Body.String(), e)
		}
		return rec.Code, result
	}
	for _, tc := range []struct{ status, reason string }{{"pending", ""}, {"accepted", ""}, {"rejected", "insufficient_funds"}} {
		if _, err = db.Exec(ctx, `UPDATE bets SET status=$2,reason=$3 WHERE id=$1`, bet.ID, tc.status, tc.reason); err != nil {
			t.Fatal(err)
		}
		code, body := get(bet.ID)
		if code != 200 || body["betId"] != bet.ID || body["status"] != tc.status || body["amount"] != "4.00" || body["odds"] != "3.95" || body["reason"] != tc.reason {
			t.Fatalf("status=%d body=%v", code, body)
		}
		if tc.reason == "" {
			if _, ok := body["reason"]; ok {
				t.Fatalf("unexpected reason: %v", body)
			}
		}
	}
	if _, err = db.Exec(ctx, `INSERT INTO bets(id,player_id,idempotency_key,offer_id,amount,odds) VALUES ('other-bet','other-player','key','offer',100,200)`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"not-found", "other-bet"} {
		code, body := get(id)
		if code != 404 || body["code"] != "not_found" {
			t.Fatalf("id=%s status=%d body=%v", id, code, body)
		}
	}
	store.Close()
	code, body := get(bet.ID)
	if code != 503 || body["code"] != "unavailable" {
		t.Fatalf("unavailable status=%d body=%v", code, body)
	}
}
