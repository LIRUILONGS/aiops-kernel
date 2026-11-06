import { AlertTriangle, Gauge } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { groupByQuery, MetricGroupSection } from '@/features/flows/performance/metric-chart';
import { type MetricSeries, parseMetricSeries } from '@/features/flows/performance/metric-series';
import {
    type BaselineSample,
    type DiffReport,
    parseBaseline,
    parseDiff,
    VERDICT_TONE,
} from '@/features/flows/performance/performance';
import { api, getApiErrorMessage, unwrapApiResponse } from '@/lib/axios';

interface ToolcallList {
    toolcalls?: ToolcallRow[];
}

// The REST list is the table's own shape, so its columns are snake_case and the
// arguments arrive as a JSON string.
interface ToolcallRow {
    args?: string;
    created_at?: string;
    name: string;
    result?: string;
}

export function diffsOf(calls: ToolcallRow[]): DiffReport[] {
    return calls
        .filter((call) => call.name === 'ops_diff')
        .map((call) => parseDiff(call.result))
        .filter((diff): diff is DiffReport => diff !== null);
}

export function samplesOf(calls: ToolcallRow[]): BaselineSample[] {
    return calls
        .filter((call) => call.name === 'ops_baseline')
        .map((call) => parseBaseline(call.result))
        .filter((sample): sample is BaselineSample => sample !== null);
}

export function seriesOf(calls: ToolcallRow[]): MetricSeries[] {
    return calls
        .filter((call) => call.name === 'prom_query')
        .flatMap((call) => parseMetricSeries(call.result));
}

function Comparison({ diff }: { diff: DiffReport }) {
    return (
        <div>
            <div className="mb-2 flex flex-wrap items-center gap-2 text-xs">
                <span className="text-muted-foreground">
                    {diff.beforeLabel} ({diff.beforeTime}) → {diff.afterLabel} ({diff.afterTime})
                </span>
                <Badge variant={diff.material > 0 ? 'orange' : 'secondary'}>
                    {diff.material} of {diff.total} material
                </Badge>
            </div>
            <Table aria-label="Measured deltas">
                <TableHeader>
                    <TableRow>
                        <TableHead>Metric</TableHead>
                        <TableHead className="text-right">Before</TableHead>
                        <TableHead className="text-right">After</TableHead>
                        <TableHead className="text-right">Change</TableHead>
                        <TableHead className="text-right">Verdict</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {diff.rows.map((row) => (
                        <TableRow
                            className={row.worsened ? 'font-medium' : 'text-muted-foreground'}
                            data-testid="performance-delta"
                            key={row.field}
                        >
                            <TableCell>{row.field}</TableCell>
                            <TableCell className="text-right tabular-nums">{row.before}</TableCell>
                            <TableCell className="text-right tabular-nums">{row.after}</TableCell>
                            <TableCell className="text-right tabular-nums">{row.change}</TableCell>
                            <TableCell className="text-right">
                                <Badge variant={VERDICT_TONE[row.verdict] ?? 'secondary'}>{row.verdict}</Badge>
                                {row.worsened && <span className="text-muted-foreground ml-1 text-xs">worsened</span>}
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
            {diff.summary && <p className="text-muted-foreground mt-2 text-xs">{diff.summary}</p>}
        </div>
    );
}

// The REST list carries the tool name, the arguments and the text result; the
// dashboard reads it rather than the GraphQL schema because the operations an
// agent ran are not part of the flow's typed shape.
async function fetchToolcalls(flowId: string): Promise<ToolcallRow[]> {
    // Relative to the axios base, which already carries /api/v1.
    // The trailing slash keeps the server from answering a redirect first.
    const response = await api.get<ToolcallList>(`flows/${flowId}/toolcalls/`, {
        params: { page: 1, pageSize: -1, type: 'init' },
    });
    // The list endpoint wraps its rows in `data` and again in `toolcalls`; the
    // double wrap is accepted rather than assumed.
    const payload = unwrapApiResponse(response) as ToolcallList & { data?: ToolcallList };
    const rows = payload.toolcalls ?? payload.data?.toolcalls ?? [];

    return [...rows].sort((left, right) => (left.created_at ?? '').localeCompare(right.created_at ?? ''));
}

function FlowPerformance({ flowId }: { flowId: string }) {
    const [calls, setCalls] = useState<ToolcallRow[]>([]);
    const [error, setError] = useState<null | string>(null);

    useEffect(() => {
        let cancelled = false;

        fetchToolcalls(flowId)
            .then((rows) => {
                if (!cancelled) {
                    setCalls(rows);
                }
            })
            .catch((cause: unknown) => {
                if (!cancelled) {
                    setError(getApiErrorMessage(cause, 'the tool calls could not be read'));
                }
            });

        return () => {
            cancelled = true;
        };
    }, [flowId]);

    const samples = useMemo(() => samplesOf(calls), [calls]);
    const diffs = useMemo(() => diffsOf(calls), [calls]);
    const series = useMemo(() => seriesOf(calls), [calls]);
    const groups = useMemo(() => groupByQuery(series), [series]);

    // A flow that never measured anything gets no panel: an empty card would read
    // as "measured, found nothing".
    if (!samples.length && !diffs.length && !series.length) {
        return null;
    }

    return (
        <Card data-testid="flow-performance">
            <CardHeader>
                <CardTitle className="flex items-center gap-2">
                    <Gauge className="size-4" />
                    Performance
                </CardTitle>
                <CardDescription>
                    {samples.length} sample{samples.length === 1 ? '' : 's'} from ops_baseline, {diffs.length}{' '}
                    comparison{diffs.length === 1 ? '' : 's'} from ops_diff, {series.length} series from prom_query
                </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-6">
                {error && (
                    <div className="text-muted-foreground flex items-center gap-2 text-sm">
                        <AlertTriangle className="size-4" />
                        Could not read the measurements: {error}
                    </div>
                )}
                {groups.length > 0 && (
                    <div className="flex flex-col gap-4">
                        <h3 className="text-sm font-medium">Metrics over time</h3>
                        {groups.map((group) => (
                            <MetricGroupSection group={group} key={group.key} />
                        ))}
                    </div>
                )}
                {samples.length > 0 && <Samples samples={samples} />}
                {diffs.map((diff, index) => (
                    <Comparison
                        diff={diff}
                        key={`${diff.beforeLabel}-${diff.afterLabel}-${index}`}
                    />
                ))}
            </CardContent>
        </Card>
    );
}

function formatTime(value: string): string {
    const parsed = new Date(value);

    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

function LoadBar({ value }: { value: number }) {
    return <span>{value.toFixed(2)}</span>;
}

function Percent({ title, value }: { title?: string; value: number }) {
    const width = Math.max(0, Math.min(100, value));

    return (
        <div className="flex flex-col items-end gap-1">
            <span>{value.toFixed(1)}%</span>
            <span
                className="bg-muted h-1 w-16 overflow-hidden rounded"
                title={title}
            >
                <span
                    className="bg-primary block h-full"
                    style={{ width: `${width}%` }}
                />
            </span>
        </div>
    );
}

function Samples({ samples }: { samples: BaselineSample[] }) {
    const first = samples[0];

    if (!first) {
        return null;
    }

    return (
        <div>
            <div className="text-muted-foreground mb-2 text-xs">
                Samples — the first one is the baseline every later one is compared with
            </div>
            <Table aria-label="Performance samples">
                <TableHeader>
                    <TableRow>
                        <TableHead>Label</TableHead>
                        <TableHead>Captured</TableHead>
                        <TableHead className="text-right">Load 1m</TableHead>
                        <TableHead className="text-right">CPU</TableHead>
                        <TableHead className="text-right">Memory</TableHead>
                        <TableHead className="text-right">Worst disk</TableHead>
                        <TableHead className="text-right">Restarts</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {samples.map((sample, index) => (
                        <TableRow
                            data-testid="performance-sample"
                            key={`${sample.label}-${sample.capturedAt}-${index}`}
                        >
                            <TableCell className="font-medium">{sample.label || '—'}</TableCell>
                            <TableCell className="text-muted-foreground text-xs">
                                {formatTime(sample.capturedAt)}
                            </TableCell>
                            <TableCell className="text-right tabular-nums">
                                <LoadBar value={sample.load1} />
                            </TableCell>
                            <TableCell className="text-right tabular-nums">
                                <Percent value={sample.cpuPercent} />
                            </TableCell>
                            <TableCell className="text-right tabular-nums">
                                <Percent value={sample.memPercent} />
                            </TableCell>
                            <TableCell className="text-right tabular-nums">
                                <Percent
                                    title={sample.worstDiskPath}
                                    value={sample.worstDiskPercent}
                                />
                            </TableCell>
                            <TableCell className="text-right tabular-nums">
                                {sample.restarts}
                                {sample.restarts === first.restarts
                                    ? ''
                                    : ` (${signed(sample.restarts - first.restarts)})`}
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>
        </div>
    );
}

function signed(value: number): string {
    return value > 0 ? `+${value}` : `${value}`;
}

export default FlowPerformance;
