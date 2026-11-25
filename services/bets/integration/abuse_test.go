//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/bets/internal/app"
	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// The attacker can only use the player's HTTP API; the test drives the worker
// directly while its calls reach the real Line and Wallet gRPC servers.
func TestAbuseOfferPriceAndReuse(t *testing.T) {
	f := system(t)
	ctx := context.Background()
	dial := func(address string) *grpc.ClientConn {
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	worker := app.NewWorker(f.store, linev1.NewLineServiceClient(dial(f.lineGRPC)), walletv1.NewWalletServiceClient(dial(f.walletGRPC)), app.Config{RetryMin: time.Millisecond, RetryMax: time.Millisecond})
	bets := httptest.NewServer(httpapi.New(f.store))
	t.Cleanup(bets.Close)
	post(t, f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "abuse-credit")
	offer := post(t, f.lineHTTP+"/api/v1/line/offers", `{"matchId":"match-1","outcome":"HOME"}`, "")
	offerID, odds := offer["offerId"].(string), offer["odds"].(string)
	var whole, fraction int64
	if _, err := fmt.Sscanf(odds, "%d.%d", &whole, &fraction); err != nil {
		t.Fatal(err)
	}
	modified := whole*100 + fraction + 1
	body := func(price string) string {
		return fmt.Sprintf(`{"offerId":%q,"odds":%q,"amount":"3.00"}`, offerID, price)
	}
	submit := func(key, price string) string {
		return post(t, bets.URL+"/api/v1/bets", body(price), key)["betId"].(string)
	}
	read := func(id string) map[string]any {
		resp, err := http.Get(bets.URL + "/api/v1/bets/" + id)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("read: %d %v", resp.StatusCode, err)
		}
		return result
	}
	finish := func(id string) map[string]any {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if err := worker.ProcessOne(ctx); err != nil {
				t.Fatal(err)
			}
			result := read(id)
			if result["status"] != "pending" {
				return result
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("bet %s did not finish", id)
		return nil
	}
	tampered := submit("abuse-tampered", fmt.Sprintf("%d.%02d", modified/100, modified%100))
	if result := finish(tampered); result["status"] != "rejected" || result["reason"] != "offer_mismatch" {
		t.Fatalf("tampered offer: %v", result)
	}
	legitimate := submit("abuse-legitimate", odds)
	if result := finish(legitimate); result["status"] != "accepted" || result["odds"] != odds {
		t.Fatalf("legitimate bet: %v", result)
	}
	reused := submit("abuse-reuse", odds)
	if result := finish(reused); result["status"] != "rejected" || result["reason"] != "offer_unavailable" {
		t.Fatalf("reused offer: %v", result)
	}
	resp, err := http.Get(f.walletHTTP + "/api/v1/wallet")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var balance map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil || balance["available"] != "7.00" || balance["reserved"] != "0.00" {
		t.Fatalf("balance=%v error=%v", balance, err)
	}
}

func TestAbuseConcurrentOfferReuse(t *testing.T) {
	f := system(t)
	ctx := context.Background()
	connect := func(address string) *grpc.ClientConn {
		c, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	worker := app.NewWorker(f.store, linev1.NewLineServiceClient(connect(f.lineGRPC)), walletv1.NewWalletServiceClient(connect(f.walletGRPC)), app.Config{})
	server := httptest.NewServer(httpapi.New(f.store))
	t.Cleanup(server.Close)
	post(t, f.walletHTTP+"/api/v1/wallet/credits", `{"amount":"10.00"}`, "race-credit")
	offer := post(t, f.lineHTTP+"/api/v1/line/offers", `{"matchId":"match-1","outcome":"HOME"}`, "")
	body := fmt.Sprintf(`{"offerId":%q,"odds":%q,"amount":"3.00"}`, offer["offerId"], offer["odds"])
	var ids [2]string
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i] = post(t, server.URL+"/api/v1/bets", body, fmt.Sprintf("race-%d", i))["betId"].(string)
		}(i)
	}
	close(start)
	wg.Wait()
	if ids[0] == ids[1] {
		t.Fatal("different keys produced the same bet")
	}
	for i := 0; i < 12; i++ {
		if err := worker.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	counts := map[string]int{}
	for _, id := range ids {
		response, err := http.Get(server.URL + "/api/v1/bets/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var bet map[string]string
		err = json.NewDecoder(response.Body).Decode(&bet)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bet["status"] == "rejected" && bet["reason"] != "offer_unavailable" {
			t.Fatalf("unexpected rejection: %v", bet)
		}
		counts[bet["status"]]++
	}
	if counts["accepted"] != 1 || counts["rejected"] != 1 {
		t.Fatalf("unexpected outcomes: %v", counts)
	}
	response, err := http.Get(f.walletHTTP + "/api/v1/wallet")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var balance map[string]string
	if err := json.NewDecoder(response.Body).Decode(&balance); err != nil || balance["available"] != "7.00" || balance["reserved"] != "0.00" {
		t.Fatalf("balance=%v error=%v", balance, err)
	}
}
