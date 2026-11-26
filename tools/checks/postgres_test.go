//go:build integration

package checks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresDatabaseIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	script, err := filepath.Abs("../../infra/postgres/init.sh")
	if err != nil {
		t.Fatal(err)
	}
	container, err := testcontainers.Run(ctx, "postgres:17.7-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_PASSWORD": "postgres"}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath: script, ContainerFilePath: "/docker-entrypoint-initdb.d/10-sportsbook.sh", FileMode: 0o755,
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		// The first ready message belongs to the temporary initialization server.
		testcontainers.WithWaitStrategy(wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(2*time.Minute)),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := container.PortEndpoint(ctx, "5432/tcp", "")
	if err != nil {
		t.Fatal(err)
	}

	databases := []string{"line", "wallet", "bets", "build_metrics"}
	for _, role := range databases {
		for _, database := range append(append([]string{}, databases...), "postgres") {
			t.Run(role+"/"+database, func(t *testing.T) {
				dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", role, role, endpoint, database)
				db, err := sql.Open("pgx", dsn)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := db.Close(); err != nil {
						t.Error(err)
					}
				})

				err = db.PingContext(ctx)
				if role == database {
					if err != nil {
						t.Fatalf("connect to own database: %v", err)
					}
					if _, err := db.ExecContext(ctx, "CREATE TABLE migration_check (id bigint PRIMARY KEY)"); err != nil {
						t.Fatalf("create table in own database: %v", err)
					}
					return
				}
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
					t.Fatalf("cross-database connection: got %v, want PostgreSQL SQLSTATE 42501", err)
				}
			})
		}
	}
}
