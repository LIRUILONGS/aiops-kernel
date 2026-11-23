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
SELECT * FROM capability_decisions
WHERE flow_id = sqlc.arg(flow_id)
  AND (sqlc.narg(verdict)::text IS NULL OR verdict = sqlc.narg(verdict)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: FindPendingCapabilityRequest :one
SELECT * FROM capability_requests
WHERE flow_id = $1
  AND status = 'pending'
  AND tool = $2
  AND target = $3
  AND command = $4
  AND shape = $5
  AND trace_program = $6
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateCapabilityRequest :one
INSERT INTO capability_requests (
  flow_id, subtask_id, role, tool, target, command, shape, trace_program, reason
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: ListCapabilityRequests :many
SELECT * FROM capability_requests
WHERE flow_id = sqlc.arg(flow_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: DecideCapabilityRequest :one
UPDATE capability_requests
SET status = $2,
    decided_by = $3,
    decided_at = now()
WHERE id = $1
  AND status = 'pending'
RETURNING *;

-- name: CreateCapabilityGrant :one
INSERT INTO capability_grants (
  flow_id, subtask_id, request_id, role, tool, target, command, shape, trace_program, reason, issued_by, expires_at
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: ListActiveCapabilityGrants :many
SELECT * FROM capability_grants
WHERE flow_id = $1
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: GetCapabilityRequest :one
SELECT * FROM capability_requests WHERE id = $1;
