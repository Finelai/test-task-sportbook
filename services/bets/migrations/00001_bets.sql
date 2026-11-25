-- +goose Up
CREATE TABLE bets (
  id text PRIMARY KEY,
  player_id text NOT NULL,
  idempotency_key text NOT NULL,
  offer_id text NOT NULL,
  amount bigint NOT NULL CHECK (amount > 0),
  odds bigint NOT NULL CHECK (odds > 0),
  status text NOT NULL DEFAULT 'pending',
  reason text NOT NULL DEFAULT '',
  UNIQUE (player_id, idempotency_key)
);

-- +goose Down
DROP TABLE bets;
