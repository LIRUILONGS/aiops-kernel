import type { ResultOf } from '@graphql-typed-document-node/core';

import type {
    FlowTemplateDocument,
    FlowTemplateFragmentFragment,
    FlowTemplatePresetsDocument,
    FlowTemplatesDocument,
} from '@/graphql/types';

import type { Cassette } from '../cassette.ts';

import { entity, mergeCassettes } from '../cassette.ts';
import { baseQueries, baseRest } from './base.ts';

const T = '2026-01-15T11:30:00Z';

export const makeTemplate = (id: string, title: string, text: string): FlowTemplateFragmentFragment =>
    entity('FlowTemplate', {
        createdAt: T,
        id,
        text,
        title,
        updatedAt: T,
        userId: '1',
    });

export const TEMPLATE_SEED = makeTemplate('11', 'E2E Seed Template', 'Scan the target and report findings');

const flowTemplates: ResultOf<typeof FlowTemplatesDocument> = { flowTemplates: [TEMPLATE_SEED] };

const PRESET_TITLE = 'E2E 内置模板';

const flowTemplatePresets: ResultOf<typeof FlowTemplatePresetsDocument> = {
    flowTemplatePresets: [
        entity('FlowTemplatePreset', {
            domain: '告警诊断',
            text: '用 ops_system 取负载与 Top 进程，再用 ops_logs 取同一时间窗的错误原文。',
            title: PRESET_TITLE,
        }),
    ],
};

/** No list-nested fence on purpose: that editor defect is tracked separately. */
export const RICH_TEMPLATE_TEXT = [
    '# Recon',
    '',
    'Target: **{{TARGET}}** (scope {{SCOPE}}).',
    '',
    '## Steps',
    '',
    '- enumerate services',
    '- capture evidence for each finding',
    '',
    '```bash',
    'nmap -sV {{TARGET}}',
    '```',
    '',
    '| Field | Value |',
    '| --- | --- |',
    '| Scope | {{SCOPE}} |',
    '',
].join('\n');

export const TEMPLATE_DETAIL = makeTemplate('11', 'E2E Seed Template', RICH_TEMPLATE_TEXT);

const flowTemplate: ResultOf<typeof FlowTemplateDocument> = { flowTemplate: TEMPLATE_DETAIL };

export const templatesCassette = (override: Cassette = {}): Cassette =>
    mergeCassettes(
        {
            queries: {
                ...baseQueries(),
                flowTemplatePresets: [{ data: flowTemplatePresets }],
                flowTemplates: [{ data: flowTemplates }],
            },
            rest: baseRest(),
        },
        override,
    );

/** Adds the single-template query the detail route issues for TEMPLATE_DETAIL. */
export const templateDetailCassette = (override: Cassette = {}): Cassette =>
    templatesCassette(
        mergeCassettes(
            {
                queries: {
                    flowTemplate: [{ data: flowTemplate, variables: { templateId: TEMPLATE_DETAIL.id } }],
                },
            },
            override,
        ),
    );
