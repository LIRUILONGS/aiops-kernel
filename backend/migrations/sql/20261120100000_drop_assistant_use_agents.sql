-- +goose Up
ALTER TABLE assistants DROP COLUMN use_agents;

-- +goose Down
ALTER TABLE assistants ADD COLUMN use_agents BOOLEAN NOT NULL DEFAULT FALSE;
