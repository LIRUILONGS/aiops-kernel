-- name: GetFlowTemplate :one
SELECT * FROM flow_templates
WHERE id = $1 AND user_id = $2 LIMIT 1;

-- name: GetFlowTemplatesByUserID :many
SELECT * FROM flow_templates
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateFlowTemplate :one
INSERT INTO flow_templates (
  user_id,
  title,
  text
) VALUES (
  $1,
  $2,
  $3
)
RETURNING *;

-- name: UpdateFlowTemplate :one
UPDATE flow_templates
SET 
  title = $3,
  text = $4
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteFlowTemplate :exec
DELETE FROM flow_templates
WHERE id = $1 AND user_id = $2;

-- name: SeedFlowTemplates :execrows
WITH claimed AS (
  INSERT INTO flow_template_seeds (user_id)
  VALUES (sqlc.arg(user_id))
  ON CONFLICT DO NOTHING
  RETURNING user_id
)
INSERT INTO flow_templates (user_id, title, text)
SELECT sqlc.arg(user_id), preset.title, preset.text
FROM unnest(sqlc.arg(titles)::text[], sqlc.arg(texts)::text[]) AS preset(title, text)
WHERE EXISTS (SELECT 1 FROM claimed);
