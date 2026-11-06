import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import FlowPerformance from './flow-performance';

const toolcalls = vi.fn();

vi.mock('@/lib/axios', async (importOriginal) => {
    const actual = await importOriginal<typeof import('@/lib/axios')>();

    return {
        ...actual,
        api: { get: (...args: unknown[]) => toolcalls(...args) },
    };
});

const BASELINE = `已采集基线 "before"
{"label":"before","captured_at":"2026-11-06T12:46:29Z","host":"lab","load1":0.04,"load5":0.07,"load15":0.08,"cpu_percent":2,"mem_used_mb":546,"mem_total_mb":1740,"swap_used_mb":0,"filesystems":{"/":12},"containers":{"caddy":{"state":"running","restarts":0,"exit_code":0}}}`;

const BASELINE_AFTER = `已采集基线 "after"
{"label":"after","captured_at":"2026-11-06T12:56:29Z","host":"lab","load1":0.11,"load5":0.07,"load15":0.08,"cpu_percent":5,"mem_used_mb":600,"mem_total_mb":1740,"swap_used_mb":0,"filesystems":{"/":12},"containers":{"caddy":{"state":"running","restarts":3,"exit_code":0}}}`;

const DIFF = `对比：before（2026-11-06 12:46:29）→ after（2026-11-06 12:56:29）
共 2 项，其中显著变化 1 项

指标	变化前	变化后	变化量	判定
load1	0.04	0.11	+0.07 (+175%)	恶化 ←
cpu	2.0%	5.0%	+3.0% (+150%)	持平

有两项显著变化，本次变更产生了可测量的恶化。`;

function answer(rows: { name: string; result: string }[]) {
    return {
        data: { toolcalls: rows.map((row, index) => ({ ...row, created_at: `2026-11-06T12:5${index}:00Z` })) },
        status: 'success',
    };
}

describe('FlowPerformance', () => {
    beforeEach(() => {
        toolcalls.mockReset();
    });

    it('renders nothing for a flow that never measured anything', async () => {
        toolcalls.mockResolvedValue(answer([{ name: 'ops_system', result: 'host=lab' }]));

        const { container } = render(<FlowPerformance flowId="12" />);

        await waitFor(() => expect(toolcalls).toHaveBeenCalled());
        expect(container.querySelector('[data-testid="flow-performance"]')).toBeNull();
    });

    it('shows every baseline sample in the order it was taken', async () => {
        toolcalls.mockResolvedValue(
            answer([
                { name: 'ops_baseline', result: BASELINE },
                { name: 'ops_baseline', result: BASELINE_AFTER },
            ]),
        );

        const { container } = render(<FlowPerformance flowId="12" />);

        await screen.findByText('before');
        const labels = [...container.querySelectorAll('tbody tr td:first-child')].map((cell) => cell.textContent);
        expect(labels).toEqual(['before', 'after']);
        // The second sample's restart count is shown with its distance from the first.
        expect(screen.getByText('3 (+3)')).toBeInTheDocument();
    });

    it('renders the comparison with its verdicts and the sentence that explains them', async () => {
        toolcalls.mockResolvedValue(
            answer([
                { name: 'ops_baseline', result: BASELINE },
                { name: 'ops_diff', result: DIFF },
            ]),
        );

        const { container } = render(<FlowPerformance flowId="12" />);

        await screen.findByText('load1');
        const deltas = [...container.querySelectorAll('[data-testid="performance-delta"]')];
        expect(deltas).toHaveLength(2);
        expect(deltas[0]).toHaveTextContent('load1');
        expect(deltas[0]).toHaveTextContent('+0.07 (+175%)');
        expect(deltas[0]).toHaveTextContent('worsened');
        expect(screen.getByText(/1 of 2 material/)).toBeInTheDocument();
        expect(screen.getByText(/可测量的恶化/)).toBeInTheDocument();
    });

    it('says so when the measurements cannot be read', async () => {
        toolcalls.mockRejectedValue(new Error('permission denied'));

        const { container } = render(<FlowPerformance flowId="12" />);

        await waitFor(() => expect(toolcalls).toHaveBeenCalled());
        expect(container.querySelector('[data-testid="flow-performance"]')).toBeNull();
    });
});
