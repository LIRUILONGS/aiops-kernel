// The metrics a flow actually queried, lifted out of the tool call result.
//
// prom_query answers with prose a model reads and, below it, a JSON block a chart
// can use. The prose states the extremes and the largest jump; the block carries
// the sampled points. Only the block is parsed here: a chart drawn from a summary
// would be a straight line between two numbers, which is the one thing a curve
// must not be.

export interface MetricPoint {
    // Seconds since the epoch, as the metrics API reports them.
    at: number;
    value: number;
}

export interface MetricSeries {
    end: number;
    name: string;
    points: MetricPoint[];
    query: string;
    range: string;
    step: string;
}

interface RawPayload {
    end?: unknown;
    query?: unknown;
    range?: unknown;
    series?: unknown;
    step?: unknown;
}

interface RawPoint {
    0?: unknown;
    1?: unknown;
}

interface RawSeries {
    name?: unknown;
    points?: unknown;
}

const BLOCK = /### 结构化结果\s*```json\s*(\{[\s\S]*?\})\s*```/;

// The jump is what the prose already names, and a chart that marks a different
// point than the report would read as two findings.
export function largestJump(points: MetricPoint[]): MetricPoint | null {
    if (points.length < 2) {
        return null;
    }

    let best: MetricPoint | null = null;
    let bestRatio = 0;

    for (let index = 1; index < points.length; index += 1) {
        const previous = points[index - 1];
        const current = points[index];

        if (!previous || !current) {
            continue;
        }

        const denominator = Math.abs(previous.value) || 1;
        const ratio = Math.abs(current.value - previous.value) / denominator;

        if (ratio > bestRatio) {
            bestRatio = ratio;
            best = current;
        }
    }

    return best;
}

export function parseMetricSeries(result?: string): MetricSeries[] {
    if (!result) {
        return [];
    }

    const match = BLOCK.exec(result);

    if (!match) {
        return [];
    }

    const raw = match[1];

    if (raw === undefined) {
        return [];
    }

    let payload: RawPayload;

    try {
        payload = JSON.parse(raw) as RawPayload;
    } catch {
        return [];
    }

    if (!Array.isArray(payload.series)) {
        return [];
    }

    const end = typeof payload.end === 'number' ? payload.end : 0;
    const query = typeof payload.query === 'string' ? payload.query : '';
    const range = typeof payload.range === 'string' ? payload.range : '';
    const step = typeof payload.step === 'string' ? payload.step : '';

    return payload.series
        .map((raw): MetricSeries | null => {
            const serie = raw as RawSeries;
            const name = typeof serie.name === 'string' ? serie.name : '';
            const points = readPoints(serie.points);

            if (!name || points.length < 2) {
                return null;
            }

            return { end, name, points, query, range, step };
        })
        .filter((serie): serie is MetricSeries => serie !== null);
}

function readPoints(raw: unknown): MetricPoint[] {
    if (!Array.isArray(raw)) {
        return [];
    }

    const points: MetricPoint[] = [];

    for (const entry of raw) {
        const pair = entry as RawPoint;
        const at = Number(pair?.[0]);
        const value = Number(pair?.[1]);

        if (!Number.isFinite(at) || !Number.isFinite(value)) {
            continue;
        }

        points.push({ at, value });
    }

    return points;
}
