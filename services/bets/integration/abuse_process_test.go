//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type abuseProxy struct {
	linev1.UnimplementedLineServiceServer
	walletv1.UnimplementedWalletServiceServer
	line    linev1.LineServiceClient
	wallet  walletv1.WalletServiceClient
	method  string
	arrived chan struct{}
	release chan struct{}
}

func (p *abuseProxy) gate(ctx context.Context, method string) error {
	if p.method != method {
		return nil
	}
	select {
	case p.arrived <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *abuseProxy) ConfirmOffer(ctx context.Context, req *linev1.ConfirmOfferRequest) (*linev1.ConfirmOfferResponse, error) {
	if err := p.gate(ctx, "confirm"); err != nil {
		return nil, err
	}
	return p.line.ConfirmOffer(ctx, req)
}
func (p *abuseProxy) Reserve(ctx context.Context, req *walletv1.ReserveRequest) (*walletv1.ReserveResponse, error) {
	if err := p.gate(ctx, "reserve"); err != nil {
		return nil, err
	}
	return p.wallet.Reserve(ctx, req)
}
func (p *abuseProxy) Capture(ctx context.Context, req *walletv1.ReserveKey) (*walletv1.CaptureResponse, error) {
	return p.wallet.Capture(ctx, req)
}
func (p *abuseProxy) Release(ctx context.Context, req *walletv1.ReserveKey) (*walletv1.ReleaseResponse, error) {
	return p.wallet.Release(ctx, req)
}

type abuseSystem struct {
	f      *systemFixture
	url    string
	proxy  *abuseProxy
	wallet *pgx.Conn
}

func startAbuseSystem(t *testing.T, gate string, workers int) *abuseSystem {
	t.Helper()
	f := systemWithLineEnv(t, map[string]string{"LINE_GENERATION_INTERVAL": "1h"})
	ctx := context.Background()
	dial := func(addr string) *grpc.ClientConn {
		c, e := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	p := &abuseProxy{line: linev1.NewLineServiceClient(dial(f.lineGRPC)), wallet: walletv1.NewWalletServiceClient(dial(f.walletGRPC)), method: gate, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	gs := grpc.NewServer()
	linev1.RegisterLineServiceServer(gs, p)
	walletv1.RegisterWalletServiceServer(gs, p)
	go func() { _ = gs.Serve(listener) }()
	t.Cleanup(gs.Stop)
	root, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	binary := filepath.Join(t.TempDir(), "bets")
	build := exec.Command("go", "build", "-o", binary, "./services/bets/cmd/server")
	build.Dir = root
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build bets: %v %s", e, out)
	}
	var first string
	for i := 0; i < workers; i++ {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		addr := l.Addr().String()
		_ = l.Close()
		cmd := exec.Command(binary)
		var logs bytes.Buffer
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		cmd.Env = append(os.Environ(), "DATABASE_URL="+f.dbURL, "LINE_GRPC_ADDR="+listener.Addr().String(), "WALLET_GRPC_ADDR="+listener.Addr().String(), "BETS_HTTP_ADDR="+addr, "BETS_POLL_INTERVAL=10ms", "BETS_GRPC_DEADLINE=10s")
		if e := cmd.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		base := "http://" + addr
		deadline := time.Now().Add(15 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			r, e := http.Get(base + "/readyz")
			if e == nil {
				_ = r.Body.Close()
				if r.StatusCode == 200 {
					ready = true
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("bets not ready: %s", logs.String())
		}
		if i == 0 {
			first = base
		}
	}
	walletURL := strings.Replace(f.dbURL, "/bets?", "/wallet?", 1)
	db, e := pgx.Connect(ctx, walletURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	return &abuseSystem{f: f, url: first, proxy: p, wallet: db}
}
func abuseRequest(t *testing.T, url, body, key string) (int, map[string]any) {
	t.Helper()
	req, e := http.NewRequest("POST", url, strings.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Body.Close() }()
	var data map[string]any
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &data); e != nil {
		t.Fatalf("decode %s: %v", raw, e)
	}
	return r.StatusCode, data
}
func abuseBet(t *testing.T, base, id string) map[string]any {
	t.Helper()
	r, e := http.Get(base + "/api/v1/bets/" + id)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Body.Close() }()
	var v map[string]any
	if e = json.NewDecoder(r.Body).Decode(&v); e != nil || r.StatusCode != 200 {
		t.Fatalf("GET %d: %v", r.StatusCode, e)
	}
	return v
}
func abuseFinish(t *testing.T, base, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		v := abuseBet(t, base, id)
		if v["status"] != "pending" {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("bet %s timed out", id)
	return nil
}
func abuseBalance(t *testing.T, f *systemFixture, available string) {
	t.Helper()
	r, e := http.Get(f.walletHTTP + "/api/v1/wallet")
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Body.Close() }()
	var v map[string]string
	if e = json.NewDecoder(r.Body).Decode(&v); e != nil || v["available"] != available || v["reserved"] != "0.00" {
		t.Fatalf("balance %v: %v", v, e)
	}
}
func abuseCaptures(t *testing.T, s *abuseSystem, want int) {
	t.Helper()
	var n int
	if e := s.wallet.QueryRow(context.Background(), "SELECT count(*) FROM wallet_journal WHERE operation='capture'").Scan(&n); e != nil || n != want {
		t.Fatalf("captures=%d want=%d err=%v", n, want, e)
	}
}
func abuseOffer(t *testing.T, s *abuseSystem, match string) map[string]any {
	t.Helper()
	return post(t, s.f.lineHTTP+"/api/v1/line/offers", fmt.Sprintf(`{"matchId":%q,"outcome":"HOME"}`, match), "")
}
func abuseBody(o map[string]any, amount string) string {
	return fmt.Sprintf(`{"offerId":%q,"odds":%q,"amount":%q}`, o["offerId"], o["odds"], amount)
}
func abuseWaitGate(t *testing.T, s *abuseSystem) {
	p := s.proxy
	t.Helper()
	for i := 0; i < 2; i++ {
		select {
		case <-p.arrived:
		case <-time.After(8 * time.Second):
			close(p.release)
			rows, err := s.f.db.Query(context.Background(), "SELECT status, stage, reason FROM bets")
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var status, stage, reason string
					if rows.Scan(&status, &stage, &reason) == nil {
						t.Logf("bet status=%s stage=%s reason=%s", status, stage, reason)
					}
				}
			}
			t.Fatal("two concurrent calls did not reach barrier")
		}
	}
	close(p.release)
}

func TestAbuseProcessConcurrentOfferConfirmation(t *testing.T) {
	s := startAbuseSystem(t, "confirm", 2)
	post(t, s.f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "credit")
	offer := abuseOffer(t, s, "match-1")
	var ids [2]string
	for i := range ids {
		code, v := abuseRequest(t, s.url+"/api/v1/bets", abuseBody(offer, "3.00"), fmt.Sprintf("offer-%d", i))
		if code != 202 {
			t.Fatalf("submit %d %v", code, v)
		}
		ids[i] = v["betId"].(string)
	}
	abuseWaitGate(t, s)
	counts := map[string]int{}
	for _, id := range ids {
		v := abuseFinish(t, s.url, id)
		if v["status"] == "rejected" && v["reason"] != "offer_unavailable" {
			t.Fatalf("unexpected %v", v)
		}
		counts[v["status"].(string)]++
	}
	if counts["accepted"] != 1 || counts["rejected"] != 1 {
		t.Fatal(counts)
	}
	abuseBalance(t, s.f, "7.00")
	abuseCaptures(t, s, 1)
}
func TestAbuseProcessConcurrentBalance(t *testing.T) {
	s := startAbuseSystem(t, "reserve", 2)
	post(t, s.f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "credit")
	var ids [2]string
	for i := range ids {
		o := abuseOffer(t, s, fmt.Sprintf("match-%d", i+1))
		code, v := abuseRequest(t, s.url+"/api/v1/bets", abuseBody(o, "7.00"), fmt.Sprintf("balance-%d", i))
		if code != 202 {
			t.Fatalf("submit %d %v", code, v)
		}
		ids[i] = v["betId"].(string)
	}
	abuseWaitGate(t, s)
	counts := map[string]int{}
	for _, id := range ids {
		v := abuseFinish(t, s.url, id)
		if v["status"] == "rejected" && v["reason"] != "insufficient_funds" {
			t.Fatalf("unexpected %v", v)
		}
		counts[v["status"].(string)]++
	}
	if counts["accepted"] != 1 || counts["rejected"] != 1 {
		t.Fatal(counts)
	}
	abuseBalance(t, s.f, "3.00")
	abuseCaptures(t, s, 1)
}
func TestAbuseProcessModifiedIdempotencyKey(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	post(t, s.f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "credit")
	o := abuseOffer(t, s, "match-1")
	other := abuseOffer(t, s, "match-2")
	original := abuseBody(o, "3.00")
	code, v := abuseRequest(t, s.url+"/api/v1/bets", original, "bet-key")
	if code != 202 {
		t.Fatalf("submit %d %v", code, v)
	}
	id := v["betId"].(string)
	var wg sync.WaitGroup
	errs := make(chan string, 3)
	for _, body := range []string{abuseBody(other, "3.00"), abuseBody(o, "4.00"), fmt.Sprintf(`{"offerId":%q,"odds":"999.99","amount":"3.00"}`, o["offerId"])} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", s.url+"/api/v1/bets", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "bet-key")
			r, e := http.DefaultClient.Do(req)
			if e != nil {
				errs <- e.Error()
				return
			}
			defer func() { _ = r.Body.Close() }()
			var v map[string]string
			if e = json.NewDecoder(r.Body).Decode(&v); e != nil || r.StatusCode != 409 || v["code"] != "idempotency_conflict" {
				errs <- fmt.Sprintf("status %d body %v error %v", r.StatusCode, v, e)
			}
		}(body)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	got := abuseFinish(t, s.url, id)
	if got["status"] != "accepted" || got["amount"] != "3.00" || got["odds"] != o["odds"] {
		t.Fatal(got)
	}
	abuseBalance(t, s.f, "7.00")
	abuseCaptures(t, s, 1)
}
func TestAbuseProcessInvalidAmounts(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	post(t, s.f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "credit")
	o := abuseOffer(t, s, "match-1")
	for i, amount := range []string{"-1", "0", "1.001", "92233720368547758.08"} {
		key := fmt.Sprintf("invalid-%d", i)
		code, v := abuseRequest(t, s.url+"/api/v1/bets", abuseBody(o, amount), key)
		if code != 400 || v["code"] != "invalid_request" {
			t.Fatalf("bet %s %d %v", amount, code, v)
		}
		code, v = abuseRequest(t, s.f.walletHTTP+"/api/v1/wallet/credits", fmt.Sprintf(`{"amount":%q}`, amount), key)
		if code != 400 || v["code"] != "invalid_request" {
			t.Fatalf("credit %s %d %v", amount, code, v)
		}
	}
	var n int
	if e := s.f.db.QueryRow(context.Background(), "SELECT count(*) FROM bets").Scan(&n); e != nil || n != 0 {
		t.Fatalf("bets=%d err=%v", n, e)
	}
	abuseBalance(t, s.f, "10.00")
	abuseCaptures(t, s, 0)
	code, v := abuseRequest(t, s.url+"/api/v1/bets", abuseBody(o, "3.00"), "valid")
	if code != 202 {
		t.Fatalf("valid %d %v", code, v)
	}
	result := abuseFinish(t, s.url, v["betId"].(string))
	if result["status"] != "accepted" {
		t.Fatal(result)
	}
	abuseBalance(t, s.f, "7.00")
	abuseCaptures(t, s, 1)
}
