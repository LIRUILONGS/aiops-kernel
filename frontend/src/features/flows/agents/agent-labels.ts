import { AgentType } from '@/graphql/types';
import { formatName } from '@/lib/utils/format';

export const AGENT_LABELS: Record<AgentType, string> = {
    [AgentType.Assistant]: '助手',
    [AgentType.ChangeAnalyst]: '变更分析员',
    [AgentType.Commander]: '事故指挥',
    [AgentType.Generator]: '诊断规划器',
    [AgentType.LogAnalyst]: '日志分析员',
    [AgentType.MetricsAnalyst]: '指标分析员',
    [AgentType.Refiner]: '计划修订器',
    [AgentType.Reflector]: '反思员',
    [AgentType.Reporter]: '事故报告员',
    [AgentType.Summarizer]: '摘要员',
    [AgentType.ToolCallFixer]: '工具调用修复',
    [AgentType.Triage]: '分诊员',
};

export const formatAgentType = (type?: null | string): string => {
    if (!type) {
        return '';
    }

    return AGENT_LABELS[type as AgentType] ?? formatName(type);
};
