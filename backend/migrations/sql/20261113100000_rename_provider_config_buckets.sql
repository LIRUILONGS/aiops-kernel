-- +goose Up
-- The commander's block was called primary_agent and the analysts' searcher. The
-- product now reads both names (handleLegacyConfig folds them), so this rewrite is
-- a tidy-up that keeps a config the product writes from carrying a second name
-- for the same bucket. `providers.config` is JSON, not JSONB, hence the casts.
UPDATE providers
SET config = (
	(config::jsonb - 'primary_agent' - 'searcher')
	|| CASE WHEN config::jsonb ? 'primary_agent'
		THEN jsonb_build_object('commander', config::jsonb -> 'primary_agent')
		ELSE '{}'::jsonb END
	|| CASE WHEN config::jsonb ? 'searcher'
		THEN jsonb_build_object('analyst', config::jsonb -> 'searcher')
		ELSE '{}'::jsonb END
)::json
WHERE config::jsonb ? 'primary_agent'
	OR config::jsonb ? 'searcher';

-- +goose Down
UPDATE providers
SET config = (
	(config::jsonb - 'commander' - 'analyst')
	|| CASE WHEN config::jsonb ? 'commander'
		THEN jsonb_build_object('primary_agent', config::jsonb -> 'commander')
		ELSE '{}'::jsonb END
	|| CASE WHEN config::jsonb ? 'analyst'
		THEN jsonb_build_object('searcher', config::jsonb -> 'analyst')
		ELSE '{}'::jsonb END
)::json
WHERE config::jsonb ? 'commander'
	OR config::jsonb ? 'analyst';
