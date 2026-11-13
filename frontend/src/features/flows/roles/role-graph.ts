import type { FlowRoleActivityQuery } from '@/graphql/types';

import { AgentType } from '@/graphql/types';

export type RoleActivity = FlowRoleActivityQuery['flowRoleActivity']['roles'][number];
export type RoleDelegation = FlowRoleActivityQuery['flowRoleActivity']['edges'][number];

export interface RoleGraph {
    edges: RoleGraphEdge[];
    hasActivity: boolean;
    nodes: RoleGraphNode[];
}

export interface RoleGraphEdge {
    from: AgentType;
    id: string;
    isActive: boolean;
    logs: number;
    to: AgentType;
}

export interface RoleGraphNode {
    chains: number;
    cost: number;
    isActive: boolean;
    lastAt: null | string;
    logs: number;
    role: AgentType;
    tokens: number;
    x: number;
    y: number;
}

const COLUMN_WIDTH = 210;
const ROW_HEIGHT = 96;
const ACTIVE_WINDOW_MS = 60_000;

// The planner sits above the operator, the evidence roles below it, and the
// roles that only consume or repair a chain at the bottom.
const ROLE_TIERS: Record<AgentType, number> = {
    [AgentType.Assistant]: 3,
    [AgentType.ChangeAnalyst]: 2,
    [AgentType.Commander]: 1,
    [AgentType.Generator]: 0,
    [AgentType.LogAnalyst]: 2,
    [AgentType.MetricsAnalyst]: 2,
    [AgentType.Refiner]: 0,
    [AgentType.Reflector]: 3,
    [AgentType.Reporter]: 3,
    [AgentType.Summarizer]: 3,
    [AgentType.ToolCallFixer]: 3,
    [AgentType.Triage]: 2,
};

export const ROLE_ORDER: AgentType[] = Object.values(AgentType);

const centerOffset = (count: number): number => ((count - 1) * COLUMN_WIDTH) / 2;

export const buildRoleGraph = (
    activity: FlowRoleActivityQuery['flowRoleActivity'] | undefined,
    now = Date.now(),
): RoleGraph => {
    const metrics = new Map(activity?.roles.map((role) => [role.role, role]) ?? []);
    const edges = activity?.edges ?? [];

    const byTier = new Map<number, AgentType[]>();

    for (const role of ROLE_ORDER) {
        const tier = ROLE_TIERS[role];
        const row = byTier.get(tier) ?? [];

        row.push(role);
        byTier.set(tier, row);
    }

    const nodes: RoleGraphNode[] = [];

    for (const role of ROLE_ORDER) {
        const tier = ROLE_TIERS[role];
        const row = byTier.get(tier) ?? [];
        const metric = metrics.get(role);
        const lastAt = metric?.lastAt ?? null;

        nodes.push({
            chains: metric?.chains ?? 0,
            cost: metric?.cost ?? 0,
            isActive: isRecent(lastAt, now),
            lastAt,
            logs: metric?.logs ?? 0,
            role,
            tokens: (metric?.usageIn ?? 0) + (metric?.usageOut ?? 0),
            x: row.indexOf(role) * COLUMN_WIDTH - centerOffset(row.length),
            y: tier * ROW_HEIGHT,
        });
    }

    return {
        edges: edges
            .map((edge) => ({
                from: edge.from,
                id: `${edge.from}->${edge.to}`,
                isActive: isRecent(edge.lastAt, now),
                logs: edge.logs,
                to: edge.to,
            }))
            .sort((a, b) => a.id.localeCompare(b.id)),
        hasActivity: edges.length > 0 || metrics.size > 0,
        nodes,
    };
};

const isRecent = (value: null | string, now: number): boolean => {
    if (!value) {
        return false;
    }

    const age = now - new Date(value).getTime();

    return age >= 0 && age <= ACTIVE_WINDOW_MS;
};

export const filterRoleLogs = <T extends { executor: AgentType; initiator: AgentType }>(
    logs: T[],
    role: AgentType | null,
): T[] => (role ? logs.filter((log) => log.initiator === role || log.executor === role) : logs);

export const formatRelativeAge = (value: null | string, now = Date.now()): string => {
    if (!value) {
        return '—';
    }

    const seconds = Math.max(0, Math.round((now - new Date(value).getTime()) / 1000));

    if (seconds < 5) {
        return 'just now';
    }

    if (seconds < 60) {
        return `${seconds}s ago`;
    }

    const minutes = Math.round(seconds / 60);

    if (minutes < 60) {
        return `${minutes}m ago`;
    }

    const hours = Math.round(minutes / 60);

    if (hours < 24) {
        return `${hours}h ago`;
    }

    return `${Math.round(hours / 24)}d ago`;
};
