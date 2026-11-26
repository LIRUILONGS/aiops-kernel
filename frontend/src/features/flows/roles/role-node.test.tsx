import { render, screen } from '@testing-library/react';
import { type NodeProps } from '@xyflow/react';
import { describe, expect, it, vi } from 'vitest';

import { AgentType } from '@/graphql/types';

import RoleNode, { type RoleFlowNode } from './role-node';

vi.mock('@xyflow/react', () => ({
    Handle: () => null,
    Position: { Bottom: 'bottom', Top: 'top' },
}));

const data = {
    graphNode: {
        chains: 3,
        cost: 0.5,
        isActive: false,
        lastAt: null,
        logs: 2,
        role: AgentType.LogAnalyst,
        tokens: 3300,
        x: 0,
        y: 192,
    },
    isSelected: false,
};

describe('RoleNode', () => {
    it('renders the Chinese role name and Chinese counters', () => {
        render(<RoleNode {...({ data, selected: false } as unknown as NodeProps<RoleFlowNode>)} />);

        expect(screen.getByText('日志分析员')).toBeInTheDocument();
        expect(screen.getByText('3 链')).toBeInTheDocument();
        expect(screen.getByText('2 日志')).toBeInTheDocument();
        expect(screen.getByText('3.3K token')).toBeInTheDocument();
        expect(screen.getByText('—')).toBeInTheDocument();
    });
});
