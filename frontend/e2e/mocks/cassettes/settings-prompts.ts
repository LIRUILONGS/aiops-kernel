import type { ResultOf } from '@graphql-typed-document-node/core';

import type {
    DefaultPromptFragmentFragment,
    SettingsPromptsDocument,
    UserPromptFragmentFragment,
} from '@/graphql/types';

import { PromptType } from '@/graphql/types';

import type { Cassette } from '../cassette.ts';

import { entity, mergeCassettes } from '../cassette.ts';
import { baseQueries, baseRest } from './base.ts';

export const promptTemplate = (type: PromptType): string => `E2E template for ${type}`;

const prompt = (type: PromptType): DefaultPromptFragmentFragment =>
    entity('DefaultPrompt', { template: promptTemplate(type), type, variables: ['Input'] });

const agentPrompt = (system: PromptType) => entity('AgentPrompt', { system: prompt(system) });

const agentPromptPair = (system: PromptType, human: PromptType) =>
    entity('AgentPrompts', { human: prompt(human), system: prompt(system) });

/** No list-nested fence on purpose: that editor defect is tracked separately, and including it
 * here would make the spec assert a known bug. */
export const RICH_PROMPT_TEMPLATE = [
    '# Refiner',
    '',
    'You are re-planning **{{.Target}}** within {{.Scope}}.',
    '',
    '## Rules',
    '',
    '- keep the plan inside the agreed scope',
    '- attach the evidence behind every step',
    '',
    '```bash',
    'kubectl get pods -n {{.Target}} -o wide',
    '<ops_probes>',
    '```',
    '',
    '| Field | Value |',
    '| --- | --- |',
    '| Scope | {{.Scope}} |',
].join('\n');

const richRefinerSystem = entity('DefaultPrompt', {
    template: RICH_PROMPT_TEMPLATE,
    type: PromptType.Refiner,
    variables: ['Target', 'Scope'],
});

const makeSettingsPrompts = (
    refinerSystem: DefaultPromptFragmentFragment = prompt(PromptType.Refiner),
    userDefined: UserPromptFragmentFragment[] = [],
): ResultOf<typeof SettingsPromptsDocument> => ({
    settingsPrompts: entity('PromptsConfig', {
        default: entity('DefaultPrompts', {
            agents: entity('AgentsPrompts', {
                assistant: agentPrompt(PromptType.Assistant),
                changeAnalyst: agentPrompt(PromptType.ChangeAnalyst),
                commander: agentPrompt(PromptType.Commander),
                generator: agentPromptPair(PromptType.Generator, PromptType.SubtasksGenerator),
                logAnalyst: agentPrompt(PromptType.LogAnalyst),
                metricsAnalyst: agentPrompt(PromptType.MetricsAnalyst),
                refiner: entity('AgentPrompts', {
                    human: prompt(PromptType.SubtasksRefiner),
                    system: refinerSystem,
                }),
                reflector: agentPromptPair(PromptType.Reflector, PromptType.QuestionReflector),
                reporter: agentPromptPair(PromptType.Reporter, PromptType.TaskReporter),
                summarizer: agentPrompt(PromptType.Summarizer),
                toolCallFixer: agentPromptPair(PromptType.ToolcallFixer, PromptType.InputToolcallFixer),
                triage: agentPrompt(PromptType.Triage),
            }),
            tools: entity('ToolsPrompts', {
                chooseDockerImage: prompt(PromptType.ImageChooser),
                chooseUserLanguage: prompt(PromptType.LanguageChooser),
                collectToolCallId: prompt(PromptType.ToolCallIdCollector),
                detectToolCallIdPattern: prompt(PromptType.ToolCallIdDetector),
                getExecutionLogs: prompt(PromptType.ExecutionLogs),
                getFlowDescription: prompt(PromptType.FlowDescriptor),
                getFullExecutionContext: prompt(PromptType.FullExecutionContext),
                getShortExecutionContext: prompt(PromptType.ShortExecutionContext),
                getTaskDescription: prompt(PromptType.TaskDescriptor),
            }),
        }),
        userDefined,
    }),
});

const settingsPrompts = makeSettingsPrompts();

export const settingsPromptsCassette = (override: Cassette = {}): Cassette =>
    mergeCassettes(
        {
            queries: {
                ...baseQueries(),
                settingsPrompts: [{ data: settingsPrompts }],
            },
            rest: baseRest(),
        },
        override,
    );

/** The agent key the prompt detail route resolves from its `:promptId` param. */
export const PROMPT_DETAIL_AGENT = 'refiner';

/** The operator's own copy of the refiner system prompt: its presence is what switches the editor
 *  from the create branch to the update branch, and its id is what a save has to carry. */
export const PROMPT_OVERRIDE = {
    id: 'user-prompt-1',
    // Shares no line with the default on purpose: an override that merely appends to it makes
    // "the default came back" indistinguishable from "nothing happened".
    template: ['# Operator override', '', 'This copy replaces the shipped prompt for {{.Target}}.'].join('\n'),
};

const promptOverride = (): UserPromptFragmentFragment =>
    entity('UserPrompt', {
        createdAt: '2026-01-15T09:00:00Z',
        id: PROMPT_OVERRIDE.id,
        template: PROMPT_OVERRIDE.template,
        type: PromptType.Refiner,
        updatedAt: '2026-01-15T09:00:00Z',
    });

/** Raised by a spec's deletePrompt entry so the refetch can answer without the override. */
export const PROMPT_RESET_FLAG = 'prompt-override-deleted';

/** The same agent, but already overridden — the branch where Save updates instead of creating. The
 *  second answer models the row actually being gone, so a reset has a real post-state to observe. */
export const promptOverrideCassette = (override: Cassette = {}): Cassette =>
    mergeCassettes(
        {
            queries: {
                ...baseQueries(),
                settingsPrompts: [
                    { data: makeSettingsPrompts(richRefinerSystem, [promptOverride()]) },
                    { data: makeSettingsPrompts(richRefinerSystem), whenFlag: PROMPT_RESET_FLAG },
                ],
            },
            rest: baseRest(),
        },
        override,
    );

/** Same config, but that agent's system prompt carries RICH_PROMPT_TEMPLATE. */
export const promptDetailCassette = (override: Cassette = {}): Cassette =>
    mergeCassettes(
        {
            queries: {
                ...baseQueries(),
                settingsPrompts: [{ data: makeSettingsPrompts(richRefinerSystem) }],
            },
            rest: baseRest(),
        },
        override,
    );
