//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func userLine(t *testing.T, s *abuseSystem) map[string]any {
	t.Helper()
	r, e := http.Get(s.f.lineHTTP + "/api/v1/line")
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Body.Close() }()
	var v map[string]any
	if e = json.NewDecoder(r.Body).Decode(&v); e != nil || r.StatusCode != 200 {
		t.Fatalf("line: %d %v", r.StatusCode, e)
	}
	matches, ok := v["matches"].([]any)
	if !ok || len(matches) != 8 {
		t.Fatalf("line matches: %v", v)
	}
	m := matches[0].(map[string]any)
	if m["open"] != true {
		t.Fatalf("market closed: %v", m)
	}
	return m
}
func userCredit(t *testing.T, s *abuseSystem, amount, key string) map[string]any {
	t.Helper()
	code, v := abuseRequest(t, s.f.walletHTTP+"/api/v1/wallet/credits", fmt.Sprintf(`{"amount":%q}`, amount), key)
	if code != 201 {
		t.Fatalf("credit %d %v", code, v)
	}
	return v
}
func userSubmit(t *testing.T, s *abuseSystem, o map[string]any, amount, key string) string {
	t.Helper()
	code, v := abuseRequest(t, s.url+"/api/v1/bets", abuseBody(o, amount), key)
	if code != 202 {
		t.Fatalf("bet %d %v", code, v)
	}
	id, ok := v["betId"].(string)
	if !ok || id == "" {
		t.Fatalf("missing id: %v", v)
	}
	return id
}
func userAccepted(t *testing.T, s *abuseSystem, id, amount string, o map[string]any) {
	t.Helper()
	b := abuseFinish(t, s.url, id)
	if b["betId"] != id || b["status"] != "accepted" || b["amount"] != amount || b["odds"] != o["odds"] || b["reason"] != nil {
		t.Fatalf("accepted bet: %v", b)
	}
}
func userJournal(t *testing.T, s *abuseSystem, operation string, want int) {
	t.Helper()
	var n int
	if e := s.wallet.QueryRow(context.Background(), "SELECT count(*) FROM wallet_journal WHERE operation=$1", operation).Scan(&n); e != nil || n != want {
		t.Fatalf("journal %s got %d want %d: %v", operation, n, want, e)
	}
}
func userOffer(t *testing.T, s *abuseSystem) map[string]any {
	t.Helper()
	m := userLine(t, s)
	return abuseOffer(t, s, m["matchId"].(string))
}

func TestUserFirstAcceptedBet(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	abuseBalance(t, s.f, "0.00")
	credit := userCredit(t, s, "10.00", "credit")
	if credit["available"] != "10.00" || credit["reserved"] != "0.00" {
		t.Fatal(credit)
	}
	offer := userOffer(t, s)
	id := userSubmit(t, s, offer, "3.00", "bet")
	userAccepted(t, s, id, "3.00", offer)
	abuseBalance(t, s.f, "7.00")
	userJournal(t, s, "credit", 1)
	userJournal(t, s, "reserve", 1)
	abuseCaptures(t, s, 1)
}
func TestUserSequentialBets(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	userCredit(t, s, "10.00", "credit-1")
	first := userOffer(t, s)
	id1 := userSubmit(t, s, first, "3.00", "bet-1")
	userAccepted(t, s, id1, "3.00", first)
	abuseBalance(t, s.f, "7.00")
	userCredit(t, s, "5.00", "credit-2")
	abuseBalance(t, s.f, "12.00")
	second := abuseOffer(t, s, "match-2")
	id2 := userSubmit(t, s, second, "4.50", "bet-2")
	if id1 == id2 {
		t.Fatal("duplicate bet id")
	}
	userAccepted(t, s, id2, "4.50", second)
	userAccepted(t, s, id1, "3.00", first)
	abuseBalance(t, s.f, "7.50")
	userJournal(t, s, "credit", 2)
	userJournal(t, s, "reserve", 2)
	abuseCaptures(t, s, 2)
}
func TestUserRetriesOperations(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	first := userCredit(t, s, "10.00", "credit")
	again := userCredit(t, s, "10.00", "credit")
	if first["operationId"] == nil || first["operationId"] != again["operationId"] {
		t.Fatalf("credit replay %v %v", first, again)
	}
	abuseBalance(t, s.f, "10.00")
	o := userOffer(t, s)
	id := userSubmit(t, s, o, "3.00", "bet")
	if other := userSubmit(t, s, o, "3.00", "bet"); id != other {
		t.Fatalf("different ids %s %s", id, other)
	}
	userAccepted(t, s, id, "3.00", o)
	if other := userSubmit(t, s, o, "3.00", "bet"); id != other {
		t.Fatalf("different ids %s %s", id, other)
	}
	abuseBalance(t, s.f, "7.00")
	userJournal(t, s, "credit", 1)
	userJournal(t, s, "reserve", 1)
	abuseCaptures(t, s, 1)
}
func testLinePrice(t *testing.T, s *abuseSystem, home, other float64) {
	t.Helper()
	db, e := pgx.Connect(context.Background(), strings.Replace(s.f.dbURL, "/bets?", "/line?", 1))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close(context.Background()) }()
	sources, e := json.Marshal([2][3]float64{{home, other, other}, {home, other, other}})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	updated, e := json.Marshal([2]time.Time{now, now})
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(context.Background(), `UPDATE line_state SET state=jsonb_set(jsonb_set(state,'{markets,0,Sources}',$1::jsonb),'{markets,0,Updated}',$2::jsonb) WHERE id=1`, string(sources), string(updated))
	if e != nil {
		t.Fatal(e)
	}
}
func TestUserOfferedPricePreserved(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	userCredit(t, s, "10.00", "credit")
	testLinePrice(t, s, 2.10, 2/(1.05-1/2.10))
	o := userOffer(t, s)
	if o["odds"] != "2.10" {
		t.Fatalf("offer: %v", o)
	}
	testLinePrice(t, s, 3.10, 2/(1.05-1/3.10))
	m := userLine(t, s)
	quotes := m["quotes"].([]any)
	if quotes[0].(map[string]any)["odds"] != "3.10" {
		t.Fatalf("new price %v", quotes)
	}
	id := userSubmit(t, s, o, "3.00", "bet")
	userAccepted(t, s, id, "3.00", o)
	abuseBalance(t, s.f, "7.00")
	abuseCaptures(t, s, 1)
}
func TestUserTopsUpAfterInsufficientFunds(t *testing.T) {
	s := startAbuseSystem(t, "", 1)
	userCredit(t, s, "2.00", "credit-1")
	o := userOffer(t, s)
	id := userSubmit(t, s, o, "3.00", "bet-1")
	rejected := abuseFinish(t, s.url, id)
	if rejected["status"] != "rejected" || rejected["reason"] != "insufficient_funds" {
		t.Fatal(rejected)
	}
	abuseBalance(t, s.f, "2.00")
	abuseCaptures(t, s, 0)
	userCredit(t, s, "5.00", "credit-2")
	o2 := abuseOffer(t, s, "match-2")
	id2 := userSubmit(t, s, o2, "3.00", "bet-2")
	if id == id2 {
		t.Fatal("duplicate bet id")
	}
	userAccepted(t, s, id2, "3.00", o2)
	if b := abuseBet(t, s.url, id); b["status"] != "rejected" || b["reason"] != "insufficient_funds" {
		t.Fatal(b)
	}
	abuseBalance(t, s.f, "4.00")
	abuseCaptures(t, s, 1)
}
