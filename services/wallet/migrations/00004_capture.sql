-- +goose Up
ALTER TABLE wallet_reserves ADD COLUMN state text;
UPDATE wallet_reserves SET state = 'reserved' WHERE result = 'reserved';
ALTER TABLE wallet_reserves ADD CONSTRAINT wallet_reserves_state_check
    CHECK ((result = 'insufficient_funds' AND state IS NULL) OR
           (result = 'reserved' AND state IN ('reserved', 'captured', 'released') AND state IS NOT NULL));
-- +goose Down
ALTER TABLE wallet_reserves DROP CONSTRAINT wallet_reserves_state_check;
ALTER TABLE wallet_reserves DROP COLUMN state;
