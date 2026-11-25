package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func confirmFixture(now time.Time) (*Service, *memoryStore, Offer) {
	store := &memoryStore{}
	svc := New(store)
	for i := 0; i < 2; i++ {
		_ = svc.Generate(context.Background(), i, now, int64(i+1))
	}
	offer, _ := svc.IssueOffer(context.Background(), "player-1", "match-1", "HOME", now)
	store.mu.Lock()
	offer.OddsHundredths = 210
	store.state.Offers[offer.ID] = offer
	store.state.Published[0].Odds[0] = 210
	store.mu.Unlock()
	return svc, store, offer
}
func TestOfferBoundaries(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name     string
		odds     int64
		seconds  time.Duration
		closed   bool
		player   string
		expected int64
		unknown  bool
		reason   string
	}{
		{"upper boundary", 310, 0, false, "player-1", 210, false, ""}, {"lower boundary", 110, 0, false, "player-1", 210, false, ""}, {"above", 311, 0, false, "player-1", 210, false, "ODDS_CHANGED"}, {"below", 109, 0, false, "player-1", 210, false, "ODDS_CHANGED"}, {"expired", 210, time.Minute, false, "player-1", 210, false, "OFFER_EXPIRED"}, {"closed", 210, 0, true, "player-1", 210, false, "MARKET_CLOSED"}, {"player", 210, 0, false, "other", 210, false, "OFFER_MISMATCH"}, {"odds", 210, 0, false, "player-1", 211, false, "OFFER_MISMATCH"}, {"unknown", 210, 0, false, "player-1", 210, true, "OFFER_UNAVAILABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, offer := confirmFixture(now)
			store.mu.Lock()
			store.state.Published[0].Odds[0] = tc.odds
			if tc.closed {
				store.state.Published[0].Open = false
			}
			store.mu.Unlock()
			id := offer.ID
			if tc.unknown {
				id = "missing"
			}
			got, err := svc.ConfirmOffer(context.Background(), ConfirmRequest{"bet-1", tc.player, id, tc.expected}, now.Add(tc.seconds))
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != tc.reason {
				t.Fatalf("reason=%q want %q", got.Reason, tc.reason)
			}
			if tc.reason == "" && (got.MatchID != "match-1" || got.OddsHundredths != 210 || got.Outcome != "HOME") {
				t.Fatalf("confirmed=%+v", got)
			}
		})
	}
}
func TestConfirmOnce(t *testing.T) {
	now := time.Now().UTC()
	svc, store, offer := confirmFixture(now)
	req := ConfirmRequest{"bet-1", "player-1", offer.ID, 210}
	first, err := svc.ConfirmOffer(context.Background(), req, now)
	if err != nil || first.Reason != "" {
		t.Fatalf("confirm: %+v %v", first, err)
	}
	restarted := New(store)
	again, err := restarted.ConfirmOffer(context.Background(), req, now.Add(time.Hour))
	if err != nil || again != first {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	changed := req
	changed.ExpectedOddsHundredths++
	_, err = restarted.ConfirmOffer(context.Background(), changed, now)
	if !errors.Is(err, ErrBetConflict) {
		t.Fatalf("conflict: %v", err)
	}
	again, err = restarted.ConfirmOffer(context.Background(), req, now)
	if err != nil || again != first {
		t.Fatal("result overwritten")
	}
	req.BetID = "bet-2"
	result, err := restarted.ConfirmOffer(context.Background(), req, now)
	if err != nil || result.Reason != "OFFER_UNAVAILABLE" {
		t.Fatalf("reuse: %+v %v", result, err)
	}
}
func TestConfirmConcurrent(t *testing.T) {
	now := time.Now().UTC()
	svc, _, offer := confirmFixture(now)
	var wg sync.WaitGroup
	results := make(chan ConfirmResult, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := svc.ConfirmOffer(context.Background(), ConfirmRequest{BetID: string(rune('a' + i)), PlayerID: "player-1", OfferID: offer.ID, ExpectedOddsHundredths: 210}, now)
			if err != nil {
				t.Error(err)
			}
			results <- got
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result.Reason == "" {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("confirmed %d times", successes)
	}
}

func TestIssueRacesConfirmation(t *testing.T) {
	now := time.Now().UTC()
	svc, _, offer := confirmFixture(now)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var issued Offer
	var confirmed ConfirmResult
	var issueErr, confirmErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		issued, issueErr = svc.IssueOffer(context.Background(), "player-1", "match-1", "HOME", now)
	}()
	go func() {
		defer wg.Done()
		<-start
		confirmed, confirmErr = svc.ConfirmOffer(context.Background(), ConfirmRequest{"bet-1", "player-1", offer.ID, 210}, now)
	}()
	close(start)
	wg.Wait()
	if issueErr != nil || confirmErr != nil || confirmed.Reason != "" {
		t.Fatalf("issue=%+v/%v confirmation=%+v/%v", issued, issueErr, confirmed, confirmErr)
	}
	switch issued.ID {
	case offer.ID:
		result, err := svc.ConfirmOffer(context.Background(), ConfirmRequest{"bet-2", "player-1", issued.ID, 210}, now)
		if err != nil || result.Reason != "OFFER_UNAVAILABLE" {
			t.Fatalf("reused original: %+v %v", result, err)
		}
	case "":
		t.Fatal("missing new offer")
	}
}
