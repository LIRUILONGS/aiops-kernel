-- +goose Up
CREATE TABLE capability_requests (
    id         BIGSERIAL PRIMARY KEY,
    flow_id    BIGINT NOT NULL REFERENCES flows (id) ON DELETE CASCADE,
    subtask_id BIGINT REFERENCES subtasks (id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT '',
    tool       TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    shape      TEXT NOT NULL DEFAULT '',
    trace_program TEXT NOT NULL DEFAULT '',
    reason     TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'pending'
        CONSTRAINT capability_requests_status_check CHECK (status IN ('pending', 'approved', 'denied')),
    decided_by TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX capability_requests_flow_idx ON capability_requests (flow_id, status, created_at DESC);
CREATE INDEX capability_requests_pending_idx ON capability_requests (flow_id, tool, target, command) WHERE status = 'pending';

CREATE TABLE capability_grants (
    id         BIGSERIAL PRIMARY KEY,
    flow_id    BIGINT NOT NULL REFERENCES flows (id) ON DELETE CASCADE,
    subtask_id BIGINT REFERENCES subtasks (id) ON DELETE CASCADE,
    request_id BIGINT REFERENCES capability_requests (id) ON DELETE SET NULL,
    role       TEXT NOT NULL DEFAULT '',
    tool       TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    shape      TEXT NOT NULL DEFAULT '',
    trace_program TEXT NOT NULL DEFAULT '',
    reason     TEXT NOT NULL DEFAULT '',
    issued_by  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX capability_grants_flow_idx ON capability_grants (flow_id, expires_at DESC);

-- +goose Down
DROP TABLE capability_grants;
DROP TABLE capability_requests;
