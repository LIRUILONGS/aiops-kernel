import { describe, expect, it } from 'vitest';

import { formatPromptLabel } from './prompt-labels';

describe('formatPromptLabel', () => {
    it('names every agent and tool prompt the settings page lists', () => {
        expect(formatPromptLabel('changeAnalyst')).toBe('变更分析员');
        expect(formatPromptLabel('toolCallFixer')).toBe('工具调用修复');
        expect(formatPromptLabel('chooseDockerImage')).toBe('选择 Docker 镜像');
        expect(formatPromptLabel('detectToolCallIdPattern')).toBe('识别工具调用 ID 格式');
    });

    it('humanises a key no label covers instead of showing the raw identifier', () => {
        expect(formatPromptLabel('someFuturePrompt')).toBe('Some Future Prompt');
    });
});
