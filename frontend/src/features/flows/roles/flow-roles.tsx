import { useQuery } from '@apollo/client/react';
import { ListFilter } from 'lucide-react';
import { useMemo, useState } from 'react';

import type { FlowRoleActivityQuery } from '@/graphql/types';

import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area';
import { Spinner } from '@/components/ui/spinner';
import { AgentType, FlowRoleActivityDocument, StatusType } from '@/graphql/types';
import { cn } from '@/lib/utils';
import { useFlow } from '@/providers/flow-provider';

import { formatAgentType } from '../agents/agent-labels';
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
                <span>点一个角色，查看它写下或应答的日志。</span>
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
                            'rounded-full border px-2.5 py-1 text-xs whitespace-nowrap transition-colors',
                            !selected
                                ? 'border-primary bg-primary/10 text-primary'
                                : 'text-muted-foreground hover:bg-muted',
                        )}
                        onClick={() => setSelected(null)}
                        type="button"
                    >
                        全部角色
                        <span className="text-muted-foreground ml-1.5 font-mono text-[10px]">{logs.length}</span>
                    </button>
                    {activeRoles.map((node) => (
                        <button
                            className={cn(
                                'flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs whitespace-nowrap transition-colors',
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
                            {formatAgentType(node.role)}
                            <span className="font-mono text-[10px]">{node.logs}</span>
                        </button>
                    ))}
                </div>
                <ScrollBar orientation="horizontal" />
            </ScrollArea>

            {error && <p className="text-destructive text-xs">读取角色活动失败：{error.message}</p>}

            {!graph.hasActivity && !error && <p className="text-muted-foreground text-xs">这条会话还没有角色跑过。</p>}

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
                        <EmptyTitle>
                            {selected ? `${formatAgentType(selected)}还没有日志` : '还没有角色日志'}
                        </EmptyTitle>
                        <EmptyDescription>
                            {selected
                                ? '这个角色在这条会话里还没有写下或应答日志。'
                                : '有角色应答委派后，日志会出现在这里。'}
                        </EmptyDescription>
                    </EmptyHeader>
                </Empty>
            )}
        </div>
    );
}

export default FlowRoles;
