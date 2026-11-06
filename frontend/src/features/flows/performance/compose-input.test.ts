import { describe, expect, it } from 'vitest';

import { composeFlowInput, composePerformanceInput, stripPerformanceGoal } from './compose-input';

const ALERT = '把 caddy 容器的 p99 延迟降下来，先采基线再改配置';

describe('composePerformanceInput', () => {
    it('leaves the input alone when no goal was filled in', () => {
        expect(composePerformanceInput(ALERT)).toBe(ALERT);
        expect(composePerformanceInput(ALERT, { constraint: '  ', metric: '', target: undefined })).toBe(ALERT);
    });

    it('appends only the lines that were filled in', () => {
        const composed = composePerformanceInput(ALERT, { metric: 'p99 延迟', target: '< 200ms' });

        expect(composed).toBe(`${ALERT}\n\n【性能目标】\n- 指标：p99 延迟\n- 目标：< 200ms`);
    });

    it('keeps all three when all three are given', () => {
        const composed = composePerformanceInput(ALERT, {
            constraint: '不要重启 caddy',
            metric: 'p99 延迟',
            target: '< 200ms',
        });

        expect(composed).toContain('- 指标：p99 延迟');
        expect(composed).toContain('- 目标：< 200ms');
        expect(composed).toContain('- 约束：不要重启 caddy');
    });

    it('replaces an existing goal block instead of stacking a second one', () => {
        const once = composePerformanceInput(ALERT, { metric: 'p99' });
        const twice = composePerformanceInput(once, { metric: 'p95', target: '< 100ms' });

        expect(twice.match(/【性能目标】/g)).toHaveLength(1);
        expect(twice).toContain('- 指标：p95');
        expect(twice).not.toContain('- 指标：p99');
    });

    it('drops a stale block when the flow is no longer an optimisation', () => {
        const once = composePerformanceInput(ALERT, { metric: 'p99', target: '< 200ms' });

        expect(composePerformanceInput(once)).toBe(ALERT);
    });

    it('keeps another bracketed section that follows the goal block', () => {
        const input = `${ALERT}\n\n【性能目标】\n- 指标：p99\n\n【禁止事项】\n- 不要动 caddy 的证书`;

        expect(stripPerformanceGoal(input)).toBe(`${ALERT}\n\n【禁止事项】\n- 不要动 caddy 的证书`);
    });
});

describe('composeFlowInput', () => {
    const values = {
        goalConstraint: '不要重启 caddy',
        goalMetric: 'p99 延迟',
        goalTarget: '< 200ms',
        message: ALERT,
        targetType: 'optimization',
    };

    it('carries the goal for an optimisation flow', () => {
        expect(composeFlowInput(values)).toBe(
            `${ALERT}\n\n【性能目标】\n- 指标：p99 延迟\n- 目标：< 200ms\n- 约束：不要重启 caddy`,
        );
    });

    it('drops the goal when the flow is a diagnosis', () => {
        expect(composeFlowInput({ ...values, targetType: 'diagnosis' })).toBe(ALERT);
    });

    it('drops the goal when the target type is left to the planner', () => {
        expect(composeFlowInput({ ...values, targetType: 'auto' })).toBe(ALERT);
    });

    it('composes an optimisation that has no goal fields at all', () => {
        expect(composeFlowInput({ message: ALERT, targetType: 'optimization' })).toBe(ALERT);
    });
});
