//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/bets/internal/app"
	"github.com/finelai/test-task-sportbook/services/bets/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type systemFixture struct {
	store                                      *postgres.Store
	db                                         *pgx.Conn
	lineHTTP, walletHTTP, lineGRPC, walletGRPC string
	dbURL                                      string
}

func system(t *testing.T) *systemFixture {
	t.Helper()
	ctx := context.Background()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"line", "wallet"} {
		cmd := exec.Command("docker", "build", "-q", "-t", "sportbook-integration-"+service, "-f", filepath.Join(root, "services", service, "Dockerfile"), root)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("build %s: %v: %s", service, e, out)
		}
	}
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(ctx) })
	start := func(req testcontainers.ContainerRequest) testcontainers.Container {
		c, e := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Terminate(ctx) })
		return c
	}
	pg := start(testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test"}, Networks: []string{nw.Name}, NetworkAliases: map[string][]string{nw.Name: {"db"}}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)})
	endpoint := func(c testcontainers.Container, port string) string {
		h, e := c.Host(ctx)
		if e != nil {
			t.Fatal(e)
		}
		p, e := c.MappedPort(ctx, nat.Port(port+"/tcp"))
		if e != nil {
			t.Fatal(e)
		}
		return h + ":" + p.Port()
	}
	base := "postgres://postgres:test@" + endpoint(pg, "5432") + "/"
	admin, e := pgx.Connect(ctx, base+"postgres?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	for _, db := range []string{"line", "wallet", "bets"} {
		if _, e = admin.Exec(ctx, "CREATE DATABASE "+db); e != nil {
			t.Fatal(e)
		}
	}
	_ = admin.Close(ctx)
	startService := func(name, httpPort, grpcPort string) testcontainers.Container {
		return start(testcontainers.ContainerRequest{Image: "sportbook-integration-" + name, Env: map[string]string{"DATABASE_URL": "postgres://postgres:test@db:5432/" + name + "?sslmode=disable"}, Networks: []string{nw.Name}, ExposedPorts: []string{httpPort + "/tcp", grpcPort + "/tcp"}, WaitingFor: wait.ForHTTP("/readyz").WithPort(nat.Port(httpPort + "/tcp")).WithStartupTimeout(time.Minute)})
	}
	line := startService("line", "8081", "9092")
	wallet := startService("wallet", "8082", "9093")
	store, e := postgres.Open(ctx, base+"bets?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(store.Close)
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	db, e := pgx.Connect(ctx, base+"bets?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	return &systemFixture{store, db, "http://" + endpoint(line, "8081"), "http://" + endpoint(wallet, "8082"), endpoint(line, "9092"), endpoint(wallet, "9093"), base + "bets?sslmode=disable"}
}
func post(t *testing.T, url, body, key string) map[string]any {
	t.Helper()
	req, e := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var data map[string]any
	if e = json.NewDecoder(res.Body).Decode(&data); e != nil || res.StatusCode >= 300 {
		t.Fatalf("POST %s: status %d response %v error %v", url, res.StatusCode, data, e)
	}
	return data
}
func TestRecoverAfterActions(t *testing.T) {
	f := system(t)
	ctx := context.Background()
	post(t, f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"30.00"}`, "credit")
	for i, action := range []string{"ConfirmOffer", "Reserve", "Capture"} {
		t.Run(action, func(t *testing.T) {
			offer := post(t, f.lineHTTP+"/api/v1/line/offers", fmt.Sprintf(`{"matchId":"match-%d","outcome":"HOME"}`, i+1), "")
			offerID := offer["offerId"].(string)
			odds := offer["odds"].(string)
			var whole, fract int64
			if _, e := fmt.Sscanf(odds, "%d.%d", &whole, &fract); e != nil {
				t.Fatal(e)
			}
			bet, e := f.store.Submit(ctx, "key-"+action, offerID, 300, whole*100+fract)
			if e != nil {
				t.Fatal(e)
			}
			var dropped atomic.Bool
			interceptor := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				e := invoke(ctx, method, req, reply, cc, opts...)
				if e == nil && method[len(method)-len(action):] == action && dropped.CompareAndSwap(false, true) {
					return status.Error(codes.Unavailable, "response dropped")
				}
				return e
			}
			dial := func(addr string) *grpc.ClientConn {
				conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(interceptor))
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { _ = conn.Close() })
				return conn
			}
			line := linev1.NewLineServiceClient(dial(f.lineGRPC))
			wallet := walletv1.NewWalletServiceClient(dial(f.walletGRPC))
			cfg := app.Config{RetryMin: time.Millisecond, RetryMax: time.Millisecond, Lease: time.Second}
			for n := 0; n < 50; n++ {
				worker := app.NewWorker(f.store, line, wallet, cfg)
				if e = worker.ProcessOne(ctx); e != nil {
					t.Fatal(e)
				}
				var state string
				if e = f.db.QueryRow(ctx, "SELECT status FROM bets WHERE id=$1", bet.ID).Scan(&state); e != nil {
					t.Fatal(e)
				}
				if state == "accepted" {
					break
				}
				if state == "rejected" {
					t.Fatalf("bet rejected at %s", action)
				}
				time.Sleep(5 * time.Millisecond)
			}
			var state, match, outcome string
			var price int64
			if e = f.db.QueryRow(ctx, "SELECT status,match_id,outcome,confirmed_odds FROM bets WHERE id=$1", bet.ID).Scan(&state, &match, &outcome, &price); e != nil {
				t.Fatal(e)
			}
			if state != "accepted" || match == "" || outcome != "HOME" || price != whole*100+fract || !dropped.Load() {
				t.Fatalf("state=%s match=%s outcome=%s odds=%d dropped=%v", state, match, outcome, price, dropped.Load())
			}
			replay, e := f.store.Submit(ctx, "key-"+action, offerID, 300, whole*100+fract)
			if e != nil || replay.ID != bet.ID {
				t.Fatalf("replay=%+v error=%v", replay, e)
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

func TestRejectWithoutCharge(t *testing.T) {
	f := system(t)
	ctx := context.Background()
	dial := func(addr string) *grpc.ClientConn {
		c, e := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	l := linev1.NewLineServiceClient(dial(f.lineGRPC))
	arrived := make(chan string, 2)
	release := make(chan struct{})
	walletConn, e := grpc.NewClient(f.walletGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if method == walletv1.WalletService_Reserve_FullMethodName {
			select {
			case arrived <- req.(*walletv1.ReserveRequest).BetId:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return invoke(ctx, method, req, reply, cc, opts...)
	}))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = walletConn.Close() })
	w := walletv1.NewWalletServiceClient(walletConn)
	worker := app.NewWorker(f.store, l, w, app.Config{RetryMin: time.Millisecond, RetryMax: time.Millisecond})
	post(t, f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"3.00"}`, "credit")
	invalid, e := f.store.Submit(ctx, "invalid", "nonexistent", 100, 210)
	if e != nil {
		t.Fatal(e)
	}
	process := func(id string) (string, string) {
		t.Helper()
		for i := 0; i < 100; i++ {
			if e := worker.ProcessOne(ctx); e != nil {
				t.Fatal(e)
			}
			var state, reason string
			if e := f.db.QueryRow(ctx, `SELECT status,reason FROM bets WHERE id=$1`, id).Scan(&state, &reason); e != nil {
				t.Fatal(e)
			}
			if state != "pending" {
				return state, reason
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("bet did not finish")
		return "", ""
	}
	state, reason := process(invalid.ID)
	if state != "rejected" || reason != "offer_unavailable" {
		t.Fatalf("invalid offer: %s %s", state, reason)
	}
	ids := make([]string, 2)
	for i := range ids {
		offer := post(t, f.lineHTTP+"/api/v1/line/offers", fmt.Sprintf(`{"matchId":"match-%d","outcome":"HOME"}`, i+1), "")
		var whole, frac int64
		if _, e = fmt.Sscanf(offer["odds"].(string), "%d.%d", &whole, &frac); e != nil {
			t.Fatal(e)
		}
		bet, e := f.store.Submit(ctx, fmt.Sprint("concurrent-", i), offer["offerId"].(string), 300, whole*100+frac)
		if e != nil {
			t.Fatal(e)
		}
		ids[i] = bet.ID
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 10; n++ {
				if e := worker.ProcessOne(ctx); e != nil {
					t.Errorf("worker: %v", e)
					return
				}
			}
		}()
	}
	var calls []string
	for len(calls) < 2 {
		select {
		case id := <-arrived:
			calls = append(calls, id)
		case <-time.After(time.Second):
			close(release)
			wg.Wait()
			t.Fatalf("expected concurrent reserve calls, got %v", calls)
		}
	}
	close(release)
	wg.Wait()
	if calls[0] == calls[1] {
		t.Fatalf("same bet reserved twice: %v", calls)
	}
	for _, id := range ids {
		state, reason = process(id)
		if state != "accepted" && !(state == "rejected" && reason == "insufficient_funds") {
			t.Fatalf("status=%s reason=%s", state, reason)
		}
	}
	var accepted, rejected int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='accepted'),count(*) FILTER (WHERE reason='insufficient_funds') FROM bets WHERE id=ANY($1)`, ids).Scan(&accepted, &rejected); e != nil || accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d err=%v", accepted, rejected, e)
	}
	res, e := http.Get(f.walletHTTP + "/api/v1/wallet")
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var balance map[string]string
	if e = json.NewDecoder(res.Body).Decode(&balance); e != nil || balance["available"] != "0.00" || balance["reserved"] != "0.00" {
		t.Fatalf("balance=%v error=%v", balance, e)
	}
}
