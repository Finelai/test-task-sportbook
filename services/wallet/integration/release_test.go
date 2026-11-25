//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/postgres"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/rpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestReleaseIdempotent(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	if _, e := f.store.Credit(ctx, "player-1", "credit", 1000); e != nil {
		t.Fatal(e)
	}
	service := rpc.New(f.store)
	request := &walletv1.ReserveRequest{BetId: "bet", PlayerId: "player-1", AmountMinor: 300}
	if r, e := service.Reserve(ctx, request); e != nil || r.GetResult() != walletv1.ReserveResult_RESERVED {
		t.Fatalf("reserve: %v %v", r, e)
	}
	key := &walletv1.ReserveKey{BetId: "bet", PlayerId: "player-1"}
	for i := 0; i < 2; i++ {
		if r, e := service.Release(ctx, key); e != nil || r.GetResult() != walletv1.ReleaseResult_RELEASED {
			t.Fatalf("release %v %v", r, e)
		}
	}
	if r, e := service.Release(ctx, &walletv1.ReserveKey{BetId: "bet", PlayerId: "other"}); e != nil || r.GetResult() != walletv1.ReleaseResult_RELEASE_PARAMETER_CONFLICT {
		t.Fatalf("conflict %v %v", r, e)
	}
	if _, e := service.Release(ctx, &walletv1.ReserveKey{BetId: "missing", PlayerId: "player-1"}); status.Code(e) != codes.NotFound {
		t.Fatalf("missing %v", e)
	}
	for _, invalid := range []*walletv1.ReserveKey{nil, {BetId: " ", PlayerId: "player-1"}, {BetId: "bet", PlayerId: ""}} {
		if _, e := service.Release(ctx, invalid); status.Code(e) != codes.InvalidArgument {
			t.Fatalf("invalid %v", e)
		}
	}
	f.store.Close()
	store, e := postgres.Open(ctx, f.url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if r, e := rpc.New(store).Release(ctx, key); e != nil || r.GetResult() != walletv1.ReleaseResult_RELEASED {
		t.Fatalf("restart %v %v", r, e)
	}
	if r, e := rpc.New(store).Capture(ctx, key); e != nil || r.GetResult() != walletv1.CaptureResult_ALREADY_RELEASED {
		t.Fatalf("capture %v %v", r, e)
	}
	var available, reserved int64
	var n int
	e = f.conn.QueryRow(ctx, `SELECT available,reserved,(SELECT count(*) FROM wallet_journal WHERE operation='release') FROM wallets WHERE player_id='player-1'`).Scan(&available, &reserved, &n)
	if e != nil || available != 1000 || reserved != 0 || n != 1 {
		t.Fatalf("balance %d %d entries %d error %v", available, reserved, n, e)
	}
}
func TestReservationFinalState(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	if _, e := f.store.Credit(ctx, "player-1", "credit", 1000); e != nil {
		t.Fatal(e)
	}
	service := rpc.New(f.store)
	if r, e := service.Reserve(ctx, &walletv1.ReserveRequest{BetId: "race", PlayerId: "player-1", AmountMinor: 300}); e != nil || r.GetResult() != walletv1.ReserveResult_RESERVED {
		t.Fatalf("reserve %v %v", r, e)
	}
	key := &walletv1.ReserveKey{BetId: "race", PlayerId: "player-1"}
	var c *walletv1.CaptureResponse
	var r *walletv1.ReleaseResponse
	var ce, re error
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; c, ce = service.Capture(ctx, key) }()
	go func() { defer wg.Done(); <-start; r, re = service.Release(ctx, key) }()
	close(start)
	wg.Wait()
	if ce != nil || re != nil {
		t.Fatalf("errors %v %v", ce, re)
	}
	var expected int64
	switch {
	case c.GetResult() == walletv1.CaptureResult_CAPTURED && r.GetResult() == walletv1.ReleaseResult_ALREADY_CAPTURED:
		expected = 700
	case c.GetResult() == walletv1.CaptureResult_ALREADY_RELEASED && r.GetResult() == walletv1.ReleaseResult_RELEASED:
		expected = 1000
	default:
		t.Fatalf("results %v %v", c, r)
	}
	c2, ce := service.Capture(ctx, key)
	r2, re := service.Release(ctx, key)
	if ce != nil || re != nil || c2.GetResult() != c.GetResult() || r2.GetResult() != r.GetResult() {
		t.Fatalf("repeat %v %v %v %v", c2, r2, ce, re)
	}
	var available, reserved int64
	var n int
	e := f.conn.QueryRow(ctx, `SELECT available,reserved,(SELECT count(*) FROM wallet_journal WHERE operation IN ('capture','release')) FROM wallets WHERE player_id='player-1'`).Scan(&available, &reserved, &n)
	if e != nil || available != expected || reserved != 0 || n != 1 {
		t.Fatalf("balance %d %d entries %d error %v", available, reserved, n, e)
	}
}
