-- name: GetOrCreateCapabilityLease :one
-- The lease is keyed by subtask: one execution scope per subtask. A re-plan
-- deletes the subtask rows, and the foreign key cascades the lease away with
-- them. Re-issuing keeps the row (and its audit trail) instead of replacing it.
INSERT INTO capability_leases (
  flow_id, subtask_id, role, mode, targets, tools, commands, shapes, trace_programs, issued_by, expires_at
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (subtask_id) DO UPDATE
SET role = capability_leases.role
RETURNING *;

-- name: GetCapabilityLease :one
SELECT * FROM capability_leases WHERE subtask_id = $1;

-- name: RecordCapabilityDecision :exec
INSERT INTO capability_decisions (
  flow_id, subtask_id, lease_id, role, tool, target, command, verdict, reason
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
);

-- name: ListCapabilityDecisions :many
SELECT * FROM capability_decisions WHERE flow_id = $1 ORDER BY created_at DESC LIMIT $2;
