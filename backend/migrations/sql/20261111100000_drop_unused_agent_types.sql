-- +goose Up
-- +goose StatementBegin
-- The penetration-testing roles and the primary agent are gone from the product: AIOps flows
-- run a commander with four analysts, and nothing else. A value cannot be removed from a
-- PostgreSQL enum in place, so the rows that carry the retired values move to commander first,
-- then the type is rebuilt with the surviving twelve.
ALTER TABLE msgchains ALTER COLUMN type DROP DEFAULT;
ALTER TABLE agentlogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE agentlogs ALTER COLUMN executor DROP DEFAULT;
ALTER TABLE vecstorelogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE vecstorelogs ALTER COLUMN executor DROP DEFAULT;
ALTER TABLE searchlogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE searchlogs ALTER COLUMN executor DROP DEFAULT;

UPDATE msgchains SET type = 'commander'
    WHERE type IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE agentlogs SET initiator = 'commander'
    WHERE initiator IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE agentlogs SET executor = 'commander'
    WHERE executor IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE vecstorelogs SET initiator = 'commander'
    WHERE initiator IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE vecstorelogs SET executor = 'commander'
    WHERE executor IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE searchlogs SET initiator = 'commander'
    WHERE initiator IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');
UPDATE searchlogs SET executor = 'commander'
    WHERE executor IN ('primary_agent', 'pentester', 'coder', 'searcher', 'installer', 'memorist', 'adviser', 'enricher');

CREATE TYPE MSGCHAIN_TYPE_NEW AS ENUM (
  'reporter',
  'generator',
  'refiner',
  'reflector',
  'summarizer',
  'tool_call_fixer',
  'assistant',
  'commander',
  'triage',
  'log_analyst',
  'metrics_analyst',
  'change_analyst'
);

ALTER TABLE msgchains
    ALTER COLUMN type TYPE MSGCHAIN_TYPE_NEW USING type::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE agentlogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE vecstorelogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE searchlogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

DROP TYPE MSGCHAIN_TYPE;

ALTER TYPE MSGCHAIN_TYPE_NEW RENAME TO MSGCHAIN_TYPE;

ALTER TABLE msgchains
    ALTER COLUMN type SET NOT NULL,
    ALTER COLUMN type SET DEFAULT 'commander';

ALTER TABLE agentlogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'commander',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'commander';

ALTER TABLE vecstorelogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'commander',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'commander';

ALTER TABLE searchlogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'commander',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'commander';

-- A custom prompt for a role that no longer exists has no reader; the rows go, then the type shrinks.
DELETE FROM prompts WHERE type IN (
  'primary_agent', 'pentester', 'question_pentester', 'coder', 'question_coder', 'installer',
  'question_installer', 'searcher', 'question_searcher', 'memorist', 'question_memorist',
  'adviser', 'question_adviser', 'enricher', 'question_enricher', 'question_execution_monitor',
  'question_task_planner', 'task_assignment_wrapper'
);

CREATE TYPE PROMPT_TYPE_NEW AS ENUM (
  'commander',
  'triage',
  'log_analyst',
  'metrics_analyst',
  'change_analyst',
  'assistant',
  'generator',
  'subtasks_generator',
  'refiner',
  'subtasks_refiner',
  'reporter',
  'task_reporter',
  'reflector',
  'question_reflector',
  'toolcall_fixer',
  'input_toolcall_fixer',
  'summarizer',
  'image_chooser',
  'language_chooser',
  'flow_descriptor',
  'task_descriptor',
  'execution_logs',
  'full_execution_context',
  'short_execution_context',
  'tool_call_id_collector',
  'tool_call_id_detector'
);

ALTER TABLE prompts
    ALTER COLUMN type TYPE PROMPT_TYPE_NEW USING type::text::PROMPT_TYPE_NEW;

DROP TYPE PROMPT_TYPE;

ALTER TYPE PROMPT_TYPE_NEW RENAME TO PROMPT_TYPE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM prompts WHERE type IN ('commander', 'triage', 'log_analyst', 'metrics_analyst', 'change_analyst');

-- Rows that were relabelled on the way up stay commander: which of them used to be a pentester
-- or the primary agent is not recorded anywhere.
ALTER TABLE msgchains ALTER COLUMN type DROP DEFAULT;
ALTER TABLE agentlogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE agentlogs ALTER COLUMN executor DROP DEFAULT;
ALTER TABLE vecstorelogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE vecstorelogs ALTER COLUMN executor DROP DEFAULT;
ALTER TABLE searchlogs ALTER COLUMN initiator DROP DEFAULT;
ALTER TABLE searchlogs ALTER COLUMN executor DROP DEFAULT;

CREATE TYPE MSGCHAIN_TYPE_NEW AS ENUM (
  'primary_agent',
  'reporter',
  'generator',
  'refiner',
  'reflector',
  'enricher',
  'adviser',
  'coder',
  'memorist',
  'searcher',
  'installer',
  'pentester',
  'summarizer',
  'tool_call_fixer',
  'assistant',
  'commander',
  'triage',
  'log_analyst',
  'metrics_analyst',
  'change_analyst'
);

ALTER TABLE msgchains
    ALTER COLUMN type TYPE MSGCHAIN_TYPE_NEW USING type::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE agentlogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE vecstorelogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

ALTER TABLE searchlogs
    ALTER COLUMN initiator TYPE MSGCHAIN_TYPE_NEW USING initiator::text::MSGCHAIN_TYPE_NEW,
    ALTER COLUMN executor TYPE MSGCHAIN_TYPE_NEW USING executor::text::MSGCHAIN_TYPE_NEW;

DROP TYPE MSGCHAIN_TYPE;

ALTER TYPE MSGCHAIN_TYPE_NEW RENAME TO MSGCHAIN_TYPE;

ALTER TABLE msgchains
    ALTER COLUMN type SET NOT NULL,
    ALTER COLUMN type SET DEFAULT 'primary_agent';

ALTER TABLE agentlogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'primary_agent',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'primary_agent';

ALTER TABLE vecstorelogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'primary_agent',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'primary_agent';

ALTER TABLE searchlogs
    ALTER COLUMN initiator SET NOT NULL,
    ALTER COLUMN initiator SET DEFAULT 'primary_agent',
    ALTER COLUMN executor SET NOT NULL,
    ALTER COLUMN executor SET DEFAULT 'primary_agent';

CREATE TYPE PROMPT_TYPE_NEW AS ENUM (
  'primary_agent',
  'assistant',
  'pentester',
  'question_pentester',
  'coder',
  'question_coder',
  'installer',
  'question_installer',
  'searcher',
  'question_searcher',
  'memorist',
  'question_memorist',
  'adviser',
  'question_adviser',
  'generator',
  'subtasks_generator',
  'refiner',
  'subtasks_refiner',
  'reporter',
  'task_reporter',
  'reflector',
  'question_reflector',
  'enricher',
  'question_enricher',
  'toolcall_fixer',
  'input_toolcall_fixer',
  'summarizer',
  'image_chooser',
  'language_chooser',
  'flow_descriptor',
  'task_descriptor',
  'execution_logs',
  'full_execution_context',
  'short_execution_context',
  'tool_call_id_collector',
  'tool_call_id_detector',
  'question_execution_monitor',
  'question_task_planner',
  'task_assignment_wrapper'
);

ALTER TABLE prompts
    ALTER COLUMN type TYPE PROMPT_TYPE_NEW USING type::text::PROMPT_TYPE_NEW;

DROP TYPE PROMPT_TYPE;

ALTER TYPE PROMPT_TYPE_NEW RENAME TO PROMPT_TYPE;
-- +goose StatementEnd
