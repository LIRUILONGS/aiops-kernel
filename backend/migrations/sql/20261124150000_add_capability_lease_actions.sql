-- +goose Up
ALTER TABLE capability_leases ADD COLUMN actions TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE capability_leases DROP COLUMN actions;
