-- name: SubmitBet :one
INSERT INTO bets (id, player_id, idempotency_key, offer_id, amount, odds)
VALUES ($1, 'player-1', $2, $3, $4, $5)
ON CONFLICT (player_id, idempotency_key)
DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, status, reason, offer_id, amount, odds;

-- name: GetBet :one
SELECT id, status, reason, amount, odds
FROM bets WHERE id = $1 AND player_id = 'player-1';

-- name: ClaimBet :one
WITH candidate AS (
    SELECT id FROM bets
    WHERE status = 'pending' AND next_attempt_at <= now()
      AND (lease_until IS NULL OR lease_until <= now())
    ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE bets b SET lease_token = sqlc.arg(token),
    lease_until = now() + sqlc.arg(lease_us)::bigint * interval '1 microsecond'
FROM candidate c WHERE b.id = c.id
RETURNING b.id, b.player_id, b.offer_id, b.status, b.reason, b.stage,
    coalesce(b.lease_token, '')::text AS token, b.match_id, b.outcome,
    b.amount, b.odds, b.confirmed_odds, b.attempts;

-- name: SaveBet :execrows
UPDATE bets SET status = sqlc.arg(status), reason = sqlc.arg(reason),
    stage = sqlc.arg(stage), match_id = sqlc.arg(match_id),
    outcome = sqlc.arg(outcome), confirmed_odds = sqlc.arg(confirmed_odds),
    attempts = 0, next_attempt_at = now(), lease_token = NULL, lease_until = NULL
WHERE id = sqlc.arg(id) AND lease_token = sqlc.arg(token)
  AND lease_until > now() AND status = 'pending';

-- name: RetryBet :execrows
UPDATE bets SET attempts = attempts + 1,
    next_attempt_at = now() + sqlc.arg(delay_us)::bigint * interval '1 microsecond',
    lease_token = NULL, lease_until = NULL
WHERE id = sqlc.arg(id) AND lease_token = sqlc.arg(token)
  AND lease_until > now() AND status = 'pending';
