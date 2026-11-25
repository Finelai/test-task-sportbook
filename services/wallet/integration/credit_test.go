//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/wallet/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type creditFixture struct {
	store *postgres.Store
	conn  *pgx.Conn
	url   string
}

func newCreditFixture(t *testing.T) *creditFixture {
	t.Helper()
	ctx := context.Background()
	c, e := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "wallet"}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)}, Started: true})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, e := c.Host(ctx)
	if e != nil {
		t.Fatal(e)
	}
	port, e := c.MappedPort(ctx, "5432")
	if e != nil {
		t.Fatal(e)
	}
	url := "postgres://postgres:test@" + host + ":" + port.Port() + "/wallet?sslmode=disable"
	store, e := postgres.Open(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(store.Close)
	conn, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return &creditFixture{store, conn, url}
}
func credit(h http.Handler, key, amount string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"amount": amount})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/credits", bytes.NewReader(body))
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestCreditIdempotent(t *testing.T) {
	f := newCreditFixture(t)
	h := httpapi.New(f.store)
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, 10)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = credit(h, "credit-1", "10.00") }(i)
	}
	wg.Wait()
	for _, r := range results {
		if r.Code != 201 || r.Body.String() != results[0].Body.String() {
			t.Fatalf("response %d %s", r.Code, r.Body.String())
		}
	}
	var result struct {
		OperationID string `json:"operationId"`
		Available   string `json:"available"`
		Reserved    string `json:"reserved"`
	}
	if e := json.Unmarshal(results[0].Body.Bytes(), &result); e != nil || result.OperationID == "" || result.Available != "10.00" || result.Reserved != "0.00" {
		t.Fatalf("result %+v %v", result, e)
	}
	r := credit(h, "credit-1", "11.00")
	if r.Code != 409 {
		t.Fatalf("conflict %d %s", r.Code, r.Body.String())
	}
	var available int64
	var entries int
	if e := f.conn.QueryRow(context.Background(), "SELECT available,(SELECT count(*) FROM wallet_journal) FROM wallets WHERE player_id='player-1'").Scan(&available, &entries); e != nil || available != 1000 || entries != 1 {
		t.Fatalf("available=%d entries=%d error=%v", available, entries, e)
	}
	f.store.Close()
	store, e := postgres.Open(context.Background(), f.url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	r = credit(httpapi.New(store), "credit-1", "10.00")
	if r.Code != 201 || r.Body.String() != results[0].Body.String() {
		t.Fatalf("restart %d %s", r.Code, r.Body.String())
	}
}
func TestInvalidAmount(t *testing.T) {
	f := newCreditFixture(t)
	h := httpapi.New(f.store)
	for i, amount := range []string{"0", "-1", "1.001", "abc", "92233720368547758.08", "1.", ".01"} {
		r := credit(h, fmt.Sprint(i), amount)
		if r.Code != 400 {
			t.Fatalf("amount %q: %d %s", amount, r.Code, r.Body.String())
		}
	}
	if r := credit(h, "", "1.00"); r.Code != 400 {
		t.Fatalf("missing key: %d", r.Code)
	}
	r := credit(h, "small", "0.01")
	if r.Code != 201 {
		t.Fatalf("small %d %s", r.Code, r.Body.String())
	}
	r = credit(h, "large", "92233720368547758.06")
	if r.Code != 201 {
		t.Fatalf("large %d %s", r.Code, r.Body.String())
	}
	r = credit(h, "overflow", "0.01")
	if r.Code != 400 {
		t.Fatalf("overflow %d %s", r.Code, r.Body.String())
	}
	var available, reserved int64
	var entries int
	if e := f.conn.QueryRow(context.Background(), "SELECT available,reserved,(SELECT count(*) FROM wallet_journal) FROM wallets WHERE player_id='player-1'").Scan(&available, &reserved, &entries); e != nil || available != 9223372036854775807 || reserved != 0 || entries != 2 {
		t.Fatalf("balances %d %d entries %d error %v", available, reserved, entries, e)
	}
}

func TestCreditOverflowWithReserved(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	_, err := f.conn.Exec(ctx, `INSERT INTO wallets(player_id,available,reserved) VALUES ('player-1', 9223372036854775800, 7)`)
	if err != nil {
		t.Fatal(err)
	}
	r := credit(httpapi.New(f.store), "overflow-reserved", "0.01")
	if r.Code != 400 {
		t.Fatalf("response %d %s", r.Code, r.Body.String())
	}
	var available, reserved int64
	var entries int
	err = f.conn.QueryRow(ctx, `SELECT available,reserved,(SELECT count(*) FROM wallet_journal) FROM wallets WHERE player_id='player-1'`).Scan(&available, &reserved, &entries)
	if err != nil || available != 9223372036854775800 || reserved != 7 || entries != 0 {
		t.Fatalf("available=%d reserved=%d entries=%d err=%v", available, reserved, entries, err)
	}
}
