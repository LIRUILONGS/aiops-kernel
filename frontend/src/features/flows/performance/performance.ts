// Reading a performance result out of the tool calls that produced it.
//
// The conclusion card shows what an agent *said* about a measurement; these
// parsers read the measurement itself. ops_baseline answers with a short prose
// block and one JSON fingerprint line, ops_diff with a tab-separated table --
// both were written to be read by a model, so the numbers have to be lifted out
// of text rather than taken from a typed payload.

export interface BaselineSample {
    capturedAt: string;
    cpuPercent: number;
    label: string;
    load1: number;
    memPercent: number;
    memTotalMB: number;
    memUsedMB: number;
    restarts: number;
    worstDiskPath: string;
    worstDiskPercent: number;
}

export interface DeltaRow {
    after: string;
    before: string;
    change: string;
    field: string;
    verdict: string;
    // The text table marks a row the agent should look at twice, and today the only
    // mark it carries is a worsening arrow; the count of material rows comes from
    // the header instead, where ops_diff states it outright.
    worsened: boolean;
}

export interface DiffReport {
    afterLabel: string;
    afterTime: string;
    beforeLabel: string;
    beforeTime: string;
    material: number;
    rows: DeltaRow[];
    summary: string;
    total: number;
}

interface RawFingerprint {
    captured_at?: unknown;
    containers?: Record<string, { restarts?: unknown }>;
    cpu_percent?: unknown;
    filesystems?: Record<string, unknown>;
    label?: unknown;
    load1?: unknown;
    mem_total_mb?: unknown;
    mem_used_mb?: unknown;
}

const HEADER = /^对比：(.+?)（(.+?)）→ (.+?)（(.+?)）$/;
const COUNT = /^共 (\d+) 项，其中显著变化 (\d+) 项$/;

export function parseBaseline(result?: string): BaselineSample | null {
    if (!result) {
        return null;
    }

    // The fingerprint is the last JSON object in the answer: everything before it
    // is prose written for a human.
    const lines = result.split(/\r?\n/);

    for (let index = lines.length - 1; index >= 0; index -= 1) {
        const line = (lines[index] ?? '').trim();

        if (!line.startsWith('{')) {
            continue;
        }

        try {
            const parsed = JSON.parse(line) as RawFingerprint;

            if (parsed.captured_at) {
                return toSample(parsed);
            }
        } catch {
            // A truncated fingerprint is no evidence; try the line above.
        }
    }

    return null;
}

export function parseDiff(result?: string): DiffReport | null {
    if (!result) {
        return null;
    }

    const lines = result.split(/\r?\n/);
    const header = lines.find((line) => HEADER.test(line.trim()));
    const counts = lines.find((line) => COUNT.test(line.trim()));

    if (!header || !counts) {
        return null;
    }

    const headerMatch = HEADER.exec(header.trim()) ?? [];
    const countMatch = COUNT.exec(counts.trim()) ?? [];
    const beforeLabel = headerMatch[1] ?? '';
    const beforeTime = headerMatch[2] ?? '';
    const afterLabel = headerMatch[3] ?? '';
    const afterTime = headerMatch[4] ?? '';
    const total = countMatch[1] ?? '';
    const material = countMatch[2] ?? '';

    const rows: DeltaRow[] = [];
    const summary: string[] = [];
    let inTable = false;

    for (const line of lines) {
        const trimmed = line.trim();

        if (!trimmed) {
            continue;
        }

        if (trimmed.startsWith('指标\t')) {
            inTable = true;
            continue;
        }

        if (trimmed.startsWith('对比：') || COUNT.test(trimmed)) {
            continue;
        }

        if (inTable) {
            const cells = trimmed.split('\t');

            if (cells.length >= 5) {
                const [field = '', before = '', after = '', change = '', verdictCell = ''] = cells;

                rows.push({
                    after,
                    before,
                    change,
                    field,
                    verdict: verdictCell.replace('←', '').trim(),
                    worsened: verdictCell.endsWith('←'),
                });
                continue;
            }

            // The summary paragraph follows the table and has no tabs.
            summary.push(trimmed);
        }
    }

    return {
        afterLabel,
        afterTime,
        beforeLabel,
        beforeTime,
        material: Number(material ?? 0),
        rows,
        summary: summary.join(' '),
        total: Number(total ?? rows.length),
    };
}

function number(value: unknown): number {
    if (typeof value === 'number') {
        return value;
    }

    const parsed = Number(value);

    return Number.isFinite(parsed) ? parsed : 0;
}

function toSample(raw: RawFingerprint): BaselineSample {
    const filesystems = raw.filesystems ?? {};
    let worstDiskPercent = 0;
    let worstDiskPath = '';

    for (const [path, value] of Object.entries(filesystems)) {
        const percent = typeof value === 'number' ? value : Number(value);

        if (Number.isFinite(percent) && percent >= worstDiskPercent) {
            worstDiskPercent = percent;
            worstDiskPath = path;
        }
    }

    let restarts = 0;

    for (const container of Object.values(raw.containers ?? {})) {
        const count = typeof container?.restarts === 'number' ? container.restarts : 0;

        restarts += count;
    }

    const memTotalMB = number(raw.mem_total_mb);
    const memUsedMB = number(raw.mem_used_mb);

    return {
        capturedAt: String(raw.captured_at ?? ''),
        cpuPercent: number(raw.cpu_percent),
        label: String(raw.label ?? ''),
        load1: number(raw.load1),
        memPercent: memTotalMB > 0 ? (memUsedMB / memTotalMB) * 100 : 0,
        memTotalMB,
        memUsedMB,
        restarts,
        worstDiskPath,
        worstDiskPercent,
    };
}

export const VERDICT_TONE: Record<string, 'green' | 'red' | 'secondary' | 'yellow'> = {
    恶化: 'red',
    持平: 'secondary',
    改善: 'green',
};
