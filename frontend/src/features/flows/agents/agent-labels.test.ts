import { describe, expect, it } from 'vitest';

import { AgentType } from '@/graphql/types';

import { AGENT_LABELS, formatAgentType } from './agent-labels';

describe('formatAgentType', () => {
    it('names every shipped role in Chinese', () => {
        expect(AGENT_LABELS).toEqual({
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
        });
    });

    it('falls back to the enum spelling for a role the build does not know', () => {
        expect(formatAgentType('future_agent')).toBe('Future Agent');
        expect(formatAgentType('')).toBe('');
        expect(formatAgentType(null)).toBe('');
        expect(formatAgentType(undefined)).toBe('');
    });
});
