package postgres

import (
	"context"
	"errors"
	"github.com/finelai/test-task-sportbook/services/wallet/internal/app"
	"github.com/finelai/test-task-sportbook/services/wallet/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"math"
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
	defer func() { _ = db.Close() }()
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

func (s *Store) Credit(ctx context.Context, player, key string, amount int64) (app.CreditResult, error) {
	var result app.CreditResult
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO wallets(player_id) VALUES ($1) ON CONFLICT DO NOTHING`, player); err != nil {
		return result, err
	}
	var available, reserved int64
	if err = tx.QueryRow(ctx, `SELECT available,reserved FROM wallets WHERE player_id=$1 FOR UPDATE`, player).Scan(&available, &reserved); err != nil {
		return result, err
	}
	var previous int64
	err = tx.QueryRow(ctx, `SELECT amount,operation_id,available,reserved FROM wallet_credits WHERE player_id=$1 AND idempotency_key=$2`, player, key).Scan(&previous, &result.OperationID, &result.Available, &result.Reserved)
	if err == nil {
		if previous != amount {
			return app.CreditResult{}, app.ErrConflict
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if amount <= 0 || reserved > math.MaxInt64-available || amount > math.MaxInt64-available-reserved {
		return result, app.ErrInvalidAmount
	}
	result = app.CreditResult{OperationID: uuid.NewString(), Available: available + amount, Reserved: reserved}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET available=$2 WHERE player_id=$1`, player, result.Available); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_journal(player_id,operation,amount) VALUES ($1,'credit',$2)`, player, amount); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_credits(player_id,idempotency_key,amount,operation_id,available,reserved) VALUES ($1,$2,$3,$4,$5,$6)`, player, key, amount, result.OperationID, result.Available, result.Reserved); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return app.CreditResult{}, err
	}
	return result, nil
}

func (s *Store) Reserve(ctx context.Context, bet, player string, amount int64) (app.ReserveResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A bet ID is globally unique, including when competing players use it.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, bet); err != nil {
		return 0, err
	}
	var previousPlayer, previousResult string
	var previousAmount int64
	err = tx.QueryRow(ctx, `SELECT player_id,amount,result FROM wallet_reserves WHERE bet_id=$1`, bet).Scan(&previousPlayer, &previousAmount, &previousResult)
	if err == nil {
		if previousPlayer != player || previousAmount != amount {
			return app.ParameterConflict, nil
		}
		if previousResult == "reserved" {
			return app.Reserved, nil
		}
		return app.InsufficientFunds, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallets(player_id) VALUES ($1) ON CONFLICT DO NOTHING`, player); err != nil {
		return 0, err
	}
	var available, reserved int64
	if err = tx.QueryRow(ctx, `SELECT available,reserved FROM wallets WHERE player_id=$1 FOR UPDATE`, player).Scan(&available, &reserved); err != nil {
		return 0, err
	}
	result := app.InsufficientFunds
	stored := "insufficient_funds"
	if available >= amount {
		if amount <= 0 || reserved > math.MaxInt64-amount {
			return 0, errors.New("invalid wallet balance")
		}
		if _, err = tx.Exec(ctx, `UPDATE wallets SET available=available-$2,reserved=reserved+$2 WHERE player_id=$1`, player, amount); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO wallet_journal(player_id,operation,amount) VALUES ($1,'reserve',$2)`, player, amount); err != nil {
			return 0, err
		}
		result = app.Reserved
		stored = "reserved"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_reserves(bet_id,player_id,amount,result,state) VALUES ($1,$2,$3,$4,CASE WHEN $4='reserved' THEN 'reserved' ELSE NULL END)`, bet, player, amount, stored); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return result, nil
}

// Capture locks the bet, then the wallet, then the reserve row. Release must use the same order.
func (s *Store) Capture(ctx context.Context, bet, player string) (app.CaptureResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, bet); err != nil {
		return 0, err
	}
	var owner, result string
	err = tx.QueryRow(ctx, `SELECT player_id,result FROM wallet_reserves WHERE bet_id=$1`, bet).Scan(&owner, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrReserveNotFound
	}
	if err != nil {
		return 0, err
	}
	if owner != player {
		return app.CaptureParameterConflict, nil
	}
	if result != "reserved" {
		return 0, app.ErrReserveNotFound
	}
	var reserved int64
	if err = tx.QueryRow(ctx, `SELECT reserved FROM wallets WHERE player_id=$1 FOR UPDATE`, owner).Scan(&reserved); err != nil {
		return 0, err
	}
	var amount int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT amount,state FROM wallet_reserves WHERE bet_id=$1 FOR UPDATE`, bet).Scan(&amount, &state); err != nil {
		return 0, err
	}
	switch state {
	case "captured":
		return app.Captured, nil
	case "released":
		return app.AlreadyReleased, nil
	case "reserved":
	default:
		return 0, errors.New("invalid reserve state")
	}
	if reserved < amount {
		return 0, errors.New("invalid reserved balance")
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET reserved=reserved-$2 WHERE player_id=$1`, owner, amount); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE wallet_reserves SET state='captured' WHERE bet_id=$1`, bet); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_journal(player_id,operation,amount) VALUES ($1,'capture',$2)`, owner, amount); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return app.Captured, nil
}

// Release locks the bet, wallet and reserve row in the same order as Capture.
func (s *Store) Release(ctx context.Context, bet, player string) (app.ReleaseResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, bet); err != nil {
		return 0, err
	}
	var owner, result string
	err = tx.QueryRow(ctx, `SELECT player_id,result FROM wallet_reserves WHERE bet_id=$1`, bet).Scan(&owner, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrReserveNotFound
	}
	if err != nil {
		return 0, err
	}
	if owner != player {
		return app.ReleaseParameterConflict, nil
	}
	if result != "reserved" {
		return 0, app.ErrReserveNotFound
	}
	var available, reserved int64
	if err = tx.QueryRow(ctx, `SELECT available,reserved FROM wallets WHERE player_id=$1 FOR UPDATE`, owner).Scan(&available, &reserved); err != nil {
		return 0, err
	}
	var amount int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT amount,state FROM wallet_reserves WHERE bet_id=$1 FOR UPDATE`, bet).Scan(&amount, &state); err != nil {
		return 0, err
	}
	switch state {
	case "released":
		return app.Released, nil
	case "captured":
		return app.AlreadyCaptured, nil
	case "reserved":
	default:
		return 0, errors.New("invalid reserve state")
	}
	if reserved < amount || available > math.MaxInt64-amount {
		return 0, errors.New("invalid wallet balance")
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET reserved=reserved-$2,available=available+$2 WHERE player_id=$1`, owner, amount); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE wallet_reserves SET state='released' WHERE bet_id=$1`, bet); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_journal(player_id,operation,amount) VALUES ($1,'release',$2)`, owner, amount); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return app.Released, nil
}
