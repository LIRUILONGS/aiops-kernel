import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, test } from 'vitest';

import FlowMessageConclusion, { parseConclusion } from './flow-message-conclusion';

const DIAGNOSIS_REPORT = `## 诊断结论

**根因**：nginx 配置第 10 行有非法指令
**置信度**：0.85

\`\`\`json
{
  "root_cause": "opslab-broken 的 nginx.conf 第 10 行有非法指令，进程启动即退出",
  "evidence": [
    {"source": "containers", "query": "ops_containers target=lab", "timestamp": "2026-11-06T18:49:00+08:00", "excerpt": "opslab-broken restarting 10 1"}
  ],
  "confidence": 0.85,
  "blast_radius": "lab 上的 HTTP 前端不可用",
  "recommendation": {"action": "删除第 10 行并重建容器", "risk": "low", "rollback": "换回上一版 nginx.conf"},
  "needs_human": true
}
\`\`\`
`;

const OPTIMIZATION_REPORT = `## 性能优化结论

**结论**：unverified

\`\`\`json
{
  "bottleneck": "配置注入的固定 sleep",
  "baseline": "load 0.12, cpu 3.45%",
  "proposal": "把 DELAY_MS 改为 0 并重建",
  "expected_gain": "延迟从 2000ms 降到 100ms",
  "verdict": "unverified",
  "rollback": "把 DELAY_MS 改回 2000",
  "evidence": [
    {"source": "host", "query": "ops_baseline target=lab", "timestamp": "2026-11-06T18:53:24Z", "excerpt": "load1=0.12"}
  ],
  "confidence": 0.6
}
\`\`\`
`;

describe('parseConclusion', () => {
    test('a diagnosis report becomes fields, not prose', () => {
        const conclusion = parseConclusion(DIAGNOSIS_REPORT);

        expect(conclusion?.kind).toBe('diagnosis');

        if (conclusion?.kind !== 'diagnosis') {
            throw new Error('expected a diagnosis');
        }

        expect(conclusion.rootCause).toBe('opslab-broken 的 nginx.conf 第 10 行有非法指令，进程启动即退出');
        expect(conclusion.action).toBe('删除第 10 行并重建容器');
        expect(conclusion.confidence).toBe(0.85);
        expect(conclusion.needsHuman).toBe(true);
        expect(conclusion.evidence).toHaveLength(1);
        expect(conclusion.evidence[0]?.query).toBe('ops_containers target=lab');
    });

    test('an optimisation report keeps the verdict it was filed with', () => {
        const conclusion = parseConclusion(OPTIMIZATION_REPORT);

        expect(conclusion?.kind).toBe('optimization');

        if (conclusion?.kind !== 'optimization') {
            throw new Error('expected an optimisation');
        }

        expect(conclusion.verdict).toBe('unverified');
        expect(conclusion.observedGain).toBeUndefined();
        expect(conclusion.bottleneck).toBe('配置注入的固定 sleep');
    });

    test('a report without a json block is left to the markdown renderer', () => {
        expect(parseConclusion('## 诊断结论\n\n只有散文，没有结构。')).toBeNull();
        expect(parseConclusion(undefined)).toBeNull();
    });

    test('a json block that is not a conclusion is not turned into one', () => {
        expect(parseConclusion('```json\n{"hello":"world"}\n```')).toBeNull();
        expect(parseConclusion('```json\nnot json at all\n```')).toBeNull();
    });
});

describe('FlowMessageConclusion', () => {
    test('an unverified optimisation says so instead of showing a gain', async () => {
        const conclusion = parseConclusion(OPTIMIZATION_REPORT);

        if (!conclusion) {
            throw new Error('the report did not parse');
        }

        render(
            <FlowMessageConclusion
                conclusion={conclusion}
                raw={OPTIMIZATION_REPORT}
            />,
        );

        expect(screen.getAllByText('未重新测量')).toHaveLength(2);
        expect(screen.queryByText('Improved')).not.toBeInTheDocument();
        expect(screen.getByText('证据 (1)')).toBeInTheDocument();
    });

    test('the raw report is one click away', async () => {
        const conclusion = parseConclusion(DIAGNOSIS_REPORT);

        if (!conclusion) {
            throw new Error('the report did not parse');
        }

        render(
            <FlowMessageConclusion
                conclusion={conclusion}
                raw={DIAGNOSIS_REPORT}
            />,
        );

        expect(screen.getByText('需要人工介入')).toBeInTheDocument();

        await userEvent.click(screen.getByRole('button', { name: '展开报告' }));

        expect(screen.getByText('收起报告')).toBeInTheDocument();
        expect(screen.queryByText('下一步动作')).not.toBeInTheDocument();
    });
});
