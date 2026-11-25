-- +goose Up
CREATE TABLE wallets (
    player_id text PRIMARY KEY,
    available bigint NOT NULL DEFAULT 0 CHECK (available >= 0),
    reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0)
);
CREATE TABLE wallet_journal (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    player_id text NOT NULL REFERENCES wallets(player_id),
    operation text NOT NULL,
    amount bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE wallet_journal;
DROP TABLE wallets;
