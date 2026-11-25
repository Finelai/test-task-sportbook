-- +goose Up
CREATE TABLE wallet_credits (
    player_id text NOT NULL REFERENCES wallets(player_id),
    idempotency_key text NOT NULL,
    amount bigint NOT NULL CHECK (amount > 0),
    operation_id text NOT NULL UNIQUE,
    available bigint NOT NULL,
    reserved bigint NOT NULL,
    PRIMARY KEY (player_id, idempotency_key)
);
-- +goose Down
DROP TABLE wallet_credits;
