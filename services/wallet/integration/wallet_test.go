//go:build integration

package integration

import (
	"context"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInitialBalance(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "wallet"}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)}, Started: true})
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
	url := "postgres://postgres:test@" + host + ":" + port.Port() + "/wallet?sslmode=disable"
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(store)
	for i := 0; i < 2; i++ {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/wallet", nil))
		if r.Code != 200 || r.Body.String() != "{\"available\":\"0.00\",\"reserved\":\"0.00\"}\n" {
			t.Fatalf("response: %d %s", r.Code, r.Body.String())
		}
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var wallets, entries int
	if err = conn.QueryRow(ctx, "SELECT (SELECT count(*) FROM wallets), (SELECT count(*) FROM wallet_journal)").Scan(&wallets, &entries); err != nil || wallets != 1 || entries != 0 {
		t.Fatalf("wallets=%d journal=%d err=%v", wallets, entries, err)
	}
	store.Close()
	for _, path := range []string{"/api/v1/wallet", "/readyz"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != 503 {
			t.Fatalf("%s: %d", path, r.Code)
		}
		if path == "/api/v1/wallet" && r.Body.String() != "{\"code\":\"unavailable\",\"message\":\"Wallet is temporarily unavailable\"}\n" {
			t.Fatal(r.Body.String())
		}
	}
}
