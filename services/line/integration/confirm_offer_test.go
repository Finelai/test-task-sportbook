//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"github.com/finelai/test-task-sportbook/services/line/internal/postgres"
	"github.com/finelai/test-task-sportbook/services/line/internal/rpc"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestConfirmOfferPostgres(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "postgres:17.7-alpine", Env: map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "line"}, ExposedPorts: []string{"5432/tcp"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("postgres://postgres:test@%s:%s/line?sslmode=disable", host, port.Port())
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	svc := app.New(store)
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err = svc.Generate(ctx, i, now, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	offer, err := svc.IssueOfferNow(ctx, "player-1", "match-1", "HOME")
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	linev1.RegisterLineServiceServer(server, rpc.New(svc))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///line", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	client := linev1.NewLineServiceClient(conn)
	var wg sync.WaitGroup
	type call struct {
		req      *linev1.ConfirmOfferRequest
		response *linev1.ConfirmOfferResponse
	}
	responses := make(chan call, 20)
	errs := make(chan error, 21)
	reissued := make(chan app.Offer, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, e := app.New(store).IssueOfferNow(ctx, "player-1", "match-1", "HOME")
		if e != nil {
			errs <- e
			return
		}
		reissued <- got
	}()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &linev1.ConfirmOfferRequest{BetId: fmt.Sprintf("bet-%d", i), PlayerId: "player-1", OfferId: offer.ID, ExpectedOddsHundredths: offer.OddsHundredths}
			result, e := client.ConfirmOffer(ctx, req)
			if e != nil {
				errs <- e
				return
			}
			responses <- call{req, result}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	close(responses)
	successes := 0
	var req *linev1.ConfirmOfferRequest
	for result := range responses {
		if result.response.GetConfirmed() != nil {
			successes++
			req = result.req
		} else if result.response.GetRejected().GetReason() != linev1.RejectionReason_OFFER_UNAVAILABLE {
			t.Fatalf("rejection: %v", result.response)
		}
	}
	if len(reissued) != 1 {
		t.Fatal("missing concurrent issuance")
	}
	if successes != 1 {
		t.Fatalf("confirmed %d times", successes)
	}
	original, err := client.ConfirmOffer(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	_, err = client.ConfirmOffer(ctx, &linev1.ConfirmOfferRequest{BetId: "database-down", PlayerId: req.PlayerId, OfferId: req.OfferId, ExpectedOddsHundredths: req.ExpectedOddsHundredths})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("database error: %v", err)
	}
	reopened, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted := rpc.New(app.New(reopened))
	replay, err := restarted.ConfirmOffer(ctx, req)
	if err != nil || !proto.Equal(original, replay) {
		t.Fatalf("replay after reconnect: %v / %v, %v", original, replay, err)
	}
}
