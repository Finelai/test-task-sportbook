package rpc

import (
	"context"
	"strings"

	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Store interface {
	Reserve(context.Context, string, string, int64) (app.ReserveResult, error)
}
type Server struct {
	walletv1.UnimplementedWalletServiceServer
	store Store
}

func New(store Store) *Server { return &Server{store: store} }
func (s *Server) Reserve(ctx context.Context, req *walletv1.ReserveRequest) (*walletv1.ReserveResponse, error) {
	if req == nil || strings.TrimSpace(req.BetId) == "" || strings.TrimSpace(req.PlayerId) == "" || req.AmountMinor <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid reserve request")
	}
	result, err := s.store.Reserve(ctx, req.BetId, req.PlayerId, req.AmountMinor)
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.Unavailable, "wallet temporarily unavailable")
	}
	switch result {
	case app.Reserved:
		return &walletv1.ReserveResponse{Result: walletv1.ReserveResult_RESERVED}, nil
	case app.InsufficientFunds:
		return &walletv1.ReserveResponse{Result: walletv1.ReserveResult_INSUFFICIENT_FUNDS}, nil
	case app.ParameterConflict:
		return &walletv1.ReserveResponse{Result: walletv1.ReserveResult_PARAMETER_CONFLICT}, nil
	default:
		return nil, status.Error(codes.Internal, "unknown reserve result")
	}
}
