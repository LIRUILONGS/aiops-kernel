-- +goose Up
ALTER TABLE flows ADD COLUMN capability_mode TEXT NOT NULL DEFAULT 'off'
    CONSTRAINT flows_capability_mode_check CHECK (capability_mode IN ('off', 'shadow', 'enforce'));

CREATE TABLE capability_leases (
    id             BIGSERIAL PRIMARY KEY,
    flow_id        BIGINT NOT NULL REFERENCES flows (id) ON DELETE CASCADE,
    subtask_id     BIGINT NOT NULL UNIQUE REFERENCES subtasks (id) ON DELETE CASCADE,
    role           TEXT NOT NULL,
    mode           TEXT NOT NULL,
    targets        TEXT[] NOT NULL DEFAULT '{}',
    tools          TEXT[] NOT NULL DEFAULT '{}',
    commands       TEXT[] NOT NULL DEFAULT '{}',
    shapes         TEXT[] NOT NULL DEFAULT '{}',
    trace_programs TEXT[] NOT NULL DEFAULT '{}',
    issued_by      TEXT NOT NULL DEFAULT 'policy',
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX capability_leases_flow_idx ON capability_leases (flow_id);

CREATE TABLE capability_decisions (
    id         BIGSERIAL PRIMARY KEY,
    flow_id    BIGINT NOT NULL REFERENCES flows (id) ON DELETE CASCADE,
    subtask_id BIGINT,
    lease_id   BIGINT,
    role       TEXT NOT NULL,
    tool       TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    verdict    TEXT NOT NULL CHECK (verdict IN ('allow', 'deny', 'would_deny')),
    reason     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX capability_decisions_flow_idx ON capability_decisions (flow_id, created_at DESC);

-- +goose Down
DROP TABLE capability_decisions;
DROP TABLE capability_leases;
ALTER TABLE flows DROP CONSTRAINT flows_capability_mode_check;
ALTER TABLE flows DROP COLUMN capability_mode;
