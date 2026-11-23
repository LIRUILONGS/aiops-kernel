import { render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const query = vi.hoisted(() => ({
    state: { decisions: [] as unknown[], loading: false },
}));

vi.mock('@apollo/client/react', () => ({
    useQuery: () => ({
        data: { capabilityDecisions: query.state.decisions },
        loading: query.state.loading,
    }),
}));

vi.mock('@/providers/flow-provider', () => ({
    useFlow: () => ({ flowId: '7', flowStatus: 'running' }),
}));

import { CapabilityMode, CapabilityVerdict } from '@/graphql/types';

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
        query.state = {
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
        };

        renderSheet(CapabilityMode.Shadow);

        expect(screen.getByText('ops_exec · listen_ports')).toBeInTheDocument();
        expect(screen.getByText('目标主机 staging 不在本子任务租约内')).toBeInTheDocument();
        expect(screen.getByText('staging')).toBeInTheDocument();
        expect(within(screen.getByRole('listitem')).getByText('将被拦截')).toBeInTheDocument();
    });

    it('says an empty log is expected rather than broken when the lease is off', () => {
        query.state = { decisions: [], loading: false };

        renderSheet(CapabilityMode.Off);

        expect(screen.getByText('租约关闭时不写任何记录。')).toBeInTheDocument();
    });
});
