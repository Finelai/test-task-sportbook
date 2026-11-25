package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Job struct {
	ID, Player, Offer, Status, Reason, Stage, Token, Match, Outcome string
	Amount, Odds, ConfirmedOdds                                     int64
	Attempts                                                        int
}
type Store interface {
	Claim(context.Context, time.Duration) (*Job, error)
	Save(context.Context, *Job) (bool, error)
	Retry(context.Context, *Job, time.Duration) (bool, error)
}
type Config struct{ Lease, Deadline, RetryMin, RetryMax time.Duration }
type Worker struct {
	store  Store
	line   linev1.LineServiceClient
	wallet walletv1.WalletServiceClient
	cfg    Config
}

func NewWorker(s Store, l linev1.LineServiceClient, w walletv1.WalletServiceClient, c Config) *Worker {
	if c.Lease <= 0 {
		c.Lease = 15 * time.Second
	}
	if c.Deadline <= 0 {
		c.Deadline = 2 * time.Second
	}
	if c.RetryMin <= 0 {
		c.RetryMin = time.Second
	}
	if c.RetryMax <= 0 {
		c.RetryMax = 30 * time.Second
	}
	return &Worker{s, l, w, c}
}
func (w *Worker) ProcessOne(ctx context.Context) error {
	job, err := w.store.Claim(ctx, w.cfg.Lease)
	if err != nil || job == nil {
		return err
	}
	call, cancel := context.WithTimeout(ctx, w.cfg.Deadline)
	defer cancel()
	switch job.Stage {
	case "confirm_offer":
		response, e := w.line.ConfirmOffer(call, &linev1.ConfirmOfferRequest{BetId: job.ID, PlayerId: job.Player, OfferId: job.Offer, ExpectedOddsHundredths: job.Odds})
		if e != nil {
			if code := status.Code(e); code == codes.InvalidArgument || code == codes.AlreadyExists || code == codes.FailedPrecondition || code == codes.NotFound {
				return errors.Join(fmt.Errorf("bet %s stage %s: %w", job.ID, job.Stage, e), w.retry(ctx, job))
			}
			return w.retry(ctx, job)
		}
		if c := response.GetConfirmed(); c != nil {
			if c.MatchId == "" || c.OddsHundredths != job.Odds || c.Outcome == linev1.Outcome_OUTCOME_UNSPECIFIED {
				return w.retry(ctx, job)
			}
			job.Match = c.MatchId
			job.Outcome = c.Outcome.String()
			job.ConfirmedOdds = c.OddsHundredths
			job.Stage = "reserve"
		} else if r := response.GetRejected(); r != nil {
			switch r.Reason {
			case linev1.RejectionReason_OFFER_EXPIRED, linev1.RejectionReason_ODDS_CHANGED, linev1.RejectionReason_OFFER_UNAVAILABLE, linev1.RejectionReason_OFFER_MISMATCH, linev1.RejectionReason_MARKET_CLOSED:
				job.Status = "rejected"
				job.Reason = strings.ToLower(r.Reason.String())
			default:
				return w.retry(ctx, job)
			}
		} else {
			return w.retry(ctx, job)
		}
	case "reserve":
		response, e := w.wallet.Reserve(call, &walletv1.ReserveRequest{BetId: job.ID, PlayerId: job.Player, AmountMinor: job.Amount})
		if e != nil {
			if code := status.Code(e); code == codes.InvalidArgument || code == codes.AlreadyExists || code == codes.FailedPrecondition || code == codes.NotFound {
				return errors.Join(fmt.Errorf("bet %s stage %s: %w", job.ID, job.Stage, e), w.retry(ctx, job))
			}
			return w.retry(ctx, job)
		}
		switch response.GetResult() {
		case walletv1.ReserveResult_RESERVED:
			job.Stage = "capture"
		case walletv1.ReserveResult_INSUFFICIENT_FUNDS:
			job.Status = "rejected"
			job.Reason = "insufficient_funds"
		default:
			return errors.Join(fmt.Errorf("bet %s reserve result %s", job.ID, response.GetResult()), w.retry(ctx, job))
		}
	case "capture":
		response, e := w.wallet.Capture(call, &walletv1.ReserveKey{BetId: job.ID, PlayerId: job.Player})
		if e != nil {
			if code := status.Code(e); code == codes.InvalidArgument || code == codes.AlreadyExists || code == codes.FailedPrecondition || code == codes.NotFound {
				return errors.Join(fmt.Errorf("bet %s capture: %w", job.ID, e), w.retry(ctx, job))
			}
			return w.retry(ctx, job)
		}
		if response.GetResult() != walletv1.CaptureResult_CAPTURED {
			return errors.Join(fmt.Errorf("bet %s capture result %s", job.ID, response.GetResult()), w.retry(ctx, job))
		}
		job.Status = "accepted"
	default:
		return fmt.Errorf("unknown stage %q", job.Stage)
	}
	_, err = w.store.Save(ctx, job)
	return err
}
func (w *Worker) retry(ctx context.Context, j *Job) error {
	delay := w.cfg.RetryMin
	for i := 0; i < j.Attempts && delay < w.cfg.RetryMax; i++ {
		if delay > w.cfg.RetryMax/2 {
			delay = w.cfg.RetryMax
			break
		}
		delay *= 2
	}
	if delay > w.cfg.RetryMax {
		delay = w.cfg.RetryMax
	}
	_, err := w.store.Retry(ctx, j, delay)
	return err
}
