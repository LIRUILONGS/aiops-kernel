-- +goose Up
CREATE TABLE credentials (
    id                   BIGSERIAL PRIMARY KEY,
    name                 TEXT NOT NULL UNIQUE,
    kind                 TEXT NOT NULL CHECK (kind IN ('ssh_key', 'ssh_password', 'k8s_token', 'db_password')),
    secret               BYTEA NOT NULL,
    host                 TEXT NOT NULL DEFAULT '',
    port                 INTEGER NOT NULL DEFAULT 0,
    username             TEXT NOT NULL DEFAULT '',
    host_key_fingerprint TEXT NOT NULL DEFAULT '',
    rotated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at         TIMESTAMPTZ,
    created_by           BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX credentials_last_used_idx ON credentials (last_used_at DESC NULLS LAST);

-- +goose Down
DROP TABLE credentials;
