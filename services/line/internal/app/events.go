package app

import (
	"context"
	"sync"
)

type MatchUpdated struct {
	Version int64 `json:"version"`
	Match   Match `json:"match"`
}
type Subscription struct {
	Snapshot Snapshot
	Events   <-chan MatchUpdated
	Done     <-chan struct{}
	events   chan MatchUpdated
	done     chan struct{}
	service  *Service
	once     sync.Once
}

func (s *Service) Subscribe(ctx context.Context) (*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.snapshotLocked(ctx)
	if err != nil {
		return nil, err
	}
	events := make(chan MatchUpdated, 64)
	done := make(chan struct{})
	sub := &Subscription{Snapshot: snap, Events: events, Done: done, events: events, done: done, service: s}
	s.subscribers[sub] = struct{}{}
	return sub, nil
}
func (sub *Subscription) Close() {
	sub.service.mu.Lock()
	defer sub.service.mu.Unlock()
	sub.closeLocked()
}
func (sub *Subscription) closeLocked() {
	sub.once.Do(func() { delete(sub.service.subscribers, sub); close(sub.done) })
}
func (s *Service) broadcast(events []MatchUpdated) {
	for sub := range s.subscribers {
		for _, event := range events {
			select {
			case sub.events <- event:
			default:
				sub.closeLocked()
			}
			if _, ok := s.subscribers[sub]; !ok {
				break
			}
		}
	}
}
