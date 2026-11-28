-- +goose Up
-- +goose StatementBegin
-- Credentials hold secrets that open production hosts, so they get their own area instead of riding
-- on `resources.*`: holding `resources.view` means "may read a shared file", not "may enumerate the
-- fleet's keys". Only role 1 is granted here. A deployment that wants analysts to read the inventory
-- grants `credentials.view` to role 2 explicitly; nothing is granted by default.
INSERT INTO privileges (role_id, name) VALUES
  (1, 'credentials.admin'),
  (1, 'credentials.view'),
  (1, 'credentials.manage'),
  (1, 'credentials.subscribe')
  ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM privileges WHERE name IN (
  'credentials.admin',
  'credentials.view',
  'credentials.manage',
  'credentials.subscribe'
);
-- +goose StatementEnd
