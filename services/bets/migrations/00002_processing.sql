-- +goose Up
ALTER TABLE bets ADD COLUMN stage text NOT NULL DEFAULT 'confirm_offer';
ALTER TABLE bets ADD COLUMN match_id text NOT NULL DEFAULT '';
ALTER TABLE bets ADD COLUMN outcome text NOT NULL DEFAULT '';
ALTER TABLE bets ADD COLUMN confirmed_odds bigint NOT NULL DEFAULT 0;
ALTER TABLE bets ADD COLUMN lease_token text;
ALTER TABLE bets ADD COLUMN lease_until timestamptz;
ALTER TABLE bets ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE bets ADD COLUMN attempts integer NOT NULL DEFAULT 0;
CREATE INDEX bets_pending ON bets(next_attempt_at) WHERE status = 'pending';
-- +goose Down
DROP INDEX bets_pending;
ALTER TABLE bets DROP COLUMN stage, DROP COLUMN match_id, DROP COLUMN outcome, DROP COLUMN confirmed_odds, DROP COLUMN lease_token, DROP COLUMN lease_until, DROP COLUMN next_attempt_at, DROP COLUMN attempts;
