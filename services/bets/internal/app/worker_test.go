package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"google.golang.org/grpc"
)

type memoryStore struct {
	job   Job
	saves int
	retry bool
}

func (s *memoryStore) Claim(context.Context, time.Duration) (*Job, error) {
	if s.job.Status != "pending" {
		return nil, nil
	}
	j := s.job
	return &j, nil
}
func (s *memoryStore) Save(_ context.Context, j *Job) (bool, error) {
	s.job = *j
	s.saves++
	return true, nil
}
func (s *memoryStore) Retry(context.Context, *Job, time.Duration) (bool, error) {
	s.retry = true
	return true, nil
}

type lineClient struct {
	response *linev1.ConfirmOfferResponse
	err      error
}

func (c *lineClient) ConfirmOffer(context.Context, *linev1.ConfirmOfferRequest, ...grpc.CallOption) (*linev1.ConfirmOfferResponse, error) {
	return c.response, c.err
}

type walletClient struct {
	reserve walletv1.ReserveResult
	capture walletv1.CaptureResult
	calls   []string
}

func (c *walletClient) Reserve(context.Context, *walletv1.ReserveRequest, ...grpc.CallOption) (*walletv1.ReserveResponse, error) {
	c.calls = append(c.calls, "reserve")
	return &walletv1.ReserveResponse{Result: c.reserve}, nil
}
func (c *walletClient) Capture(context.Context, *walletv1.ReserveKey, ...grpc.CallOption) (*walletv1.CaptureResponse, error) {
	c.calls = append(c.calls, "capture")
	return &walletv1.CaptureResponse{Result: c.capture}, nil
}
func (c *walletClient) Release(context.Context, *walletv1.ReserveKey, ...grpc.CallOption) (*walletv1.ReleaseResponse, error) {
	panic("unexpected release")
}
func TestWorkerConfirmReserveCapture(t *testing.T) {
	s := &memoryStore{job: Job{ID: "bet", Player: "player-1", Offer: "offer", Amount: 300, Odds: 210, Status: "pending", Stage: "confirm_offer"}}
	l := &lineClient{response: &linev1.ConfirmOfferResponse{Result: &linev1.ConfirmOfferResponse_Confirmed{Confirmed: &linev1.ConfirmedOffer{MatchId: "match", Outcome: linev1.Outcome_HOME, OddsHundredths: 210}}}}
	w := &walletClient{reserve: walletv1.ReserveResult_RESERVED, capture: walletv1.CaptureResult_CAPTURED}
	worker := NewWorker(s, l, w, Config{})
	for i := 0; i < 3; i++ {
		if err := worker.ProcessOne(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if s.job.Status != "accepted" || s.job.Match != "match" || s.job.Outcome != "HOME" || s.job.ConfirmedOdds != 210 || len(w.calls) != 2 || w.calls[0] != "reserve" || w.calls[1] != "capture" {
		t.Fatalf("job=%+v calls=%v", s.job, w.calls)
	}
}
func TestWorkerRejectAndRetry(t *testing.T) {
	for reason, expected := range map[linev1.RejectionReason]string{linev1.RejectionReason_OFFER_EXPIRED: "offer_expired", linev1.RejectionReason_ODDS_CHANGED: "odds_changed", linev1.RejectionReason_OFFER_UNAVAILABLE: "offer_unavailable", linev1.RejectionReason_OFFER_MISMATCH: "offer_mismatch", linev1.RejectionReason_MARKET_CLOSED: "market_closed"} {
		t.Run(reason.String(), func(t *testing.T) {
			s := &memoryStore{job: Job{Status: "pending", Stage: "confirm_offer"}}
			l := &lineClient{response: &linev1.ConfirmOfferResponse{Result: &linev1.ConfirmOfferResponse_Rejected{Rejected: &linev1.RejectedOffer{Reason: reason}}}}
			w := &walletClient{}
			if err := NewWorker(s, l, w, Config{}).ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if s.job.Status != "rejected" || s.job.Reason != expected || len(w.calls) != 0 {
				t.Fatalf("job=%+v calls=%v", s.job, w.calls)
			}
		})
	}
	s := &memoryStore{job: Job{Status: "pending", Stage: "confirm_offer"}}
	w := &walletClient{}
	if err := NewWorker(s, &lineClient{err: errors.New("lost response")}, w, Config{}).ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.retry || s.job.Status != "pending" || len(w.calls) != 0 {
		t.Fatalf("job=%+v retry=%v", s.job, s.retry)
	}
}

func TestInsufficientFundsDoesNotCapture(t *testing.T) {
	s := &memoryStore{job: Job{ID: "bet", Player: "player-1", Amount: 300, Status: "pending", Stage: "reserve"}}
	w := &walletClient{reserve: walletv1.ReserveResult_INSUFFICIENT_FUNDS}
	if err := NewWorker(s, &lineClient{}, w, Config{}).ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.job.Status != "rejected" || s.job.Reason != "insufficient_funds" || len(w.calls) != 1 || w.calls[0] != "reserve" {
		t.Fatalf("job=%+v calls=%v", s.job, w.calls)
	}
}
func TestCaptureConflictIsNotAccepted(t *testing.T) {
	s := &memoryStore{job: Job{ID: "bet", Player: "player-1", Status: "pending", Stage: "capture"}}
	w := &walletClient{capture: walletv1.CaptureResult_ALREADY_RELEASED}
	if err := NewWorker(s, &lineClient{}, w, Config{}).ProcessOne(context.Background()); err == nil || !strings.Contains(err.Error(), "ALREADY_RELEASED") {
		t.Fatalf("expected visible capture conflict, got %v", err)
	}
	if s.job.Status != "pending" || !s.retry {
		t.Fatalf("job=%+v retry=%v", s.job, s.retry)
	}
}
