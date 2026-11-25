package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"github.com/finelai/test-task-sportbook/services/line/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/line/internal/postgres"
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
	service := app.New(store)
	for source := 0; source < 2; source++ {
		if err := service.Generate(ctx, source, time.Now(), time.Now().UnixNano()+int64(source)); err != nil {
			log.Fatal(err)
		}
	}

	for source := 0; source < 2; source++ {
		source := source
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				if err := service.Generate(ctx, source, time.Now(), time.Now().UnixNano()+int64(source)); err != nil && ctx.Err() == nil {
					log.Printf("generate: %v", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
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
