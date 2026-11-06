import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { MetricSeries } from '@/features/flows/performance/metric-series';

import {
    groupByQuery,
    MetricChart,
    type MetricGroup,
    MetricGroupSection,
    pathOf,
} from '@/features/flows/performance/metric-chart';

const series: MetricSeries = {
    end: 1793976145,
    name: 'node_load1{instance="lab:9100"}',
    points: [
        { at: 1793976000, value: 0.5 },
        { at: 1793976030, value: 0.6 },
        { at: 1793976060, value: 4.1 },
    ],
    query: 'node_load1',
    range: '5m0s',
    step: '30s',
};

describe('pathOf', () => {
    it('draws one line through every point, in time order', () => {
        const path = pathOf(series, 100, 100);

        expect(path.startsWith('M')).toBe(true);
        expect(path.split(' L')).toHaveLength(3);
        // The last value is the highest, so the last y is the smallest of the three.
        const ys = path
            .replace('M', '')
            .split(' L')
            .map((pair) => Number(pair.split(',')[1] ?? NaN));

        const [, low, high] = ys;
        expect(high).toBeLessThan(low ?? 0);
        expect(low).toBeLessThan(ys[0] ?? 0);
    });

    it('draws nothing for a series with no points', () => {
        expect(pathOf({ ...series, points: [] })).toBe('');
    });
});

describe('groupByQuery', () => {
    const idle = { ...series, name: '{mode="idle"}', query: 'sum by (mode) (rate(cpu[1m]))' };
    const iowait = { ...series, name: '{mode="iowait"}', query: 'sum by (mode) (rate(cpu[1m]))' };

    it('puts the series of one query under one group', () => {
        const groups = groupByQuery([series, idle, iowait]);

        expect(groups).toHaveLength(2);
        expect(groups[0]?.query).toBe('node_load1');
        expect(groups[1]?.series.map((item) => item.name)).toEqual(['{mode="idle"}', '{mode="iowait"}']);
    });

    it('treats the same query over another window as another group', () => {
        const groups = groupByQuery([series, { ...series, range: '1h0m0s' }]);

        expect(groups).toHaveLength(2);
    });

    it('has nothing to group when the flow queried nothing', () => {
        expect(groupByQuery([])).toEqual([]);
    });
});

describe('MetricGroupSection', () => {
    it('states the query once and draws every series under it', () => {
        const group = groupByQuery([
            { ...series, name: '{mode="idle"}', query: 'sum by (mode) (rate(cpu[1m]))' },
            { ...series, name: '{mode="iowait"}', query: 'sum by (mode) (rate(cpu[1m]))' },
        ])[0];

        expect(group).toBeDefined();

        const { container } = render(<MetricGroupSection group={group as MetricGroup} />);

        expect(screen.getByText('sum by (mode) (rate(cpu[1m]))')).toBeInTheDocument();
        expect(screen.getByText(/2 series · 5m0s · step 30s/)).toBeInTheDocument();
        expect(container.querySelectorAll('[data-testid="metric-chart"]')).toHaveLength(2);
    });
});

describe('MetricChart', () => {
    it('renders the selector and the value range', () => {
        render(<MetricChart series={series} />);

        expect(screen.getByText('node_load1{instance="lab:9100"}')).toBeInTheDocument();
        expect(screen.getByText(/0\.5 – 4\.1/)).toBeInTheDocument();
    });

    it('marks the largest jump on the line', () => {
        const { container } = render(<MetricChart series={series} />);

        expect(container.querySelector('[data-testid="metric-chart-jump"]')).not.toBeNull();
    });

    it('marks nothing on a flat series', () => {
        const { container } = render(
            <MetricChart
                series={{
                    ...series,
                    points: [
                        { at: 1, value: 2 },
                        { at: 2, value: 2 },
                    ],
                }}
            />,
        );

        expect(container.querySelector('[data-testid="metric-chart-jump"]')).toBeNull();
    });
});
