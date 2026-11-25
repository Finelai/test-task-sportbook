package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrBetConflict = errors.New("bet id has different confirmation parameters")

type ConfirmRequest struct {
	BetID                  string `json:"betId"`
	PlayerID               string `json:"playerId"`
	OfferID                string `json:"offerId"`
	ExpectedOddsHundredths int64  `json:"expectedOddsHundredths"`
}
type ConfirmResult struct {
	MatchID        string `json:"matchId,omitempty"`
	Outcome        string `json:"outcome,omitempty"`
	OddsHundredths int64  `json:"oddsHundredths,omitempty"`
	Reason         string `json:"reason,omitempty"`
}
type Confirmation struct {
	Request ConfirmRequest `json:"request"`
	Result  ConfirmResult  `json:"result"`
}

func (s *Service) ConfirmOffer(ctx context.Context, req ConfirmRequest, now time.Time) (ConfirmResult, error) {
	return s.confirmOffer(ctx, req, func() time.Time { return now })
}
func (s *Service) ConfirmOfferNow(ctx context.Context, req ConfirmRequest) (ConfirmResult, error) {
	return s.confirmOffer(ctx, req, time.Now)
}
func (s *Service) confirmOffer(ctx context.Context, req ConfirmRequest, clock func() time.Time) (ConfirmResult, error) {
	if req.BetID == "" || req.PlayerID == "" || req.OfferID == "" || req.ExpectedOddsHundredths <= 0 {
		return ConfirmResult{}, ErrInvalidRequest
	}
	var result ConfirmResult
	err := s.store.Change(ctx, func(st *State) error {
		if previous, ok := st.Confirmations[req.BetID]; ok {
			if previous.Request != req {
				return ErrBetConflict
			}
			result = previous.Result
			return nil
		}
		now := clock()
		offer, exists := st.Offers[req.OfferID]
		switch {
		case !exists || st.UsedOffers[req.OfferID] != "" || st.ActiveOffers[offerKey(offer.PlayerID, offer.MatchID, offer.Outcome)] != req.OfferID:
			result.Reason = "OFFER_UNAVAILABLE"
		case offer.PlayerID != req.PlayerID || offer.OddsHundredths != req.ExpectedOddsHundredths:
			result.Reason = "OFFER_MISMATCH"
		case !now.Before(offer.ExpiresAt):
			result.Reason = "OFFER_EXPIRED"
		default:
			index := -1
			for i := range teams {
				if offer.MatchID == fmt.Sprintf("match-%d", i+1) {
					index = i
					break
				}
			}
			outcome := -1
			for i, v := range []string{"HOME", "DRAW", "AWAY"} {
				if offer.Outcome == v {
					outcome = i
					break
				}
			}
			if index < 0 || outcome < 0 {
				return errors.New("stored offer references unknown market")
			}
			if !st.Markets[index].Open(now) || !st.Published[index].Open {
				result.Reason = "MARKET_CLOSED"
			} else {
				diff := offer.OddsHundredths - st.Published[index].Odds[outcome]
				if diff < 0 {
					diff = -diff
				}
				if diff > 100 {
					result.Reason = "ODDS_CHANGED"
				} else {
					result = ConfirmResult{MatchID: offer.MatchID, Outcome: offer.Outcome, OddsHundredths: offer.OddsHundredths}
					if st.UsedOffers == nil {
						st.UsedOffers = make(map[string]string)
					}
					st.UsedOffers[offer.ID] = req.BetID
				}
			}
		}
		if st.Confirmations == nil {
			st.Confirmations = make(map[string]Confirmation)
		}
		st.Confirmations[req.BetID] = Confirmation{Request: req, Result: result}
		return nil
	})
	return result, err
}
