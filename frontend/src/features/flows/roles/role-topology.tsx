import {
    Background,
    BackgroundVariant,
    Controls,
    type Edge,
    MarkerType,
    type NodeTypes,
    ReactFlow,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useCallback, useMemo } from 'react';

import { AgentType } from '@/graphql/types';

import type { RoleGraph } from './role-graph';

import RoleNode, { type RoleFlowNode } from './role-node';

const nodeTypes: NodeTypes = { role: RoleNode };

interface RoleTopologyProps {
    graph: RoleGraph;
    onSelect: (role: AgentType | null) => void;
    selected: AgentType | null;
}

function RoleTopology({ graph, onSelect, selected }: RoleTopologyProps) {
    const nodes = useMemo<RoleFlowNode[]>(
        () =>
            graph.nodes.map((graphNode) => ({
                data: { graphNode, isSelected: graphNode.role === selected },
                id: graphNode.role,
                position: { x: graphNode.x, y: graphNode.y },
                selected: graphNode.role === selected,
                type: 'role' as const,
            })),
        [graph.nodes, selected],
    );

    const edges = useMemo<Edge[]>(
        () =>
            graph.edges.map((edge) => ({
                animated: edge.isActive,
                id: edge.id,
                label: String(edge.logs),
                labelBgPadding: [4, 2] as [number, number],
                labelStyle: { fontSize: 10 },
                markerEnd: { height: 12, type: MarkerType.ArrowClosed, width: 12 },
                source: edge.from,
                style: { strokeWidth: 1.5 },
                target: edge.to,
                type: 'smoothstep',
            })),
        [graph.edges],
    );

    const handleNodeClick = useCallback(
        (_event: unknown, node: { id: string }) => {
            const role = node.id as AgentType;

            onSelect(role === selected ? null : role);
        },
        [onSelect, selected],
    );

    return (
        <div className="bg-muted/30 h-72 w-full rounded-md border">
            <ReactFlow
                edges={edges}
                elementsSelectable
                fitView
                fitViewOptions={{ maxZoom: 1, padding: 0.12 }}
                minZoom={0.3}
                nodes={nodes}
                nodesConnectable={false}
                nodesDraggable={false}
                nodeTypes={nodeTypes}
                onNodeClick={handleNodeClick}
                onPaneClick={() => onSelect(null)}
                proOptions={{ hideAttribution: true }}
            >
                <Background
                    gap={16}
                    size={1}
                    variant={BackgroundVariant.Dots}
                />
                <Controls
                    position="bottom-right"
                    showInteractive={false}
                />
            </ReactFlow>
        </div>
    );
}

export default RoleTopology;
