import { formatPromptId } from '@/lib/route-titles/format-prompt-id';

const PROMPT_LABELS: Record<string, string> = {
    assistant: '助手',
    changeAnalyst: '变更分析员',
    chooseDockerImage: '选择 Docker 镜像',
    chooseUserLanguage: '选择用户语言',
    collectToolCallId: '收集工具调用 ID',
    commander: '事故指挥',
    detectToolCallIdPattern: '识别工具调用 ID 格式',
    generator: '诊断规划器',
    getExecutionLogs: '获取执行日志',
    getFlowDescription: '生成会话描述',
    getFullExecutionContext: '获取完整执行上下文',
    getShortExecutionContext: '获取简要执行上下文',
    getTaskDescription: '生成任务描述',
    logAnalyst: '日志分析员',
    metricsAnalyst: '指标分析员',
    refiner: '计划修订器',
    reflector: '反思员',
    reporter: '事故报告员',
    summarizer: '摘要员',
    toolCallFixer: '工具调用修复',
    triage: '分诊员',
};

export const formatPromptLabel = (key: string): string => PROMPT_LABELS[key] ?? formatPromptId(key);
