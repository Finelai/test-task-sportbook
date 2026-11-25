package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/finelai/test-task-sportbook/services/bets/internal/app"
	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/bets/internal/postgres/db"
	"github.com/finelai/test-task-sportbook/services/bets/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func Open(ctx context.Context, url string) (*Store, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{pool: p, queries: db.New(p)}, nil
}
func (s *Store) Close() { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	database := stdlib.OpenDBFromPool(s.pool)
	defer func() { _ = database.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
func (s *Store) Ping(ctx context.Context) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id=2 AND is_applied)`).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("migration not applied")
	}
	return nil
}
func (s *Store) Submit(ctx context.Context, key, offer string, amount, odds int64) (httpapi.Bet, error) {
	row, err := s.queries.SubmitBet(ctx, db.SubmitBetParams{ID: uuid.NewString(), IdempotencyKey: key, OfferID: offer, Amount: amount, Odds: odds})
	if err != nil {
		return httpapi.Bet{}, err
	}
	if row.OfferID != offer || row.Amount != amount || row.Odds != odds {
		return httpapi.Bet{}, httpapi.ErrConflict
	}
	return httpapi.Bet{ID: row.ID, Status: row.Status, Reason: row.Reason, Amount: row.Amount, Odds: row.Odds}, nil
}
func (s *Store) Claim(ctx context.Context, lease time.Duration) (*app.Job, error) {
	row, err := s.queries.ClaimBet(ctx, db.ClaimBetParams{Token: pgtype.Text{String: uuid.NewString(), Valid: true}, LeaseUs: lease.Microseconds()})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app.Job{ID: row.ID, Player: row.PlayerID, Offer: row.OfferID, Status: row.Status, Reason: row.Reason, Stage: row.Stage, Token: row.Token, Match: row.MatchID, Outcome: row.Outcome, Amount: row.Amount, Odds: row.Odds, ConfirmedOdds: row.ConfirmedOdds, Attempts: int(row.Attempts)}, nil
}
func (s *Store) Save(ctx context.Context, j *app.Job) (bool, error) {
	n, err := s.queries.SaveBet(ctx, db.SaveBetParams{ID: j.ID, Token: pgtype.Text{String: j.Token, Valid: true}, Status: j.Status, Reason: j.Reason, Stage: j.Stage, MatchID: j.Match, Outcome: j.Outcome, ConfirmedOdds: j.ConfirmedOdds})
	return n == 1, err
}
func (s *Store) Retry(ctx context.Context, j *app.Job, delay time.Duration) (bool, error) {
	n, err := s.queries.RetryBet(ctx, db.RetryBetParams{ID: j.ID, Token: pgtype.Text{String: j.Token, Valid: true}, DelayUs: delay.Microseconds()})
	return n == 1, err
}
func (s *Store) Get(ctx context.Context, id string) (httpapi.Bet, error) {
	row, err := s.queries.GetBet(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpapi.Bet{}, httpapi.ErrNotFound
	}
	if err != nil {
		return httpapi.Bet{}, err
	}
	return httpapi.Bet{ID: row.ID, Status: row.Status, Reason: row.Reason, Amount: row.Amount, Odds: row.Odds}, nil
}
