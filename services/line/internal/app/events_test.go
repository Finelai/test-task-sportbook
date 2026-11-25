package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	state   State
	entered chan struct{}
	proceed chan struct{}
}

func (m *memoryStore) Change(_ context.Context, fn func(*State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entered != nil {
		close(m.entered)
		<-m.proceed
		m.entered = nil
	}
	return fn(&m.state)
}
func (m *memoryStore) Read(_ context.Context) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}
func TestSubscribeWithoutGap(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	service := New(store)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := service.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store.entered = make(chan struct{})
	store.proceed = make(chan struct{})
	subscribed := make(chan *Subscription, 1)
	go func() {
		sub, err := service.Subscribe(ctx)
		if err != nil {
			t.Error(err)
			return
		}
		subscribed <- sub
	}()
	<-store.entered
	updated := make(chan error, 1)
	go func() { updated <- service.Generate(ctx, 0, now, 100) }()
	close(store.proceed)
	sub := <-subscribed
	defer sub.Close()
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	if sub.Snapshot.Version != before.Version {
		t.Fatalf("snapshot version %d, want %d", sub.Snapshot.Version, before.Version)
	}
	last := sub.Snapshot.Version
	for i := 0; i < 8; i++ {
		select {
		case event := <-sub.Events:
			if event.Version != last+1 {
				t.Fatalf("event version %d after %d", event.Version, last)
			}
			last = event.Version
		case <-time.After(time.Second):
			t.Fatal("lost update")
		}
	}
	if err := service.Generate(ctx, 1, now, 101); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		event := <-sub.Events
		if event.Version != last+1 {
			t.Fatalf("event version %d after %d", event.Version, last)
		}
		last = event.Version
	}
	// A slow subscriber must be disconnected when its bounded queue fills.
	for i := 0; i < 100; i++ {
		if err := service.Generate(ctx, 0, now, int64(1000+i)); err != nil {
			t.Fatal(err)
		}
		select {
		case <-sub.Done:
			goto overflow
		default:
		}
	}
	t.Fatal("slow subscriber stayed connected")
overflow:
	replacement, err := service.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	current, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Snapshot.Version != current.Version || len(replacement.Snapshot.Matches) != 8 {
		t.Fatalf("reconnect snapshot = %+v", replacement.Snapshot)
	}
}

func TestRefreshPublishesClosedMarkets(t *testing.T) {
	ctx := context.Background()
	service := New(&memoryStore{})
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := service.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	sub, err := service.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if err = service.Refresh(ctx, now.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		select {
		case event := <-sub.Events:
			if event.Match.Open || event.Version != sub.Snapshot.Version+int64(i+1) {
				t.Fatalf("unexpected closed event: %+v", event)
			}
		case <-time.After(time.Second):
			t.Fatal("missing closed market event")
		}
	}
}
