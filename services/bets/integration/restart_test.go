//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type lossProxy struct {
	linev1.UnimplementedLineServiceServer
	walletv1.UnimplementedWalletServiceServer
	line    linev1.LineServiceClient
	wallet  walletv1.WalletServiceClient
	target  string
	lost    chan struct{}
	dropped atomic.Bool
}

func (p *lossProxy) lose(ctx context.Context, method string, err error) error {
	if err == nil && p.target == method && p.dropped.CompareAndSwap(false, true) {
		close(p.lost)
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}
func (p *lossProxy) ConfirmOffer(ctx context.Context, r *linev1.ConfirmOfferRequest) (*linev1.ConfirmOfferResponse, error) {
	v, e := p.line.ConfirmOffer(ctx, r)
	return v, p.lose(ctx, "confirm", e)
}
func (p *lossProxy) Reserve(ctx context.Context, r *walletv1.ReserveRequest) (*walletv1.ReserveResponse, error) {
	v, e := p.wallet.Reserve(ctx, r)
	return v, p.lose(ctx, "reserve", e)
}
func (p *lossProxy) Capture(ctx context.Context, r *walletv1.ReserveKey) (*walletv1.CaptureResponse, error) {
	v, e := p.wallet.Capture(ctx, r)
	return v, p.lose(ctx, "capture", e)
}
func TestRecoverAfterProcessRestart(t *testing.T) {
	f := system(t)
	ctx := context.Background()
	post(t, f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"30.00"}`, "restart-credit")
	root, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	binary := filepath.Join(t.TempDir(), "bets")
	build := exec.Command("go", "build", "-o", binary, "./services/bets/cmd/server")
	build.Dir = root
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, out)
	}
	dial := func(addr string) *grpc.ClientConn {
		c, e := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	line := linev1.NewLineServiceClient(dial(f.lineGRPC))
	wallet := walletv1.NewWalletServiceClient(dial(f.walletGRPC))
	for i, target := range []string{"confirm", "reserve", "capture"} {
		t.Run(target, func(t *testing.T) {
			offer := post(t, f.lineHTTP+"/api/v1/line/offers", fmt.Sprintf(`{"matchId":"match-%d","outcome":"AWAY"}`, i+1), "")
			var whole, frac int64
			if _, e := fmt.Sscanf(offer["odds"].(string), "%d.%d", &whole, &frac); e != nil {
				t.Fatal(e)
			}
			bet, e := f.store.Submit(ctx, "restart-"+target, offer["offerId"].(string), 300, whole*100+frac)
			if e != nil {
				t.Fatal(e)
			}
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			proxy := &lossProxy{line: line, wallet: wallet, target: target, lost: make(chan struct{})}
			server := grpc.NewServer()
			linev1.RegisterLineServiceServer(server, proxy)
			walletv1.RegisterWalletServiceServer(server, proxy)
			go func() { _ = server.Serve(listener) }()
			defer server.Stop()
			httpListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			httpAddr := httpListener.Addr().String()
			_ = httpListener.Close()
			var logs bytes.Buffer
			start := func() *exec.Cmd {
				cmd := exec.Command(binary)
				cmd.Stdout = &logs
				cmd.Stderr = &logs
				cmd.Env = append(os.Environ(), "DATABASE_URL="+f.dbURL, "LINE_GRPC_ADDR="+listener.Addr().String(), "WALLET_GRPC_ADDR="+listener.Addr().String(), "BETS_LEASE=200ms", "BETS_POLL_INTERVAL=10ms", "BETS_RETRY_MIN=10ms", "BETS_RETRY_MAX=20ms", "BETS_HTTP_ADDR="+httpAddr)
				if e := cmd.Start(); e != nil {
					t.Fatal(e)
				}
				return cmd
			}
			first := start()
			ready := false
			for n := 0; n < 100; n++ {
				r, err := http.Get("http://" + httpAddr + "/readyz")
				if err == nil {
					r.Body.Close()
					if r.StatusCode == 200 {
						ready = true
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !ready {
				_ = first.Process.Kill()
				_ = first.Wait()
				t.Fatalf("server never became ready: %s", logs.String())
			}
			select {
			case <-proxy.lost:
			case <-time.After(10 * time.Second):
				var activity string
				_ = f.db.QueryRow(ctx, `SELECT coalesce(string_agg(state || '/' || coalesce(wait_event,'') || '/' || left(query,80), E'\n'),'') FROM pg_stat_activity WHERE datname=current_database()`).Scan(&activity)
				_ = first.Process.Kill()
				_ = first.Wait()
				var state, stage string
				var attempts int
				_ = f.db.QueryRow(ctx, "SELECT status,stage,attempts FROM bets WHERE id=$1", bet.ID).Scan(&state, &stage, &attempts)
				t.Fatalf("action did not reach proxy; status=%s stage=%s attempts=%d output: %s activity: %s", state, stage, attempts, logs.String(), activity)
			}
			if e = first.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			_ = first.Wait()
			second := start()
			defer func() { _ = second.Process.Kill(); _ = second.Wait() }()
			finished := false
			for n := 0; n < 200; n++ {
				var state string
				if e = f.db.QueryRow(ctx, "SELECT status FROM bets WHERE id=$1", bet.ID).Scan(&state); e != nil {
					t.Fatal(e)
				}
				if state == "accepted" {
					finished = true
					break
				}
				if state == "rejected" {
					t.Fatalf("unexpected rejection after %s", target)
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !finished {
				var state, stage string
				var attempts int
				_ = f.db.QueryRow(ctx, "SELECT status,stage,attempts FROM bets WHERE id=$1", bet.ID).Scan(&state, &stage, &attempts)
				t.Fatalf("bet did not recover after restart: status=%s stage=%s attempts=%d logs=%s", state, stage, attempts, logs.String())
			}
			var confirmed int64
			if e = f.db.QueryRow(ctx, "SELECT confirmed_odds FROM bets WHERE id=$1", bet.ID).Scan(&confirmed); e != nil || confirmed != whole*100+frac {
				t.Fatalf("confirmed odds=%d error=%v", confirmed, e)
			}
			replay, e := f.store.Submit(ctx, "restart-"+target, offer["offerId"].(string), 300, whole*100+frac)
			if e != nil || replay.ID != bet.ID {
				t.Fatalf("replay %+v error %v", replay, e)
			}
		})
	}
	res, e := http.Get(f.walletHTTP + "/api/v1/wallet")
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var balance map[string]string
	if e = json.NewDecoder(res.Body).Decode(&balance); e != nil || balance["available"] != "21.00" || balance["reserved"] != "0.00" {
		t.Fatalf("balance=%v error=%v", balance, e)
	}

}
