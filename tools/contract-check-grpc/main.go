// Command contract-check-grpc serves fixed gRPC examples for local contract inspection.
package main

import (
	"context"
	"log"
	"net"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"google.golang.org/grpc"
)

type lineServer struct {
	linev1.UnimplementedLineServiceServer
}

func (lineServer) ConfirmOffer(context.Context, *linev1.ConfirmOfferRequest) (*linev1.ConfirmOfferResponse, error) {
	return &linev1.ConfirmOfferResponse{Result: &linev1.ConfirmOfferResponse_Confirmed{Confirmed: &linev1.ConfirmedOffer{MatchId: "match-1", Outcome: linev1.Outcome_HOME, OddsHundredths: 210}}}, nil
}

func main() {
	listener, err := net.Listen("tcp", ":9091")
	if err != nil {
		log.Fatal(err)
	}
	server := grpc.NewServer()
	linev1.RegisterLineServiceServer(server, lineServer{})
	log.Fatal(server.Serve(listener))
}
