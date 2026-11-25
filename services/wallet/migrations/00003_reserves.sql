-- +goose Up
CREATE TABLE wallet_reserves (
    bet_id text PRIMARY KEY,
    player_id text NOT NULL REFERENCES wallets(player_id),
    amount bigint NOT NULL CHECK (amount > 0),
    result text NOT NULL CHECK (result IN ('reserved', 'insufficient_funds'))
);
-- +goose Down
DROP TABLE wallet_reserves;
