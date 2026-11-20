-- +goose Up
ALTER TABLE containers ADD COLUMN requested_image TEXT;

-- +goose Down
ALTER TABLE containers DROP COLUMN requested_image;
