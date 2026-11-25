package main

import (
	"context"
	"net"
	"testing"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestConfirmOffer(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	linev1.RegisterLineServiceServer(server, lineServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///contract-check", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	response, err := linev1.NewLineServiceClient(conn).ConfirmOffer(context.Background(), &linev1.ConfirmOfferRequest{BetId: "bet-1", PlayerId: "player-1", OfferId: "offer-1", ExpectedOddsHundredths: 210})
	if err != nil {
		t.Fatal(err)
	}
	confirmed := response.GetConfirmed()
	if confirmed == nil || confirmed.GetMatchId() != "match-1" || confirmed.GetOutcome() != linev1.Outcome_HOME || confirmed.GetOddsHundredths() != 210 {
		t.Fatalf("unexpected confirmation: %v", response)
	}
}
