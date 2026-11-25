package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"github.com/finelai/test-task-sportbook/services/line/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/line/internal/postgres"
	"github.com/finelai/test-task-sportbook/services/line/internal/rpc"
	"google.golang.org/grpc"
)

func generationInterval(value string) (time.Duration, error) {
	if value == "" {
		return 5 * time.Second, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid LINE_GENERATION_INTERVAL: %q", value)
	}
	return duration, nil
}

func main() {
	interval, err := generationInterval(os.Getenv("LINE_GENERATION_INTERVAL"))
	if err != nil {
		log.Fatal(err)
	}
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
	service := app.New(store)
	for source := 0; source < 2; source++ {
		if err := service.Generate(ctx, source, time.Now(), time.Now().UnixNano()+int64(source)); err != nil {
			log.Fatal(err)
		}
	}

	for source := 0; source < 2; source++ {
		source := source
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					if err := service.Generate(ctx, source, now, now.UnixNano()+int64(source)); err != nil && ctx.Err() == nil {
						log.Printf("generate: %v", err)
					}
				}
			}
		}()
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := service.Refresh(ctx, now); err != nil && ctx.Err() == nil {
					log.Printf("refresh: %v", err)
				}
			}
		}
	}()

	listener, err := net.Listen("tcp", ":9092")
	if err != nil {
		log.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	linev1.RegisterLineServiceServer(grpcServer, rpc.New(service))
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("grpc server: %v", err)
			stop()
		}
	}()
	go func() { <-ctx.Done(); grpcServer.Stop() }()
	server := &http.Server{Addr: ":8081", Handler: httpapi.New(service, store), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
