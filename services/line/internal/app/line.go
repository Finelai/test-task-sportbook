package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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
	Version     int64            `json:"version"`
	Markets     [8]domain.Market `json:"markets"`
	Published   [8]Published     `json:"published"`
	Initialized bool             `json:"initialized"`
}
type Published struct {
	Open bool           `json:"open"`
	Odds domain.Odds1X2 `json:"odds"`
}
type Store interface {
	Change(context.Context, func(*State) error) error
	Read(context.Context) (State, error)
}
type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

var teams = [8][2]string{{"Arsenal", "Chelsea"}, {"Liverpool", "Everton"}, {"Barcelona", "Valencia"}, {"Real Madrid", "Sevilla"}, {"Bayern", "Dortmund"}, {"Milan", "Inter"}, {"Ajax", "PSV"}, {"Porto", "Benfica"}}

func (s *Service) Generate(ctx context.Context, source int, now time.Time, seed int64) error {
	rng := rand.New(rand.NewSource(seed))
	return s.store.Change(ctx, func(st *State) error {
		for i := range st.Markets {
			odds := [3]float64{2 + rng.Float64()*2, 2.5 + rng.Float64()*2, 2 + rng.Float64()*2}
			if err := st.Markets[i].Update(source, odds, now); err != nil {
				return err
			}
		}
		publish(st, now)
		return nil
	})
}
func publish(st *State, now time.Time) {
	next := st.Published
	for i, market := range st.Markets {
		odds, err := domain.Price(market.Sources)
		if err != nil {
			return
		}
		next[i] = Published{Open: market.Open(now), Odds: odds}
	}
	if !st.Initialized || next != st.Published {
		st.Version++
	}
	st.Published = next
	st.Initialized = true
}
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := s.store.Change(ctx, func(st *State) error { publish(st, time.Now()); return nil }); err != nil {
		return Snapshot{}, err
	}
	state, err := s.store.Read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !state.Initialized {
		return Snapshot{}, errors.New("sources unavailable")
	}
	matches := make([]Match, 0, 8)
	for i, market := range state.Published {
		quotes := make([]Quote, 3)
		for j, outcome := range []string{"HOME", "DRAW", "AWAY"} {
			odds := market.Odds[j]
			quotes[j] = Quote{outcome, fmt.Sprintf("%d.%02d", odds/100, odds%100)}
		}
		matches = append(matches, Match{fmt.Sprintf("match-%d", i+1), teams[i][0], teams[i][1], market.Open, quotes})
	}
	return Snapshot{state.Version, matches}, nil
}
