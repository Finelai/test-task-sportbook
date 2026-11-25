package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/domain"
)

type Quote struct {
	Outcome string `json:"outcome"`
	Odds    string `json:"odds"`
}
type Match struct {
	MatchID  string  `json:"matchId"`
	HomeTeam string  `json:"homeTeam"`
	AwayTeam string  `json:"awayTeam"`
	Open     bool    `json:"open"`
	Quotes   []Quote `json:"quotes"`
}
type Snapshot struct {
	Version int64   `json:"version"`
	Matches []Match `json:"matches"`
}
type State struct {
	Version       int64                   `json:"version"`
	Markets       [8]domain.Market        `json:"markets"`
	Published     [8]Published            `json:"published"`
	Initialized   bool                    `json:"initialized"`
	Offers        map[string]Offer        `json:"offers,omitempty"`
	ActiveOffers  map[string]string       `json:"activeOffers,omitempty"`
	Confirmations map[string]Confirmation `json:"confirmations,omitempty"`
	UsedOffers    map[string]string       `json:"usedOffers,omitempty"`
}
type Published struct {
	Open bool           `json:"open"`
	Odds domain.Odds1X2 `json:"odds"`
}
type Store interface {
	Change(context.Context, func(*State) error) error
	Read(context.Context) (State, error)
}
type Service struct {
	store       Store
	mu          sync.Mutex
	subscribers map[*Subscription]struct{}
}

func New(store Store) *Service {
	return &Service{store: store, subscribers: make(map[*Subscription]struct{})}
}

var teams = [8][2]string{{"Arsenal", "Chelsea"}, {"Liverpool", "Everton"}, {"Barcelona", "Valencia"}, {"Real Madrid", "Sevilla"}, {"Bayern", "Dortmund"}, {"Milan", "Inter"}, {"Ajax", "PSV"}, {"Porto", "Benfica"}}

func (s *Service) Generate(ctx context.Context, source int, now time.Time, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []MatchUpdated
	err := s.store.Change(ctx, func(st *State) error {
		for i := range st.Markets {
			odds := [3]float64{2 + rng.Float64()*2, 2.5 + rng.Float64()*2, 2 + rng.Float64()*2}
			if err := st.Markets[i].Update(source, odds, now); err != nil {
				return err
			}
		}
		events = publish(st, now)
		return nil
	})
	if err == nil {
		s.broadcast(events)
	}
	return err
}
func publish(st *State, now time.Time) []MatchUpdated {
	next := st.Published
	for i, market := range st.Markets {
		odds, err := domain.Price(market.Sources)
		if err != nil {
			return nil
		}
		next[i] = Published{Open: market.Open(now), Odds: odds}
	}
	var events []MatchUpdated
	for i, market := range next {
		if !st.Initialized || market != st.Published[i] {
			st.Version++
			events = append(events, MatchUpdated{Version: st.Version, Match: match(i, market)})
		}
	}
	st.Published = next
	st.Initialized = true
	return events
}
func match(i int, market Published) Match {
	quotes := make([]Quote, 3)
	for j, outcome := range []string{"HOME", "DRAW", "AWAY"} {
		odds := market.Odds[j]
		quotes[j] = Quote{outcome, fmt.Sprintf("%d.%02d", odds/100, odds%100)}
	}
	return Match{fmt.Sprintf("match-%d", i+1), teams[i][0], teams[i][1], market.Open, quotes}
}
func snapshot(st State) (Snapshot, error) {
	if !st.Initialized {
		return Snapshot{}, errors.New("sources unavailable")
	}
	matches := make([]Match, 0, 8)
	for i, market := range st.Published {
		matches = append(matches, match(i, market))
	}
	return Snapshot{st.Version, matches}, nil
}

// snapshotLocked serializes the persisted snapshot with publication and subscription.
func (s *Service) snapshotLocked(ctx context.Context) (Snapshot, error) {
	var state State
	var events []MatchUpdated
	err := s.store.Change(ctx, func(st *State) error { events = publish(st, time.Now()); state = *st; return nil })
	if err != nil {
		return Snapshot{}, err
	}
	s.broadcast(events)
	return snapshot(state)
}
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(ctx)
}

// Refresh publishes freshness changes even when no source produces updates.
func (s *Service) Refresh(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []MatchUpdated
	err := s.store.Change(ctx, func(st *State) error { events = publish(st, now); return nil })
	if err == nil {
		s.broadcast(events)
	}
	return err
}
