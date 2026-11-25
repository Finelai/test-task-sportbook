package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/bets/internal/app"
	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/bets/internal/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func duration(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Fatalf("invalid %s", name)
	}
	return d
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := postgres.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	line, err := grpc.NewClient(os.Getenv("LINE_GRPC_ADDR"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = line.Close() }()
	wallet, err := grpc.NewClient(os.Getenv("WALLET_GRPC_ADDR"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wallet.Close() }()
	worker := app.NewWorker(store, linev1.NewLineServiceClient(line), walletv1.NewWalletServiceClient(wallet), app.Config{Lease: duration("BETS_LEASE", 15*time.Second), Deadline: duration("BETS_GRPC_DEADLINE", 2*time.Second), RetryMin: duration("BETS_RETRY_MIN", time.Second), RetryMax: duration("BETS_RETRY_MAX", 30*time.Second)})
	go func() {
		ticker := time.NewTicker(duration("BETS_POLL_INTERVAL", 200*time.Millisecond))
		defer ticker.Stop()
		for {
			if err := worker.ProcessOne(ctx); err != nil && ctx.Err() == nil {
				log.Printf("worker: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	addr := os.Getenv("BETS_HTTP_ADDR")
	if addr == "" {
		addr = ":8083"
	}
	server := &http.Server{Addr: addr, Handler: httpapi.New(store), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
