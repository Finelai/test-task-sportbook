package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidRequest = errors.New("invalid request")
var ErrMarketClosed = errors.New("market closed")
var ErrUnavailable = errors.New("line unavailable")

type Offer struct {
	ID             string    `json:"offerId"`
	PlayerID       string    `json:"playerId"`
	MatchID        string    `json:"matchId"`
	Outcome        string    `json:"outcome"`
	OddsHundredths int64     `json:"oddsHundredths"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func (o Offer) Odds() string {
	return fmt.Sprintf("%d.%02d", o.OddsHundredths/100, o.OddsHundredths%100)
}
func (s *Service) IssueOffer(ctx context.Context, player, matchID, outcome string, now time.Time) (Offer, error) {
	return s.issueOffer(ctx, player, matchID, outcome, func() time.Time { return now })
}

// IssueOfferNow reads the clock after acquiring the database lock.
func (s *Service) IssueOfferNow(ctx context.Context, player, matchID, outcome string) (Offer, error) {
	return s.issueOffer(ctx, player, matchID, outcome, time.Now)
}
func (s *Service) issueOffer(ctx context.Context, player, matchID, outcome string, clock func() time.Time) (Offer, error) {
	index := -1
	for i := range teams {
		if matchID == fmt.Sprintf("match-%d", i+1) {
			index = i
			break
		}
	}
	outcomeIndex := -1
	for i, v := range []string{"HOME", "DRAW", "AWAY"} {
		if outcome == v {
			outcomeIndex = i
			break
		}
	}
	if player == "" || index < 0 || outcomeIndex < 0 {
		return Offer{}, ErrInvalidRequest
	}
	// The same state-row lock used for price changes serializes issue and replacement.
	var result Offer
	err := s.store.Change(ctx, func(st *State) error {
		now := clock()
		if !st.Initialized {
			return ErrUnavailable
		}
		if !st.Markets[index].Open(now) || !st.Published[index].Open {
			return ErrMarketClosed
		}
		if st.Offers == nil {
			st.Offers = make(map[string]Offer)
		}
		if st.ActiveOffers == nil {
			st.ActiveOffers = make(map[string]string)
		}
		key := fmt.Sprintf("%d:%s:%s:%s", len(player), player, matchID, outcome)
		odds := st.Published[index].Odds[outcomeIndex]
		if id := st.ActiveOffers[key]; id != "" {
			old := st.Offers[id]
			diff := old.OddsHundredths - odds
			if diff < 0 {
				diff = -diff
			}
			if now.Before(old.ExpiresAt) && diff <= 100 {
				result = old
				return nil
			}
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		result = Offer{ID: id.String(), PlayerID: player, MatchID: matchID, Outcome: outcome, OddsHundredths: odds, ExpiresAt: now.UTC().Add(time.Minute)}
		st.Offers[result.ID] = result
		st.ActiveOffers[key] = result.ID
		return nil
	})
	return result, err
}
