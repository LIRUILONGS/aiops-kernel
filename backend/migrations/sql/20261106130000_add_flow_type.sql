-- +goose Up
-- +goose StatementBegin
-- The flow's own intent. `auto` keeps the previous behaviour, where the planner
-- decides per plan entry whether a subtask is an incident or a performance task.
ALTER TABLE flows ADD COLUMN flow_type TEXT NOT NULL DEFAULT 'auto';

ALTER TABLE flows ADD CONSTRAINT flows_flow_type_check
    CHECK (flow_type IN ('auto', 'diagnosis', 'optimization'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE flows DROP CONSTRAINT flows_flow_type_check;

ALTER TABLE flows DROP COLUMN flow_type;
-- +goose StatementEnd
