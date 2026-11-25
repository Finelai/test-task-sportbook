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

func TestCaptureIdempotent(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	if _, e := f.store.Credit(ctx, "player-1", "credit", 1000); e != nil {
		t.Fatal(e)
	}
	service := rpc.New(f.store)
	request := &walletv1.ReserveRequest{BetId: "bet", PlayerId: "player-1", AmountMinor: 300}
	if r, e := service.Reserve(ctx, request); e != nil || r.Result != walletv1.ReserveResult_RESERVED {
		t.Fatalf("reserve %v %v", r, e)
	}
	key := &walletv1.ReserveKey{BetId: "bet", PlayerId: "player-1"}
	var wg sync.WaitGroup
	results := make([]*walletv1.CaptureResponse, 10)
	errs := make([]error, 10)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = service.Capture(ctx, key) }(i)
	}
	wg.Wait()
	for i, r := range results {
		if errs[i] != nil || r.GetResult() != walletv1.CaptureResult_CAPTURED {
			t.Fatalf("capture %v %v", r, errs[i])
		}
	}
	check := func() {
		t.Helper()
		var available, reserved int64
		var n int
		e := f.conn.QueryRow(ctx, `SELECT available,reserved,(SELECT count(*) FROM wallet_journal WHERE operation='capture') FROM wallets WHERE player_id='player-1'`).Scan(&available, &reserved, &n)
		if e != nil || available != 700 || reserved != 0 || n != 1 {
			t.Fatalf("balance %d %d journal %d err %v", available, reserved, n, e)
		}
	}
	check()
	if r, e := service.Capture(ctx, &walletv1.ReserveKey{BetId: "bet", PlayerId: "other"}); e != nil || r.GetResult() != walletv1.CaptureResult_CAPTURE_PARAMETER_CONFLICT {
		t.Fatalf("conflict %v %v", r, e)
	}
	if _, e := service.Capture(ctx, &walletv1.ReserveKey{BetId: "missing", PlayerId: "player-1"}); status.Code(e) != codes.NotFound {
		t.Fatalf("missing %v", e)
	}
	if r, e := service.Reserve(ctx, request); e != nil || r.GetResult() != walletv1.ReserveResult_RESERVED {
		t.Fatalf("reserve retry %v %v", r, e)
	}
	f.store.Close()
	store, e := postgres.Open(ctx, f.url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if r, e := rpc.New(store).Capture(ctx, key); e != nil || r.GetResult() != walletv1.CaptureResult_CAPTURED {
		t.Fatalf("restart %v %v", r, e)
	}
	check()
}
func TestCaptureInvalidAndRejected(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	service := rpc.New(f.store)
	for _, key := range []*walletv1.ReserveKey{nil, {BetId: "", PlayerId: "player-1"}, {BetId: "bet", PlayerId: " "}} {
		if _, e := service.Capture(ctx, key); status.Code(e) != codes.InvalidArgument {
			t.Fatalf("invalid %v: %v", key, e)
		}
	}
	if r, e := service.Reserve(ctx, &walletv1.ReserveRequest{BetId: "rejected", PlayerId: "player-1", AmountMinor: 100}); e != nil || r.GetResult() != walletv1.ReserveResult_INSUFFICIENT_FUNDS {
		t.Fatalf("reserve %v %v", r, e)
	}
	if _, e := service.Capture(ctx, &walletv1.ReserveKey{BetId: "rejected", PlayerId: "player-1"}); status.Code(e) != codes.NotFound {
		t.Fatalf("rejected reserve: %v", e)
	}
	var n int
	if e := f.conn.QueryRow(ctx, `SELECT count(*) FROM wallet_journal`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("journal %d %v", n, e)
	}
}
