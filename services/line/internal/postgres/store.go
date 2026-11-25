package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/finelai/test-task-sportbook/services/line/internal/app"
	"github.com/finelai/test-task-sportbook/services/line/internal/postgres/db"
	"github.com/finelai/test-task-sportbook/services/line/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{pool}, nil
}
func (s *Store) Close() { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
func (s *Store) Ready(ctx context.Context) error {
	var n int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM line_state WHERE id=1 AND EXISTS (SELECT 1 FROM goose_db_version WHERE version_id=1 AND is_applied)").Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("migration not applied")
	}
	return nil
}
func (s *Store) Read(ctx context.Context) (app.State, error) {
	b, err := db.New(s.pool).ReadState(ctx)
	var st app.State
	if err == nil {
		err = json.Unmarshal(b, &st)
	}
	return st, err
}
func (s *Store) Change(ctx context.Context, fn func(*app.State) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	b, err := queries.LockState(ctx)
	if err != nil {
		return err
	}
	var st app.State
	if err = json.Unmarshal(b, &st); err != nil {
		return err
	}
	if err = fn(&st); err != nil {
		return err
	}
	b, err = json.Marshal(st)
	if err != nil {
		return err
	}
	if err = queries.SaveState(ctx, b); err != nil {
		return fmt.Errorf("save line: %w", err)
	}
	return tx.Commit(ctx)
}
