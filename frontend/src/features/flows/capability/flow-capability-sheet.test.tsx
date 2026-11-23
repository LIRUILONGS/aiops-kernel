import { render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const apollo = vi.hoisted(() => ({
    decide: vi.fn(),
    state: { decisions: [] as unknown[], loading: false, pending: [] as unknown[] },
}));

vi.mock('@apollo/client/react', () => ({
    useMutation: () => [apollo.decide],
    useQuery: (document: { definitions?: { name?: { value?: string } }[] }) => {
        if (document.definitions?.[0]?.name?.value === 'flowCapabilityRequests') {
            return { data: { capabilityRequests: apollo.state.pending }, loading: false };
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
        apollo.state = { decisions: [], loading: false, pending: [] };

        renderSheet(CapabilityMode.Off);

        expect(screen.getByText('租约关闭时不写任何记录。')).toBeInTheDocument();
    });

    it('offers a pending approval and sends the decision the operator made', async () => {
        apollo.decide.mockResolvedValue({ data: { decideCapabilityRequest: { id: '4' } } });
        apollo.state = {
            decisions: [],
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

    it('sends a refusal when the operator denies the request', () => {
        apollo.decide.mockResolvedValue({ data: { decideCapabilityRequest: { id: '4' } } });
        apollo.state = {
            decisions: [],
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
