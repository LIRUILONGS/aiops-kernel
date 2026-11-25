import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { SidebarProvider } from '@/components/ui/sidebar';

const queryResult = vi.hoisted(() => ({
    current: { data: undefined, error: undefined, loading: false, refetch: vi.fn() } as Record<string, unknown>,
}));

const { createCredential, deleteCredential, rotateCredential } = vi.hoisted(() => ({
    createCredential: vi.fn(async () => ({ data: {} })),
    deleteCredential: vi.fn(async () => ({ data: {} })),
    rotateCredential: vi.fn(async () => ({ data: {} })),
}));

vi.mock('@apollo/client/react', () => ({
    useMutation: (document: unknown) => {
        const name =
            (document as { definitions?: { name?: { value?: string } }[] })?.definitions?.[0]?.name?.value ?? '';

        if (name === 'rotateCredential') {
            return [rotateCredential, { loading: false }];
        }

        if (name === 'deleteCredential') {
            return [deleteCredential, { loading: false }];
        }

        return [createCredential, { loading: false }];
    },
    useQuery: () => queryResult.current,
    useSubscription: () => ({ data: undefined, loading: false }),
}));

import SettingsCredentials from './settings-credentials';

const renderPage = () =>
    render(
        <MemoryRouter>
            <SidebarProvider>
                <SettingsCredentials />
            </SidebarProvider>
        </MemoryRouter>,
    );

const credential = (overrides: Record<string, unknown> = {}) => ({
    __typename: 'CredentialInfo',
    createdAt: '2026-11-01T07:00:00Z',
    host: '10.0.0.5',
    hostKeyFingerprint: 'SHA256:abc',
    id: '3',
    kind: 'ssh_password',
    lastUsedAt: null,
    name: 'lab-root',
    port: 22,
    rotatedAt: '2026-11-20T07:00:00Z',
    username: 'root',
    ...overrides,
});

beforeEach(() => {
    queryResult.current = { data: undefined, error: undefined, loading: false, refetch: vi.fn() };
    vi.clearAllMocks();
});

describe('SettingsCredentials list', () => {
    it('shows the stored metadata and says a credential was never used', () => {
        queryResult.current = {
            data: { credentials: [credential()] },
            error: undefined,
            loading: false,
            refetch: vi.fn(),
        };

        renderPage();

        expect(screen.getByText('lab-root')).toBeInTheDocument();
        expect(screen.getByText('SSH 密码')).toBeInTheDocument();
        expect(screen.getByText('10.0.0.5:22')).toBeInTheDocument();
        expect(screen.getByText('从未使用')).toBeInTheDocument();
    });

    it('marks the kinds no tool reads yet', () => {
        queryResult.current = {
            data: { credentials: [credential({ host: '', kind: 'k8s_token', name: 'cluster', port: 0 })] },
            error: undefined,
            loading: false,
            refetch: vi.fn(),
        };

        renderPage();

        expect(screen.getByText('K8s 令牌')).toBeInTheDocument();
        expect(screen.getByText('仅保存，暂无工具引用')).toBeInTheDocument();
    });

    it('offers the create action when nothing is stored', () => {
        queryResult.current = { data: { credentials: [] }, error: undefined, loading: false, refetch: vi.fn() };

        renderPage();

        expect(screen.getByText('还没有凭据')).toBeInTheDocument();
        expect(screen.getAllByRole('button', { name: '新建凭据' }).length).toBeGreaterThan(0);
    });

    it('surfaces the master-key refusal instead of an empty list', () => {
        const refetch = vi.fn();
        queryResult.current = {
            data: undefined,
            error: new Error('no credential master key is configured'),
            loading: false,
            refetch,
        };

        renderPage();

        expect(screen.getByText('凭据加载失败')).toBeInTheDocument();
        expect(screen.getByText('no credential master key is configured')).toBeInTheDocument();
    });
});

describe('SettingsCredentials create', () => {
    it('refuses to submit without a name and a secret', async () => {
        const user = userEvent.setup();

        queryResult.current = { data: { credentials: [] }, error: undefined, loading: false, refetch: vi.fn() };

        renderPage();

        await user.click(screen.getAllByRole('button', { name: '新建凭据' })[0] as HTMLElement);
        await user.click(screen.getByRole('button', { name: '保存' }));

        expect(createCredential).not.toHaveBeenCalled();
    });

    it('sends the entered fields and the secret the operator typed', async () => {
        const user = userEvent.setup();

        queryResult.current = { data: { credentials: [] }, error: undefined, loading: false, refetch: vi.fn() };

        renderPage();

        await user.click(screen.getAllByRole('button', { name: '新建凭据' })[0] as HTMLElement);
        await user.type(screen.getByLabelText('名称'), 'lab-root');
        await user.type(screen.getByLabelText('密钥'), 'hunter2');
        await user.click(screen.getByRole('button', { name: '保存' }));

        await waitFor(() => expect(createCredential).toHaveBeenCalledTimes(1));

        const call = createCredential.mock.calls[0] as unknown as [Record<string, unknown>];
        const input = (call[0] as { variables: { input: Record<string, unknown> } }).variables.input;

        expect(input).toMatchObject({
            host: null,
            kind: 'ssh_password',
            name: 'lab-root',
            port: null,
            secret: 'hunter2',
            username: null,
        });
    });
});
