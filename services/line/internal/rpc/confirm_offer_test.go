package rpc

import (
	"context"
	"sync"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memoryStore struct {
	mu    sync.Mutex
	state app.State
}

func (s *memoryStore) Change(_ context.Context, f func(*app.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return f(&s.state)
}
func (s *memoryStore) Read(_ context.Context) (app.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}
func TestConfirmOfferRPC(t *testing.T) {
	ctx := context.Background()
	svc := app.New(&memoryStore{})
	for i := 0; i < 2; i++ {
		if err := svc.Generate(ctx, i, time.Now(), int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	offer, err := svc.IssueOfferNow(ctx, "player-1", "match-1", "HOME")
	if err != nil {
		t.Fatal(err)
	}
	server := New(svc)
	req := &linev1.ConfirmOfferRequest{BetId: "bet-1", PlayerId: "player-1", OfferId: offer.ID, ExpectedOddsHundredths: offer.OddsHundredths}
	result, err := server.ConfirmOffer(ctx, req)
	if err != nil || result.GetConfirmed() == nil || result.GetConfirmed().GetOutcome() != linev1.Outcome_HOME || result.GetConfirmed().GetOddsHundredths() != offer.OddsHundredths {
		t.Fatalf("confirm: %v %v", result, err)
	}
	req.BetId = "bet-2"
	result, err = server.ConfirmOffer(ctx, req)
	if err != nil || result.GetRejected().GetReason() != linev1.RejectionReason_OFFER_UNAVAILABLE {
		t.Fatalf("reuse: %v %v", result, err)
	}
	req.BetId = "bet-1"
	req.ExpectedOddsHundredths++
	_, err = server.ConfirmOffer(ctx, req)
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("conflict: %v", err)
	}
}
