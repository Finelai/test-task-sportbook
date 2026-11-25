package main

import (
	"context"
	"errors"
	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/postgres"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/rpc"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

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

	listener, err := net.Listen("tcp", ":9093")
	if err != nil {
		log.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	walletv1.RegisterWalletServiceServer(grpcServer, rpc.New(store))
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("grpc server: %v", err)
			stop()
		}
	}()
	go func() { <-ctx.Done(); grpcServer.Stop() }()
	server := &http.Server{Addr: ":8082", Handler: httpapi.New(store), ReadHeaderTimeout: 5 * time.Second}
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
