import { useQuery } from '@apollo/client/react';
import { ListFilter } from 'lucide-react';
import { useMemo, useState } from 'react';

import type { FlowRoleActivityQuery } from '@/graphql/types';

import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area';
import { Spinner } from '@/components/ui/spinner';
import { AgentType, FlowRoleActivityDocument, StatusType } from '@/graphql/types';
import { cn } from '@/lib/utils';
import { formatName } from '@/lib/utils/format';
import { useFlow } from '@/providers/flow-provider';

import FlowAgent from '../agents/flow-agent';
import FlowAgentIcon from '../agents/flow-agent-icon';
import { buildRoleGraph, filterRoleLogs } from './role-graph';
import RoleTopology from './role-topology';

const liveStatuses: StatusType[] = [StatusType.Created, StatusType.Running, StatusType.Waiting];

function FlowRoles() {
    const { flowData, flowId, flowStatus } = useFlow();
    const [selected, setSelected] = useState<AgentType | null>(null);

    const isLive = !!flowStatus && liveStatuses.includes(flowStatus);

    const { data, error, loading } = useQuery(FlowRoleActivityDocument, {
        pollInterval: isLive ? 5_000 : 0,
        skip: !flowId,
        variables: { flowId: flowId ?? '' },
    });

    // Apollo types every query result as deeply partial; the field is there once `data` is.
    const activity = data?.flowRoleActivity as FlowRoleActivityQuery['flowRoleActivity'] | undefined;

    const graph = useMemo(() => buildRoleGraph(activity), [activity]);

    const logs = useMemo(() => flowData?.agentLogs ?? [], [flowData?.agentLogs]);
    const roleLogs = useMemo(() => filterRoleLogs(logs, selected), [logs, selected]);

    const activeRoles = useMemo(() => graph.nodes.filter((node) => node.chains > 0 || node.logs > 0), [graph.nodes]);

    if (loading && !data) {
        return (
            <div className="flex size-full items-center justify-center">
                <Spinner />
            </div>
        );
    }

    return (
        <div className="flex size-full min-h-0 flex-col gap-3">
            <div className="text-muted-foreground flex items-center gap-2 text-xs">
                <ListFilter className="size-3.5" />
                <span>Click a role to read the log it wrote or answered.</span>
            </div>

            <RoleTopology
                graph={graph}
                onSelect={setSelected}
                selected={selected}
            />

            <ScrollArea className="w-full">
                <div className="flex w-fit items-center gap-1.5 pb-1">
                    <button
                        className={cn(
                            'rounded-full border px-2.5 py-1 text-xs transition-colors',
                            !selected
                                ? 'border-primary bg-primary/10 text-primary'
                                : 'text-muted-foreground hover:bg-muted',
                        )}
                        onClick={() => setSelected(null)}
                        type="button"
                    >
                        All roles
                        <span className="text-muted-foreground ml-1.5 font-mono text-[10px]">{logs.length}</span>
                    </button>
                    {activeRoles.map((node) => (
                        <button
                            className={cn(
                                'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors',
                                selected === node.role
                                    ? 'border-primary bg-primary/10 text-primary'
                                    : 'text-muted-foreground hover:bg-muted',
                            )}
                            key={node.role}
                            onClick={() => setSelected(node.role === selected ? null : node.role)}
                            type="button"
                        >
                            <FlowAgentIcon
                                className="size-3"
                                tooltip=""
                                type={node.role}
                            />
                            {formatName(node.role)}
                            <span className="font-mono text-[10px]">{node.logs}</span>
                        </button>
                    ))}
                </div>
                <ScrollBar orientation="horizontal" />
            </ScrollArea>

            {error && <p className="text-destructive text-xs">Failed to read the role activity: {error.message}</p>}

            {!graph.hasActivity && !error && (
                <p className="text-muted-foreground text-xs">No role has run in this flow yet.</p>
            )}

            {roleLogs.length > 0 ? (
                <ScrollArea className="min-h-0 flex-1 pr-3">
                    <div className="flex flex-col gap-3">
                        {roleLogs.map((log) => (
                            <FlowAgent
                                key={log.id}
                                log={log}
                            />
                        ))}
                    </div>
                </ScrollArea>
            ) : (
                <Empty className="flex-1 border border-dashed">
                    <EmptyHeader>
                        <EmptyMedia variant="icon">
                            <ListFilter />
                        </EmptyMedia>
                        <EmptyTitle>{selected ? `No log for ${formatName(selected)}` : 'No role log yet'}</EmptyTitle>
                        <EmptyDescription>
                            {selected
                                ? 'This role has not written or answered a log in this flow.'
                                : 'A role log appears here as soon as a role answers a delegation.'}
                        </EmptyDescription>
                    </EmptyHeader>
                </Empty>
            )}
        </div>
    );
}

export default FlowRoles;
