import type { ResultOf } from '@graphql-typed-document-node/core';

import type {
    FlowsDocument,
    FlowTemplatePresetsDocument,
    FlowTemplatesDocument,
    ProvidersDocument,
    ResourcesDocument,
    SettingsDocument,
    SettingsUserDocument,
    VersionInfoDocument,
} from '@/graphql/types';

import { UpdateState } from '@/graphql/types';

import type { Cassette } from '../cassette.ts';

import { authenticatedInfoEntry } from '../../fixtures/auth.ts';
import { entity } from '../cassette.ts';

const flows: ResultOf<typeof FlowsDocument> = { flows: [] };

const flowTemplates: ResultOf<typeof FlowTemplatesDocument> = { flowTemplates: [] };

export const PRESET_TITLE = 'E2E 内置模板';

// The new-flow composer reads the presets on every route that can start a flow,
// so a cassette without this entry leaves the query unresolved and the page empty.
export const flowTemplatePresets: ResultOf<typeof FlowTemplatePresetsDocument> = {
    flowTemplatePresets: [
        entity('FlowTemplatePreset', {
            domain: '告警诊断',
            text: '用 ops_system 取负载与 Top 进程，再用 ops_logs 取同一时间窗的错误原文。',
            title: PRESET_TITLE,
        }),
    ],
};

// After a websocket reconnect the app re-requests resources on the wire with {recursive:true};
// removing this entry red-lights the reconnect spec on the 501 gate.
const resources: ResultOf<typeof ResourcesDocument> = { resources: [] };

const providers: ResultOf<typeof ProvidersDocument> = { providers: [] };

const versionInfo: ResultOf<typeof VersionInfoDocument> = {
    versionInfo: entity('VersionInfo', {
        build: 'e2e00000',
        checkedAt: null,
        current: 'e2e',
        failedAt: null,
        latest: null,
        state: UpdateState.Disabled,
        strategy: 'preview',
    }),
};

const settings: ResultOf<typeof SettingsDocument> = {
    settings: entity('Settings', {
        askUser: false,
        debug: false,
        dockerInside: false,
        isDevelopMode: false,
        version: 'e2e',
    }),
};

export const SIGNED_IN_USER_ID = '1';

const settingsUser: ResultOf<typeof SettingsUserDocument> = {
    settingsUser: entity('UserPreferences', {
        favoriteFlows: [],
        id: SIGNED_IN_USER_ID,
    }),
};

export const baseQueries = (): NonNullable<Cassette['queries']> => ({
    flows: [{ data: flows }],
    flowTemplatePresets: [{ data: flowTemplatePresets }],
    flowTemplates: [{ data: flowTemplates }],
    providers: [{ data: providers }],
    resources: [{ data: resources }],
    settings: [{ data: settings }],
    settingsUser: [{ data: settingsUser }],
    versionInfo: [{ data: versionInfo }],
});

export const baseRest = (): NonNullable<Cassette['rest']> => ({
    'GET /api/v1/info': [authenticatedInfoEntry()],
    'GET /api/v1/resources/': [{ body: { data: { items: [] }, status: 'success' } }],
});
