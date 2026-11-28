import type { ResultOf } from '@graphql-typed-document-node/core';

import type { CredentialFragmentFragment, CredentialsDocument } from '@/graphql/types';

import { CredentialKind } from '@/graphql/types';

import type { Cassette } from '../cassette.ts';

import { entity, mergeCassettes } from '../cassette.ts';
import { baseQueries, baseRest } from './base.ts';

const ROTATED_AT = '2026-01-15T08:00:00Z';
const LAST_USED_AT = '2026-01-16T09:30:00Z';

const credentialFields = (
    id: string,
    name: string,
    kind: CredentialKind,
    extra: Partial<CredentialFragmentFragment> = {},
): CredentialFragmentFragment => ({
    createdAt: ROTATED_AT,
    host: '10.0.0.5',
    hostKeyFingerprint: 'SHA256:abcdef',
    id,
    kind,
    lastUsedAt: LAST_USED_AT,
    name,
    port: 22,
    rotatedAt: ROTATED_AT,
    username: 'ops',
    ...extra,
});

export const makeCredential = (
    id: string,
    name: string,
    kind: CredentialKind = CredentialKind.SshKey,
    extra: Partial<CredentialFragmentFragment> = {},
): CredentialFragmentFragment => entity('CredentialInfo', credentialFields(id, name, kind, extra));

export const SEED_CREDENTIAL = makeCredential('1', 'E2E lab key');
export const CREATED_CREDENTIAL = makeCredential('2', 'E2E new key', CredentialKind.SshPassword, {
    host: '10.0.0.7',
    lastUsedAt: null,
    port: 2222,
    username: 'root',
});
export const ROTATED_CREDENTIAL = makeCredential('1', 'E2E lab key', CredentialKind.SshKey, {
    rotatedAt: '2026-02-01T10:00:00Z',
});

export const credentialsList = (...items: CredentialFragmentFragment[]): ResultOf<typeof CredentialsDocument> => ({
    credentials: items,
});

export const credentialsCassette = (override: Cassette = {}): Cassette =>
    mergeCassettes(
        {
            queries: {
                ...baseQueries(),
                credentials: [{ data: credentialsList(SEED_CREDENTIAL) }],
            },
            rest: baseRest(),
        },
        override,
    );
