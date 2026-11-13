-- +goose Up
ALTER TABLE flows DROP COLUMN functions;
ALTER TABLE assistants DROP COLUMN functions;

-- +goose Down
ALTER TABLE flows ADD COLUMN functions JSON NOT NULL DEFAULT '{}';
ALTER TABLE assistants ADD COLUMN functions JSON NOT NULL DEFAULT '{}';
