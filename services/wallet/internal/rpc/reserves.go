package rpc

import (
	"context"
	"errors"
	"strings"

	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Store interface {
	Reserve(context.Context, string, string, int64) (app.ReserveResult, error)
	Capture(context.Context, string, string) (app.CaptureResult, error)
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

func (s *Server) Capture(ctx context.Context, key *walletv1.ReserveKey) (*walletv1.CaptureResponse, error) {
	if key == nil || strings.TrimSpace(key.BetId) == "" || strings.TrimSpace(key.PlayerId) == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid reserve key")
	}
	result, err := s.store.Capture(ctx, key.BetId, key.PlayerId)
	if errors.Is(err, app.ErrReserveNotFound) {
		return nil, status.Error(codes.NotFound, "reserve not found")
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.Unavailable, "wallet temporarily unavailable")
	}
	switch result {
	case app.Captured:
		return &walletv1.CaptureResponse{Result: walletv1.CaptureResult_CAPTURED}, nil
	case app.CaptureParameterConflict:
		return &walletv1.CaptureResponse{Result: walletv1.CaptureResult_CAPTURE_PARAMETER_CONFLICT}, nil
	case app.AlreadyReleased:
		return &walletv1.CaptureResponse{Result: walletv1.CaptureResult_ALREADY_RELEASED}, nil
	default:
		return nil, status.Error(codes.Internal, "unknown capture result")
	}
}
