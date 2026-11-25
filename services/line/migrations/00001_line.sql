-- +goose Up
CREATE TABLE IF NOT EXISTS line_state (id integer PRIMARY KEY CHECK (id = 1), state jsonb NOT NULL);
INSERT INTO line_state (id, state) VALUES (1, '{}') ON CONFLICT DO NOTHING;
-- +goose Down
DROP TABLE line_state;
