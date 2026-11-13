import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { AgentType, StatusType } from '@/graphql/types';

const queryState = vi.hoisted(() => ({ current: {} as Record<string, unknown> }));
const flowState = vi.hoisted(() => ({ current: {} as Record<string, unknown> }));

vi.mock('@apollo/client/react', () => ({ useQuery: () => queryState.current }));
vi.mock('@/providers/flow-provider', () => ({ useFlow: () => flowState.current }));
vi.mock('../agents/flow-agent', () => ({
    default: ({ log }: { log: { executor: AgentType; task: string } }) => (
        <div data-slot="flow-agent">
            {log.executor}:{log.task}
        </div>
    ),
}));
vi.mock('@xyflow/react', () => ({
    Background: () => null,
    BackgroundVariant: { Dots: 'dots' },
    Controls: () => null,
    Handle: () => null,
    MarkerType: { ArrowClosed: 'arrow-closed' },
    Position: { Bottom: 'bottom', Top: 'top' },
    ReactFlow: ({
        nodes,
        onNodeClick,
    }: {
        nodes: { id: string }[];
        onNodeClick: (event: unknown, node: { id: string }) => void;
    }) => (
        <div data-slot="role-topology">
            {nodes.map((node) => (
                <button
                    key={node.id}
                    onClick={() => onNodeClick({}, node)}
                    type="button"
                >
                    node-{node.id}
                </button>
            ))}
        </div>
    ),
}));

const { default: FlowRoles } = await import('./flow-roles');

const agentLogs = [
    { executor: AgentType.Commander, id: '1', initiator: AgentType.Generator, task: 'decompose the incident' },
    { executor: AgentType.LogAnalyst, id: '2', initiator: AgentType.Commander, task: 'read the nginx log' },
    { executor: AgentType.LogAnalyst, id: '3', initiator: AgentType.Commander, task: 'read the app log' },
];

const activity = {
    edges: [
        { from: AgentType.Commander, lastAt: '2026-11-13T09:59:50.000Z', logs: 2, to: AgentType.LogAnalyst },
        { from: AgentType.Generator, lastAt: '2026-11-13T09:59:40.000Z', logs: 1, to: AgentType.Commander },
    ],
    roles: [
        {
            chains: 1,
            cost: 0,
            firstAt: '2026-11-13T09:30:00.000Z',
            lastAt: '2026-11-13T09:59:55.000Z',
            logs: 1,
            role: AgentType.Commander,
            usageIn: 900,
            usageOut: 100,
        },
        {
            chains: 0,
            cost: 0,
            firstAt: null,
            lastAt: '2026-11-13T09:59:50.000Z',
            logs: 2,
            role: AgentType.LogAnalyst,
            usageIn: 0,
            usageOut: 0,
        },
        {
            chains: 1,
            cost: 0,
            firstAt: '2026-11-13T09:30:00.000Z',
            lastAt: '2026-11-13T09:59:40.000Z',
            logs: 0,
            role: AgentType.Generator,
            usageIn: 400,
            usageOut: 600,
        },
    ],
};

const renderRoles = (state: Record<string, unknown> = {}) => {
    flowState.current = {
        flowData: { agentLogs },
        flowId: '42',
        flowStatus: StatusType.Running,
        ...state,
    };

    return render(<FlowRoles />);
};

describe('FlowRoles', () => {
    it('offers a chip for each role that ran and the whole log by default', () => {
        queryState.current = { data: { flowRoleActivity: activity }, loading: false };

        renderRoles();

        expect(screen.getByRole('button', { name: /All roles/ })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Commander/ })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Log Analyst/ })).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: /Reporter/ })).not.toBeInTheDocument();
        expect(screen.getAllByText(/^(commander|log_analyst|generator):/)).toHaveLength(3);
    });

    it('narrows the log to the role picked in the topology', async () => {
        queryState.current = { data: { flowRoleActivity: activity }, loading: false };

        renderRoles();
        await userEvent.click(screen.getByRole('button', { name: /^Commander/ }));

        expect(screen.getAllByText(/^(commander|log_analyst|generator):/)).toHaveLength(3);

        await userEvent.click(screen.getByRole('button', { name: 'node-log_analyst' }));

        expect(screen.getAllByText(/^(commander|log_analyst|generator):/)).toHaveLength(2);
        expect(screen.getByText('log_analyst:read the nginx log')).toBeInTheDocument();
        expect(screen.queryByText('commander:decompose the incident')).not.toBeInTheDocument();

        await userEvent.click(screen.getByRole('button', { name: /All roles/ }));

        expect(screen.getAllByText(/^(commander|log_analyst|generator):/)).toHaveLength(3);
    });

    it('says so when no role has run yet', () => {
        queryState.current = { data: { flowRoleActivity: { edges: [], roles: [] } }, loading: false };

        renderRoles({ flowData: { agentLogs: [] } });

        expect(screen.getByText('No role has run in this flow yet.')).toBeInTheDocument();
        expect(screen.getByText('No role log yet')).toBeInTheDocument();
    });

    it('reports a failed read of the role activity', () => {
        queryState.current = { data: undefined, error: new Error('agentlogs.view denied'), loading: false };

        renderRoles();

        expect(screen.getByText(/Failed to read the role activity: agentlogs\.view denied/)).toBeInTheDocument();
    });
});
