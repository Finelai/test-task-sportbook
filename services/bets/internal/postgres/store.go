package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/finelai/test-task-sportbook/services/bets/internal/app"
	"github.com/jackc/pgx/v5"

	"github.com/finelai/test-task-sportbook/services/bets/internal/httpapi"
	"github.com/finelai/test-task-sportbook/services/bets/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{p}, nil
}
func (s *Store) Close() { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer func() { _ = db.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
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
	var b httpapi.Bet
	var savedOffer string
	err := s.pool.QueryRow(ctx, `INSERT INTO bets (id,player_id,idempotency_key,offer_id,amount,odds) VALUES ($1,'player-1',$2,$3,$4,$5) ON CONFLICT (player_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key RETURNING id,status,reason,offer_id,amount,odds`, uuid.NewString(), key, offer, amount, odds).Scan(&b.ID, &b.Status, &b.Reason, &savedOffer, &b.Amount, &b.Odds)
	if err != nil {
		return httpapi.Bet{}, err
	}
	if savedOffer != offer || b.Amount != amount || b.Odds != odds {
		return httpapi.Bet{}, httpapi.ErrConflict
	}
	return b, nil
}

func (s *Store) Claim(ctx context.Context, lease time.Duration) (*app.Job, error) {
	var j app.Job
	err := s.pool.QueryRow(ctx, `WITH candidate AS (SELECT id FROM bets WHERE status='pending' AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<=now()) ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE bets b SET lease_token=$1, lease_until=now()+$2*interval '1 microsecond' FROM candidate c WHERE b.id=c.id RETURNING b.id,b.player_id,b.offer_id,b.status,b.reason,b.stage,b.lease_token,b.match_id,b.outcome,b.amount,b.odds,b.confirmed_odds,b.attempts`, uuid.NewString(), lease.Microseconds()).Scan(&j.ID, &j.Player, &j.Offer, &j.Status, &j.Reason, &j.Stage, &j.Token, &j.Match, &j.Outcome, &j.Amount, &j.Odds, &j.ConfirmedOdds, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}
func (s *Store) Save(ctx context.Context, j *app.Job) (bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE bets SET status=$3,reason=$4,stage=$5,match_id=$6,outcome=$7,confirmed_odds=$8,attempts=0,next_attempt_at=now(),lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND lease_until>now() AND status='pending'`, j.ID, j.Token, j.Status, j.Reason, j.Stage, j.Match, j.Outcome, j.ConfirmedOdds)
	return result.RowsAffected() == 1, err
}
func (s *Store) Retry(ctx context.Context, j *app.Job, delay time.Duration) (bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE bets SET attempts=attempts+1,next_attempt_at=now()+$3*interval '1 microsecond',lease_token=NULL,lease_until=NULL WHERE id=$1 AND lease_token=$2 AND lease_until>now() AND status='pending'`, j.ID, j.Token, delay.Microseconds())
	return result.RowsAffected() == 1, err
}

func (s *Store) Get(ctx context.Context, id string) (httpapi.Bet, error) {
	var bet httpapi.Bet
	err := s.pool.QueryRow(ctx, `SELECT id,status,reason,amount,odds FROM bets WHERE id=$1 AND player_id='player-1'`, id).Scan(&bet.ID, &bet.Status, &bet.Reason, &bet.Amount, &bet.Odds)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpapi.Bet{}, httpapi.ErrNotFound
	}
	return bet, err
}
