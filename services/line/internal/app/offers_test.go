package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOfferIssuedOnce(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	svc := New(store)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 2; i++ {
		if err := svc.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	issue := func(at time.Time) Offer {
		t.Helper()
		offer, err := svc.IssueOffer(ctx, "player-1", "match-1", "HOME", at)
		if err != nil {
			t.Fatal(err)
		}
		return offer
	}
	first := issue(now)
	if !first.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatal(first.ExpiresAt)
	}
	var wg sync.WaitGroup
	results := make(chan Offer, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			offer, err := svc.IssueOffer(ctx, "player-1", "match-1", "HOME", now)
			results <- offer
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for got := range results {
		if got.ID != first.ID || !got.ExpiresAt.Equal(first.ExpiresAt) {
			t.Fatalf("duplicate: %+v", got)
		}
	}
	store.mu.Lock()
	store.state.Markets[0].Updated[0] = now.Add(time.Minute)
	store.state.Markets[0].Updated[1] = now.Add(time.Minute)
	store.mu.Unlock()
	expired := issue(now.Add(time.Minute))
	if expired.ID == first.ID {
		t.Fatal("expired offer reused")
	}
	store.mu.Lock()
	state := &store.state
	state.Published[0].Odds[0] = expired.OddsHundredths + 100
	store.mu.Unlock()
	within := issue(now.Add(time.Minute))
	if within.ID != expired.ID {
		t.Fatal("boundary should retain offer")
	}
	store.mu.Lock()
	store.state.Published[0].Odds[0] = expired.OddsHundredths + 101
	store.mu.Unlock()
	changed := issue(now.Add(time.Minute))
	if changed.ID == expired.ID {
		t.Fatal("price change did not replace offer")
	}
	_, err := svc.IssueOffer(ctx, "player-1", "unknown", "HOME", now)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown match: %v", err)
	}
	_, err = svc.IssueOffer(ctx, "player-1", "match-1", "BAD", now)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown outcome: %v", err)
	}
	_, err = svc.IssueOffer(ctx, "player-1", "match-1", "HOME", now.Add(2*time.Minute))
	if !errors.Is(err, ErrMarketClosed) {
		t.Fatalf("stale market: %v", err)
	}
}
