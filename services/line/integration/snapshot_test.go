//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"github.com/finelai/test-task-sportbook/services/line/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/line/internal/postgres"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSnapshot(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "line"}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)}, Started: true})
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
	store, err := postgres.Open(ctx, "postgres://postgres:test@"+host+":"+port.Port()+"/line?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service := app.New(store)
	if err = service.Generate(ctx, 0, time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	if err = service.Generate(ctx, 1, time.Now(), 2); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(service, store)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		return r
	}
	r := request("/api/v1/line")
	if r.Code != 200 {
		t.Fatalf("snapshot: %d %s", r.Code, r.Body.String())
	}
	var snapshot app.Snapshot
	if err = json.Unmarshal(r.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Matches) != 8 {
		t.Fatalf("matches: %d", len(snapshot.Matches))
	}
	for _, m := range snapshot.Matches {
		if !m.Open || len(m.Quotes) != 3 || m.Quotes[0].Outcome != "HOME" || m.Quotes[1].Outcome != "DRAW" || m.Quotes[2].Outcome != "AWAY" || m.Quotes[0].Odds == "" {
			t.Fatalf("match: %+v", m)
		}
	}
	if err = service.Generate(ctx, 0, time.Now(), 3); err != nil {
		t.Fatal(err)
	}
	var next app.Snapshot
	if err = json.Unmarshal(request("/api/v1/line").Body.Bytes(), &next); err != nil || next.Version <= snapshot.Version {
		t.Fatalf("version: %d -> %d, %v", snapshot.Version, next.Version, err)
	}

	sub, err := service.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if err = service.Generate(ctx, 1, time.Now(), 30); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		select {
		case event := <-sub.Events:
			if event.Version != sub.Snapshot.Version+int64(i+1) {
				t.Fatalf("event version: %d", event.Version)
			}
		case <-time.After(time.Second):
			t.Fatal("missing persisted update")
		}
	}
	store.Close()
	r = request("/api/v1/line")
	if r.Code != 503 {
		t.Fatalf("unavailable: %d", r.Code)
	}
	var failure map[string]string
	_ = json.Unmarshal(r.Body.Bytes(), &failure)
	if failure["code"] != "unavailable" {
		t.Fatalf("error: %s", r.Body.String())
	}
	if r = request("/readyz"); r.Code != 503 {
		t.Fatalf("ready: %d", r.Code)
	}
}
