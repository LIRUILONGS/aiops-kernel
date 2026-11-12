-- +goose Up
ALTER TABLE subtasks ADD COLUMN plan_mode TEXT NOT NULL DEFAULT 'diagnosis';

UPDATE subtasks SET plan_mode = 'optimization' WHERE description LIKE '【目标类型：性能优化】%';

UPDATE subtasks SET description = regexp_replace(description, '^【目标类型：[^】]*】[[:space:]]*', '')
WHERE description LIKE '【目标类型：%';

-- +goose Down
ALTER TABLE subtasks DROP COLUMN plan_mode;
