package rpc

import (
	"context"
	"errors"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	linev1.UnimplementedLineServiceServer
	service *app.Service
}

func New(service *app.Service) *Server { return &Server{service: service} }
func (s *Server) ConfirmOffer(ctx context.Context, req *linev1.ConfirmOfferRequest) (*linev1.ConfirmOfferResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	result, err := s.service.ConfirmOfferNow(ctx, app.ConfirmRequest{BetID: req.BetId, PlayerID: req.PlayerId, OfferID: req.OfferId, ExpectedOddsHundredths: req.ExpectedOddsHundredths})
	switch {
	case errors.Is(err, app.ErrInvalidRequest):
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	case errors.Is(err, app.ErrBetConflict):
		return nil, status.Error(codes.AlreadyExists, "bet id has different confirmation parameters")
	case errors.Is(err, context.Canceled):
		return nil, status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return nil, status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case err != nil:
		return nil, status.Error(codes.Unavailable, "line temporarily unavailable")
	}
	if result.Reason != "" {
		value, ok := linev1.RejectionReason_value[result.Reason]
		if !ok {
			return nil, status.Error(codes.Internal, "unknown rejection reason")
		}
		return &linev1.ConfirmOfferResponse{Result: &linev1.ConfirmOfferResponse_Rejected{Rejected: &linev1.RejectedOffer{Reason: linev1.RejectionReason(value)}}}, nil
	}
	outcome, ok := linev1.Outcome_value[result.Outcome]
	if !ok {
		return nil, status.Error(codes.Internal, "unknown outcome")
	}
	return &linev1.ConfirmOfferResponse{Result: &linev1.ConfirmOfferResponse_Confirmed{Confirmed: &linev1.ConfirmedOffer{MatchId: result.MatchID, Outcome: linev1.Outcome(outcome), OddsHundredths: result.OddsHundredths}}}, nil
}
