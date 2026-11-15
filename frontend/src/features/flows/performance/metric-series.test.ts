import { describe, expect, it } from 'vitest';

import { largestJump, parseMetricSeries } from '@/features/flows/performance/metric-series';

const result = `监控查询：node_load1
窗口：2026-11-06T22:45:23+08:00 到 2026-11-06T22:50:23+08:00（步长 15s，5m0s）
序列数：1

1. node_load1{instance="lab:9100"}
   点数 3，首值 0.5，末值 4.1（+720.0% 上升），最小 0.5 @22:45:23，最大 4.1 @22:46:23
   最大跳变 +583.3% 上升：22:45:53 0.6 → 4.1 @22:46:23

### 结构化结果

\`\`\`json
{"end":1793976145,"query":"node_load1","range":"5m0s","series":[{"name":"node_load1{instance=\\"lab:9100\\"}","points":[["1793976000","0.5"],["1793976030","0.6"],["1793976060","4.1"]]}],"step":"30s"}
\`\`\`
`;

describe('parseMetricSeries', () => {
    it('reads the series a chart needs out of the tool result', () => {
        const series = parseMetricSeries(result);

        expect(series).toHaveLength(1);

        const first = series[0];
        expect(first?.name).toBe('node_load1{instance="lab:9100"}');
        expect(first?.query).toBe('node_load1');
        expect(first?.range).toBe('5m0s');
        expect(first?.step).toBe('30s');
        expect(first?.points).toEqual([
            { at: 1793976000, value: 0.5 },
            { at: 1793976030, value: 0.6 },
            { at: 1793976060, value: 4.1 },
        ]);
    });

    it('returns nothing when the result carries no block', () => {
        expect(parseMetricSeries('监控查询：node_load1\n窗口内没有数据。')).toEqual([]);
        expect(parseMetricSeries(undefined)).toEqual([]);
    });

    it('returns nothing when the block is not JSON', () => {
        expect(parseMetricSeries('### 结构化结果\n\n```json\n{not json}\n```')).toEqual([]);
    });

    it('drops a series that cannot be drawn', () => {
        const single =
            '### 结构化结果\n\n```json\n{"end":1,"query":"q","range":"5m","series":[{"name":"a","points":[["1","1"]]},{"name":"b","points":[["1","1"],["2","2"]]}],"step":"15s"}\n```';

        expect(parseMetricSeries(single).map((sery) => sery.name)).toEqual(['b']);
    });

    it('skips points whose timestamp or value is not a number', () => {
        const broken =
            '### 结构化结果\n\n```json\n{"end":1,"query":"q","range":"5m","series":[{"name":"a","points":[["1","1"],["x","2"],["3","y"],["4","4"]]}],"step":"15s"}\n```';

        expect(parseMetricSeries(broken)[0]?.points).toEqual([
            { at: 1, value: 1 },
            { at: 4, value: 4 },
        ]);
    });
});

describe('largestJump', () => {
    it('marks the point that moved most relative to the one before it', () => {
        const jump = largestJump([
            { at: 1, value: 0.5 },
            { at: 2, value: 0.6 },
            { at: 3, value: 4.1 },
        ]);

        expect(jump).toEqual({ at: 3, value: 4.1 });
    });

    it('has nothing to mark on a flat series', () => {
        expect(
            largestJump([
                { at: 1, value: 2 },
                { at: 2, value: 2 },
            ]),
        ).toBeNull();
    });
});
