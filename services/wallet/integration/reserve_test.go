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

func TestConcurrentReserve(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	if _, err := f.store.Credit(ctx, "player-1", "initial", 1000); err != nil {
		t.Fatal(err)
	}
	service := rpc.New(f.store)
	reqs := []*walletv1.ReserveRequest{{BetId: "bet-1", PlayerId: "player-1", AmountMinor: 700}, {BetId: "bet-2", PlayerId: "player-1", AmountMinor: 700}}
	responses := make([]*walletv1.ReserveResponse, 2)
	failures := make([]error, 2)
	var wg sync.WaitGroup
	for i := range reqs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i], failures[i] = service.Reserve(ctx, reqs[i]) }(i)
	}
	wg.Wait()
	winner, loser := -1, -1
	for i, r := range responses {
		if failures[i] != nil {
			t.Fatal(failures[i])
		}
		switch r.Result {
		case walletv1.ReserveResult_RESERVED:
			winner = i
		case walletv1.ReserveResult_INSUFFICIENT_FUNDS:
			loser = i
		default:
			t.Fatalf("unexpected %v", r)
		}
	}
	if winner < 0 || loser < 0 {
		t.Fatalf("results %v", responses)
	}
	var a, b int64
	var entries int
	if e := f.conn.QueryRow(ctx, `SELECT available,reserved,(SELECT count(*) FROM wallet_journal WHERE operation='reserve') FROM wallets WHERE player_id='player-1'`).Scan(&a, &b, &entries); e != nil || a != 300 || b != 700 || entries != 1 {
		t.Fatalf("balance %d %d entries %d: %v", a, b, entries, e)
	}
	if _, e := f.store.Credit(ctx, "player-1", "later", 1000); e != nil {
		t.Fatal(e)
	}
	r, e := service.Reserve(ctx, reqs[loser])
	if e != nil || r.Result != walletv1.ReserveResult_INSUFFICIENT_FUNDS {
		t.Fatalf("retry failure %v %v", r, e)
	}
	for _, request := range reqs {
		r, e = service.Reserve(ctx, &walletv1.ReserveRequest{BetId: request.BetId, PlayerId: request.PlayerId, AmountMinor: 701})
		if e != nil || r.Result != walletv1.ReserveResult_PARAMETER_CONFLICT {
			t.Fatalf("conflict %v %v", r, e)
		}
	}
	r, e = service.Reserve(ctx, &walletv1.ReserveRequest{BetId: reqs[winner].BetId, PlayerId: "player-2", AmountMinor: 700})
	if e != nil || r.Result != walletv1.ReserveResult_PARAMETER_CONFLICT {
		t.Fatalf("player conflict %v %v", r, e)
	}
	f.store.Close()
	store, e := postgres.Open(ctx, f.url)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	r, e = rpc.New(store).Reserve(ctx, reqs[winner])
	if e != nil || r.Result != walletv1.ReserveResult_RESERVED {
		t.Fatalf("restart %v %v", r, e)
	}
}
func TestReserveInvalid(t *testing.T) {
	f := newCreditFixture(t)
	service := rpc.New(f.store)
	ctx := context.Background()
	for _, req := range []*walletv1.ReserveRequest{{BetId: "", PlayerId: "player-1", AmountMinor: 1}, {BetId: "bet", PlayerId: " ", AmountMinor: 1}, {BetId: "bet", PlayerId: "player-1", AmountMinor: 0}, {BetId: "bet", PlayerId: "player-1", AmountMinor: -1}} {
		_, err := service.Reserve(ctx, req)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%v: %v", req, err)
		}
	}
	var wallets, entries int
	if e := f.conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM wallets),(SELECT count(*) FROM wallet_journal)`).Scan(&wallets, &entries); e != nil || wallets != 0 || entries != 0 {
		t.Fatalf("wallets %d entries %d error %v", wallets, entries, e)
	}
}

func TestConcurrentSameBetDifferentPlayers(t *testing.T) {
	f := newCreditFixture(t)
	ctx := context.Background()
	for _, player := range []string{"p1", "p2"} {
		if _, err := f.store.Credit(ctx, player, "initial", 1000); err != nil {
			t.Fatal(err)
		}
	}
	service := rpc.New(f.store)
	requests := []*walletv1.ReserveRequest{{BetId: "same", PlayerId: "p1", AmountMinor: 700}, {BetId: "same", PlayerId: "p2", AmountMinor: 700}}
	results := make([]*walletv1.ReserveResponse, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = service.Reserve(ctx, requests[i]) }(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("errors %v", errs)
	}
	if (results[0].Result != walletv1.ReserveResult_RESERVED || results[1].Result != walletv1.ReserveResult_PARAMETER_CONFLICT) && (results[1].Result != walletv1.ReserveResult_RESERVED || results[0].Result != walletv1.ReserveResult_PARAMETER_CONFLICT) {
		t.Fatalf("results %v", results)
	}
	var available, reserved int64
	var count int
	if err := f.conn.QueryRow(ctx, `SELECT sum(available),sum(reserved),(SELECT count(*) FROM wallet_journal WHERE operation='reserve') FROM wallets`).Scan(&available, &reserved, &count); err != nil || available != 1300 || reserved != 700 || count != 1 {
		t.Fatalf("available %d reserved %d journal %d err %v", available, reserved, count, err)
	}
}
