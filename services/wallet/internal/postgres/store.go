package postgres

import (
	"context"
	"errors"
	"github.com/finelai/test-task-sportbook/services/wallet/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	return &Store{p}, nil
}
func (s *Store) Close() { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	p, e := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if e != nil {
		return e
	}
	_, e = p.Up(ctx)
	return e
}
func (s *Store) Ping(ctx context.Context) error {
	var applied bool
	e := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id=1 AND is_applied)").Scan(&applied)
	if e != nil {
		return e
	}
	if !applied {
		return errors.New("migration not applied")
	}
	return nil
}
func (s *Store) Balance(ctx context.Context, player string) (int64, int64, error) {
	var a, r int64
	err := s.pool.QueryRow(ctx, `INSERT INTO wallets(player_id) VALUES ($1) ON CONFLICT (player_id) DO UPDATE SET player_id=EXCLUDED.player_id RETURNING available,reserved`, player).Scan(&a, &r)
	return a, r, err
}
