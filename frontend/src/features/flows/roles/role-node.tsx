import { Handle, type Node, type NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';

import { cn } from '@/lib/utils';
import { formatCost, formatTokenCount } from '@/lib/utils/format';

import { formatAgentType } from '../agents/agent-labels';
import FlowAgentIcon from '../agents/flow-agent-icon';
import { formatRelativeAge, type RoleGraphNode } from './role-graph';

export type RoleFlowNode = Node<{ graphNode: RoleGraphNode; isSelected: boolean }, 'role'>;

function RoleNode({ data, selected }: NodeProps<RoleFlowNode>) {
    const { graphNode } = data;
    const isHighlighted = selected || data.isSelected;

    return (
        <div
            className={cn(
                'bg-card flex w-[184px] flex-col gap-1 rounded-md border px-3 py-2 text-left shadow-xs transition-colors',
                graphNode.isActive && 'border-emerald-500/60',
                isHighlighted && 'ring-primary ring-2',
            )}
        >
            <Handle
                className="opacity-0"
                position={Position.Top}
                type="target"
            />
            <div className="flex items-center gap-1.5">
                <FlowAgentIcon
                    className="size-3.5"
                    tooltip=""
                    type={graphNode.role}
                />
                <span className="truncate text-xs font-medium">{formatAgentType(graphNode.role)}</span>
                {graphNode.isActive && (
                    <span className="ml-auto size-1.5 shrink-0 animate-pulse rounded-full bg-emerald-500" />
                )}
            </div>
            <div className="text-muted-foreground flex items-center gap-2 font-mono text-[10px]">
                <span>{graphNode.chains} 链</span>
                <span>{graphNode.logs} 日志</span>
                <span>{formatTokenCount(graphNode.tokens)} token</span>
            </div>
            <div className="text-muted-foreground flex items-center gap-2 font-mono text-[10px]">
                {graphNode.cost > 0 && <span>{formatCost(graphNode.cost)}</span>}
                <span className="ml-auto">{formatRelativeAge(graphNode.lastAt)}</span>
            </div>
            <Handle
                className="opacity-0"
                position={Position.Bottom}
                type="source"
            />
        </div>
    );
}

export default memo(RoleNode);
