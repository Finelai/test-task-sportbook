//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/rpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestWalletCreditReserveCaptureFlow(t *testing.T) {
	f := newCreditFixture(t)
	handler := httpapi.New(f.store)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	walletv1.RegisterWalletServiceServer(server, rpc.New(f.store))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///wallet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := walletv1.NewWalletServiceClient(conn)
	checkBalance := func(available, reserved string) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/wallet", nil))
		var balance struct {
			Available string `json:"available"`
			Reserved  string `json:"reserved"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &balance); err != nil || w.Code != http.StatusOK || balance.Available != available || balance.Reserved != reserved {
			t.Fatalf("balance: status=%d body=%s error=%v", w.Code, w.Body.String(), err)
		}
	}
	checkBalance("0.00", "0.00")
	response := credit(handler, "flow-credit", "10.00")
	if response.Code != http.StatusCreated {
		t.Fatalf("credit: %d %s", response.Code, response.Body.String())
	}
	var credited struct {
		OperationID string `json:"operationId"`
		Available   string `json:"available"`
		Reserved    string `json:"reserved"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &credited); err != nil || credited.OperationID == "" || credited.Available != "10.00" || credited.Reserved != "0.00" {
		t.Fatalf("credit: %s error=%v", response.Body.String(), err)
	}
	ctx := context.Background()
	reserved, err := client.Reserve(ctx, &walletv1.ReserveRequest{BetId: "flow-bet", PlayerId: "player-1", AmountMinor: 300})
	if err != nil || reserved.GetResult() != walletv1.ReserveResult_RESERVED {
		t.Fatalf("reserve: %v %v", reserved, err)
	}
	checkBalance("7.00", "3.00")
	captured, err := client.Capture(ctx, &walletv1.ReserveKey{BetId: "flow-bet", PlayerId: "player-1"})
	if err != nil || captured.GetResult() != walletv1.CaptureResult_CAPTURED {
		t.Fatalf("capture: %v %v", captured, err)
	}
	checkBalance("7.00", "0.00")
}
