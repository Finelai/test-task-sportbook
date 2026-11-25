package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
)

type store struct {
	mu    sync.Mutex
	state app.State
}

func (s *store) Change(_ context.Context, f func(*app.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return f(&s.state)
}
func (s *store) Read(_ context.Context) (app.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}
func (s *store) Ready(context.Context) error { return nil }
func TestEventsStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := &store{}
	service := app.New(db)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := service.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(New(service, db))
	defer server.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/line/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("response: %d %v", resp.StatusCode, resp.Header)
	}
	reader := bufio.NewReader(resp.Body)
	read := func() (string, string) {
		t.Helper()
		name, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		data, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(name), strings.TrimSpace(strings.TrimPrefix(data, "data: "))
	}
	name, data := read()
	if name != "event: snapshot" {
		t.Fatal(name)
	}
	var snap app.Snapshot
	if err = json.Unmarshal([]byte(data), &snap); err != nil || len(snap.Matches) != 8 {
		t.Fatalf("snapshot %s: %v", data, err)
	}
	if err = service.Generate(ctx, 0, now, 99); err != nil {
		t.Fatal(err)
	}
	name, data = read()
	if name != "event: match_updated" {
		t.Fatal(name)
	}
	var event app.MatchUpdated
	if err = json.Unmarshal([]byte(data), &event); err != nil || event.Version != snap.Version+1 {
		t.Fatalf("event %s: %v", data, err)
	}
}

func TestEventsSurviveIdleInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := &store{}
	service := app.New(db)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := service.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(New(service, db))
	defer server.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/line/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	for i := 0; i < 3; i++ {
		if _, err = reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(6 * time.Second)
	if err = service.Generate(ctx, 0, time.Now(), 99); err != nil {
		t.Fatal(err)
	}
	name, err := reader.ReadString('\n')
	if err != nil || name != "event: match_updated\n" {
		t.Fatalf("event after idle = %q, %v", name, err)
	}
}
