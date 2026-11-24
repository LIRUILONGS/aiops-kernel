import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

const apollo = vi.hoisted(() => ({
    decide: vi.fn(),
    state: { decisions: [] as unknown[], leases: [] as unknown[], loading: false, pending: [] as unknown[] },
}));

vi.mock('@apollo/client/react', () => ({
    useMutation: () => [apollo.decide],
    useQuery: (document: { definitions?: { name?: { value?: string } }[] }) => {
        if (document.definitions?.[0]?.name?.value === 'flowCapabilityRequests') {
            return { data: { capabilityRequests: apollo.state.pending }, loading: false };
        }

        if (document.definitions?.[0]?.name?.value === 'flowCapabilityLeases') {
            return { data: { capabilityLeases: apollo.state.leases }, loading: false };
        }

        return { data: { capabilityDecisions: apollo.state.decisions }, loading: apollo.state.loading };
    },
}));

vi.mock('@/providers/flow-provider', () => ({
    useFlow: () => ({ flowId: '7', flowStatus: 'running' }),
}));

import { CapabilityMode, CapabilityRequestStatus, CapabilityVerdict } from '@/graphql/types';

import FlowCapabilitySheet from './flow-capability-sheet';

const renderSheet = (mode: CapabilityMode) =>
    render(
        <FlowCapabilitySheet
            mode={mode}
            onOpenChange={vi.fn()}
            open
        />,
    );

describe('FlowCapabilitySheet', () => {
    it('shows what enforcement would have stopped, with the reason the guard gave', () => {
        apollo.state = {
            decisions: [
                {
                    command: 'listen_ports',
                    createdAt: '2026-01-15T11:30:00Z',
                    flowId: '7',
                    id: '2',
                    reason: '目标主机 staging 不在本子任务租约内',
                    role: 'log_analyst',
                    subtaskId: '12',
                    target: 'staging',
                    tool: 'ops_exec',
                    verdict: CapabilityVerdict.WouldDeny,
                },
            ],
            leases: [],
            loading: false,
            pending: [],
        };

        renderSheet(CapabilityMode.Shadow);

        expect(screen.getByText('ops_exec · listen_ports')).toBeInTheDocument();
        expect(screen.getByText('目标主机 staging 不在本子任务租约内')).toBeInTheDocument();
        expect(screen.getByText('staging')).toBeInTheDocument();
        expect(within(screen.getByRole('listitem')).getByText('将被拦截')).toBeInTheDocument();
    });

    it('says an empty log is expected rather than broken when the lease is off', () => {
        apollo.state = { decisions: [], leases: [], loading: false, pending: [] };

        renderSheet(CapabilityMode.Off);

        expect(screen.getByText('租约关闭时不写任何记录。')).toBeInTheDocument();
    });

    it('offers a pending approval and sends the decision the operator made', async () => {
        apollo.decide.mockResolvedValue({ data: { decideCapabilityRequest: { id: '4' } } });
        apollo.state = {
            decisions: [],
            leases: [],
            loading: false,
            pending: [
                {
                    command: 'unit_logs',
                    createdAt: '2026-01-15T11:30:00Z',
                    flowId: '7',
                    id: '4',
                    reason: '目标主机 prod 不在这个子任务的租约里',
                    role: 'log_analyst',
                    shape: 'probe',
                    status: CapabilityRequestStatus.Pending,
                    subtaskId: '12',
                    target: 'prod',
                    tool: 'ops_exec',
                    traceProgram: '',
                },
            ],
        };

        renderSheet(CapabilityMode.Enforce);

        expect(screen.getByText('待批准（1）')).toBeInTheDocument();

        screen.getByRole('button', { name: '批准 4 小时' }).click();

        expect(apollo.decide).toHaveBeenCalledWith({
            refetchQueries: expect.any(Array),
            variables: { approve: true, requestId: '4' },
        });
    });

    it('shows what a subtask was issued and how much of its budget it spent', async () => {
        const expiresAt = new Date(Date.now() + 90 * 60_000).toISOString();

        apollo.state = {
            decisions: [],
            leases: [
                {
                    actions: [],
                    budgets: [{ limit: 60, tool: 'ops_exec' }],
                    commands: ['listen_ports', 'unit_logs'],
                    createdAt: '2026-01-15T11:00:00Z',
                    expiresAt,
                    flowId: '7',
                    id: '2',
                    issuedBy: 'policy',
                    planMode: 'diagnosis',
                    role: 'log_analyst',
                    shapes: ['probe'],
                    subtaskId: '12',
                    targets: ['staging'],
                    tools: ['ops_exec', 'ops_logs'],
                    tracePrograms: [],
                    used: [{ tool: 'ops_exec', used: 12 }],
                },
            ],
            loading: false,
            pending: [],
        };

        renderSheet(CapabilityMode.Enforce);

        await userEvent.click(screen.getByRole('tab', { name: '租约（1）' }));

        expect(screen.getByText(/子任务 #12/)).toBeInTheDocument();
        expect(screen.getByText('diagnosis')).toBeInTheDocument();
        expect(screen.getByText('2 小时后到期')).toBeInTheDocument();

        await userEvent.click(screen.getByRole('button', { name: /子任务 #12/ }));

        expect(screen.getByText('主机 staging · 工具 2 个 · 形状 probe')).toBeInTheDocument();
        expect(screen.getByText('listen_ports、unit_logs')).toBeInTheDocument();
        expect(screen.getByText('12 / 60')).toBeInTheDocument();
    });

    it('says a flow with the switch off issued no lease rather than that it lost them', async () => {
        apollo.state = { decisions: [], leases: [], loading: false, pending: [] };

        renderSheet(CapabilityMode.Off);

        await userEvent.click(screen.getByRole('tab', { name: '租约（0）' }));

        expect(screen.getByText('还没有签发租约')).toBeInTheDocument();
    });

    it('sends a refusal when the operator denies the request', () => {
        apollo.decide.mockResolvedValue({ data: { decideCapabilityRequest: { id: '4' } } });
        apollo.state = {
            decisions: [],
            leases: [],
            loading: false,
            pending: [
                {
                    command: 'unit_logs',
                    createdAt: '2026-01-15T11:30:00Z',
                    flowId: '7',
                    id: '4',
                    reason: '目标主机 prod 不在这个子任务的租约里',
                    role: 'log_analyst',
                    shape: 'probe',
                    status: CapabilityRequestStatus.Pending,
                    subtaskId: '12',
                    target: 'prod',
                    tool: 'ops_exec',
                    traceProgram: '',
                },
            ],
        };

        renderSheet(CapabilityMode.Enforce);

        screen.getByRole('button', { name: '拒绝' }).click();

        expect(apollo.decide).toHaveBeenCalledWith({
            refetchQueries: expect.any(Array),
            variables: { approve: false, requestId: '4' },
        });
    });
});
