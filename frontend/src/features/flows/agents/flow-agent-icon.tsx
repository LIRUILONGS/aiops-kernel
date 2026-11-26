import type { LucideIcon } from 'lucide-react';

import {
    Bot,
    FileText,
    LayoutList,
    MessagesSquare,
    Radio,
    RefreshCw,
    ScrollText,
    Sigma,
    Siren,
    Wrench,
} from 'lucide-react';

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { AgentType } from '@/graphql/types';
import { cn } from '@/lib/utils';

import { formatAgentType } from './agent-labels';

interface FlowAgentIconProps {
    className?: string;
    tooltip?: string;
    type?: AgentType;
}

const icons: Record<AgentType, LucideIcon> = {
    [AgentType.Assistant]: Bot,
    [AgentType.ChangeAnalyst]: RefreshCw,
    [AgentType.Commander]: Siren,
    [AgentType.Generator]: LayoutList,
    [AgentType.LogAnalyst]: ScrollText,
    [AgentType.MetricsAnalyst]: Sigma,
    [AgentType.Refiner]: RefreshCw,
    [AgentType.Reflector]: MessagesSquare,
    [AgentType.Reporter]: FileText,
    [AgentType.Summarizer]: Sigma,
    [AgentType.ToolCallFixer]: Wrench,
    [AgentType.Triage]: Radio,
};
const defaultIcon = Bot;

function FlowAgentIcon({ className, type, tooltip = type }: FlowAgentIconProps) {
    const Icon = type ? icons[type] || defaultIcon : defaultIcon;
    const iconElement = <Icon className={cn('size-3 shrink-0', className)} />;

    if (!tooltip) {
        return iconElement;
    }

    return (
        <Tooltip>
            <TooltipTrigger asChild>{iconElement}</TooltipTrigger>
            <TooltipContent>{formatAgentType(tooltip)}</TooltipContent>
        </Tooltip>
    );
}

export default FlowAgentIcon;
