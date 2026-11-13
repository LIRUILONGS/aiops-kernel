import { describe, expect, it } from 'vitest';

import { AgentType } from '@/graphql/types';

import { buildRoleGraph, filterRoleLogs, formatRelativeAge } from './role-graph';

const NOW = Date.parse('2026-11-13T10:00:00.000Z');

const activity = {
    edges: [
        { from: AgentType.Commander, lastAt: '2026-11-13T09:59:50.000Z', logs: 4, to: AgentType.LogAnalyst },
        { from: AgentType.Commander, lastAt: '2026-11-13T09:40:00.000Z', logs: 1, to: AgentType.Generator },
    ],
    roles: [
        {
            chains: 2,
            cost: 0.75,
            firstAt: '2026-11-13T09:30:00.000Z',
            lastAt: '2026-11-13T09:59:55.000Z',
            logs: 1,
            role: AgentType.Commander,
            usageIn: 1200,
            usageOut: 300,
        },
        {
            chains: 0,
            cost: 0,
            firstAt: null,
            lastAt: '2026-11-13T09:59:50.000Z',
            logs: 4,
            role: AgentType.LogAnalyst,
            usageIn: 0,
            usageOut: 0,
        },
    ],
};

const node = (role: AgentType) => buildRoleGraph(activity, NOW).nodes.find((item) => item.role === role);

describe('buildRoleGraph', () => {
    it('places every role of the product on its tier, even the ones that never ran', () => {
        const { nodes } = buildRoleGraph(undefined, NOW);

        expect(nodes).toHaveLength(12);
        expect(node(AgentType.Generator)?.y).toBe(0);
        expect(node(AgentType.Commander)?.y).toBe(96);
        expect(node(AgentType.MetricsAnalyst)?.y).toBe(192);
        expect(node(AgentType.Reporter)?.y).toBe(288);
        expect(node(AgentType.Reporter)?.chains).toBe(0);
        expect(node(AgentType.Reporter)?.tokens).toBe(0);
    });

    it('centers a tier and spreads its roles by column', () => {
        const { nodes } = buildRoleGraph(undefined, NOW);
        const analysts = nodes.filter((item) => item.y === 192);
        const zeros = analysts.map((item) => item.x);

        expect(zeros).toEqual([-315, -105, 105, 315]);
    });

    it('carries the metrics of a role that ran and zeroes the ones that did not', () => {
        const commander = node(AgentType.Commander);
        const reporter = node(AgentType.Reporter);

        expect(commander?.chains).toBe(2);
        expect(commander?.logs).toBe(1);
        expect(commander?.tokens).toBe(1500);
        expect(commander?.cost).toBe(0.75);
        expect(commander?.isActive).toBe(true);
        expect(reporter?.isActive).toBe(false);
        expect(reporter?.lastAt).toBeNull();
    });

    it('marks a delegation active only while it is inside the window', () => {
        const { edges } = buildRoleGraph(activity, NOW);

        expect(edges.map((edge) => [edge.id, edge.isActive, edge.logs])).toEqual([
            ['commander->generator', false, 1],
            ['commander->log_analyst', true, 4],
        ]);
    });

    it('reports whether anything ran at all', () => {
        expect(buildRoleGraph(undefined, NOW).hasActivity).toBe(false);
        expect(buildRoleGraph({ edges: [], roles: [] }, NOW).hasActivity).toBe(false);
        expect(buildRoleGraph(activity, NOW).hasActivity).toBe(true);
    });
});

describe('filterRoleLogs', () => {
    const logs = [
        { executor: AgentType.Commander, id: '1', initiator: AgentType.Commander },
        { executor: AgentType.LogAnalyst, id: '2', initiator: AgentType.Commander },
        { executor: AgentType.Generator, id: '3', initiator: AgentType.Commander },
    ];

    it('keeps the log a role wrote and the log it asked for', () => {
        expect(filterRoleLogs(logs, AgentType.Commander).map((log) => log.id)).toEqual(['1', '2', '3']);
        expect(filterRoleLogs(logs, AgentType.LogAnalyst).map((log) => log.id)).toEqual(['2']);
        expect(filterRoleLogs(logs, AgentType.Reporter)).toEqual([]);
    });

    it('keeps everything when no role is selected', () => {
        expect(filterRoleLogs(logs, null)).toHaveLength(3);
    });
});

describe('formatRelativeAge', () => {
    it('answers a short age for every distance', () => {
        expect(formatRelativeAge(null, NOW)).toBe('—');
        expect(formatRelativeAge('2026-11-13T09:59:58.000Z', NOW)).toBe('just now');
        expect(formatRelativeAge('2026-11-13T09:59:30.000Z', NOW)).toBe('30s ago');
        expect(formatRelativeAge('2026-11-13T09:58:00.000Z', NOW)).toBe('2m ago');
        expect(formatRelativeAge('2026-11-13T08:00:00.000Z', NOW)).toBe('2h ago');
        expect(formatRelativeAge('2026-11-10T10:00:00.000Z', NOW)).toBe('3d ago');
    });
});
