import { useMemo } from 'react';

import { largestJump, type MetricSeries } from '@/features/flows/performance/metric-series';

const WIDTH = 320;
const HEIGHT = 96;
const PAD = 6;

interface Point {
    x: number;
    y: number;
}

// A curve is here to answer one question the tables cannot: was there a moment
// when this metric moved. So the axis limits are printed as numbers and the
// largest jump is marked on the line itself.
export function MetricChart({ series }: { series: MetricSeries }) {
    const values = useMemo(() => series.points.map((point) => point.value), [series.points]);
    const marker = useMemo(() => markerOf(series), [series]);
    const path = useMemo(() => pathOf(series), [series]);

    const min = Math.min(...values);
    const max = Math.max(...values);
    const first = series.points[0];
    const last = series.points[series.points.length - 1];

    if (!first || !last) {
        return null;
    }

    return (
        <figure className="flex flex-col gap-2" data-testid="metric-chart">
            <figcaption className="flex flex-wrap items-baseline justify-between gap-2">
                <code className="text-xs break-all">{series.name}</code>
                <span className="text-muted-foreground text-xs tabular-nums">
                    {series.query} · {series.range} · step {series.step}
                </span>
            </figcaption>
            <svg
                aria-label={`${series.name} over ${series.range}`}
                className="text-primary h-24 w-full"
                preserveAspectRatio="none"
                role="img"
                viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
            >
                <line
                    className="text-border"
                    stroke="currentColor"
                    strokeDasharray="2 3"
                    x1={PAD}
                    x2={WIDTH - PAD}
                    y1={HEIGHT / 2}
                    y2={HEIGHT / 2}
                />
                <path d={path} fill="none" stroke="currentColor" strokeLinejoin="round" strokeWidth={1.5} />
                {marker && (
                    <circle
                        className="text-destructive"
                        cx={marker.x}
                        cy={marker.y}
                        data-testid="metric-chart-jump"
                        fill="currentColor"
                        r={3}
                    />
                )}
            </svg>
            <div className="text-muted-foreground flex justify-between text-xs tabular-nums">
                <span>
                    {formatValue(min)} – {formatValue(max)}
                </span>
                <span>
                    {formatTime(first.at)} → {formatTime(last.at)}
                </span>
            </div>
        </figure>
    );
}

export function pathOf(series: MetricSeries, width = WIDTH, height = HEIGHT): string {
    const points = project(series, width, height);

    if (!points.length) {
        return '';
    }

    return points.map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x.toFixed(1)},${point.y.toFixed(1)}`).join(' ');
}

function formatTime(at: number): string {
    return new Date(at * 1000).toLocaleTimeString(undefined, {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    });
}

function formatValue(value: number): string {
    if (Math.abs(value) >= 1000) {
        return value.toFixed(0);
    }

    return Number.parseFloat(value.toPrecision(4)).toString();
}

function markerOf(series: MetricSeries, width = WIDTH, height = HEIGHT): null | Point {
    const jump = largestJump(series.points);

    if (!jump) {
        return null;
    }

    const index = series.points.indexOf(jump);

    if (index < 0) {
        return null;
    }

    return project(series, width, height)[index] ?? null;
}

function project(series: MetricSeries, width = WIDTH, height = HEIGHT): Point[] {
    const values = series.points.map((point) => point.value);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const span = max - min || 1;
    const usableWidth = width - PAD * 2;
    const usableHeight = height - PAD * 2;
    const lastIndex = Math.max(series.points.length - 1, 1);

    return series.points.map((point, index) => ({
        x: PAD + (index / lastIndex) * usableWidth,
        y: PAD + (1 - (point.value - min) / span) * usableHeight,
    }));
}
