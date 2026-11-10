import { memo, type ReactNode, useMemo, useState } from 'react';

import Markdown from '@/components/shared/markdown';
import { Badge, type BadgeProps } from '@/components/ui/badge';

export type Conclusion = DiagnosisConclusion | OptimizationConclusion;

interface DiagnosisConclusion {
    action?: string;
    blastRadius?: string;
    confidence?: number;
    evidence: EvidenceItem[];
    kind: 'diagnosis';
    needsHuman?: boolean;
    risk?: string;
    rollback?: string;
    rootCause: string;
}

interface EvidenceItem {
    excerpt?: string;
    query?: string;
    source?: string;
    timestamp?: string;
}

interface OptimizationConclusion {
    baseline?: string;
    bottleneck: string;
    confidence?: number;
    evidence: EvidenceItem[];
    expectedGain?: string;
    kind: 'optimization';
    observedGain?: string;
    proposal?: string;
    rollback?: string;
    verdict: string;
}

const JSON_FENCE = /```json\s*([\s\S]*?)```/gi;

interface RawConclusion {
    baseline?: unknown;
    blast_radius?: unknown;
    bottleneck?: unknown;
    confidence?: unknown;
    evidence?: unknown;
    expected_gain?: unknown;
    needs_human?: unknown;
    observed_gain?: unknown;
    proposal?: unknown;
    recommendation?: { action?: unknown; risk?: unknown; rollback?: unknown };
    rollback?: unknown;
    root_cause?: unknown;
    verdict?: unknown;
}

export function parseConclusion(result?: string): Conclusion | null {
    if (!result) {
        return null;
    }

    const fences = [...result.matchAll(JSON_FENCE)];
    const body = fences.at(-1)?.[1];

    if (!body) {
        return null;
    }

    let payload: RawConclusion;

    try {
        payload = JSON.parse(body) as RawConclusion;
    } catch {
        return null;
    }

    if (typeof payload !== 'object' || payload === null) {
        return null;
    }

    const rootCause = text(payload.root_cause);

    if (rootCause) {
        return {
            action: text(payload.recommendation?.action),
            blastRadius: text(payload.blast_radius),
            confidence: typeof payload.confidence === 'number' ? payload.confidence : undefined,
            evidence: evidenceOf(payload.evidence),
            kind: 'diagnosis',
            needsHuman: typeof payload.needs_human === 'boolean' ? payload.needs_human : undefined,
            risk: text(payload.recommendation?.risk),
            rollback: text(payload.recommendation?.rollback),
            rootCause,
        };
    }

    const bottleneck = text(payload.bottleneck);
    const verdict = text(payload.verdict);

    if (bottleneck && verdict) {
        return {
            baseline: text(payload.baseline),
            bottleneck,
            confidence: typeof payload.confidence === 'number' ? payload.confidence : undefined,
            evidence: evidenceOf(payload.evidence),
            expectedGain: text(payload.expected_gain),
            kind: 'optimization',
            observedGain: text(payload.observed_gain),
            proposal: text(payload.proposal),
            rollback: text(payload.rollback),
            verdict,
        };
    }

    return null;
}

function evidenceOf(value: unknown): EvidenceItem[] {
    if (!Array.isArray(value)) {
        return [];
    }

    return value.filter((item): item is EvidenceItem => typeof item === 'object' && item !== null);
}

function text(value: unknown): string | undefined {
    return typeof value === 'string' && value.trim() ? value : undefined;
}

const VERDICT_LABELS: Record<string, string> = {
    degraded: 'Degraded',
    improved: 'Improved',
    no_measurable_effect: 'No measurable effect',
    unverified: 'Not re-measured',
};

const VERDICT_VARIANTS: Record<string, BadgeProps['variant']> = {
    degraded: 'red',
    improved: 'green',
    no_measurable_effect: 'yellow',
    unverified: 'secondary',
};

interface FlowMessageConclusionProps {
    conclusion: Conclusion;
    // Rendered inside the card so an action on the conclusion reads as part of it.
    footer?: ReactNode;
    raw: string;
    searchValue?: string;
}

function EvidenceList({ items }: { items: EvidenceItem[] }) {
    if (!items.length) {
        return null;
    }

    return (
        <div className="mt-3">
            <div className="text-muted-foreground mb-1 text-xs">Evidence ({items.length})</div>
            <ol className="flex flex-col gap-2">
                {items.map((item, index) => (
                    <li
                        className="rounded-md border p-2 text-xs"
                        key={`${item.source ?? 'evidence'}-${index}`}
                    >
                        <div className="flex flex-wrap items-center gap-2">
                            {item.source && <Badge variant="secondary">{item.source}</Badge>}
                            {item.timestamp && <span className="text-muted-foreground">{item.timestamp}</span>}
                        </div>
                        {item.query && (
                            <code className="text-muted-foreground mt-1 block font-mono break-all">{item.query}</code>
                        )}
                        {item.excerpt && (
                            <blockquote className="border-muted-foreground/30 mt-1 border-l-2 pl-2 whitespace-pre-wrap">
                                {item.excerpt}
                            </blockquote>
                        )}
                    </li>
                ))}
            </ol>
        </div>
    );
}

function FlowMessageConclusion({ conclusion, footer, raw, searchValue = '' }: FlowMessageConclusionProps) {
    const [isRawVisible, setIsRawVisible] = useState(false);

    const confidence = useMemo(() => {
        if (conclusion.confidence === undefined) {
            return null;
        }

        return `${Math.round(conclusion.confidence * 100)}%`;
    }, [conclusion.confidence]);

    return (
        <div
            className="flex flex-col gap-2"
            data-testid="flow-message-conclusion"
        >
            <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold">
                    {conclusion.kind === 'diagnosis' ? 'Diagnosis' : 'Optimization'}
                </span>
                {conclusion.kind === 'optimization' && (
                    <Badge variant={VERDICT_VARIANTS[conclusion.verdict] ?? 'secondary'}>
                        {VERDICT_LABELS[conclusion.verdict] ?? conclusion.verdict}
                    </Badge>
                )}
                {confidence && <Badge variant="outline">confidence {confidence}</Badge>}
                {conclusion.kind === 'diagnosis' && conclusion.needsHuman && (
                    <Badge variant="orange">需要人工介入</Badge>
                )}
                <button
                    className="text-muted-foreground ml-auto cursor-pointer text-xs hover:underline"
                    onClick={() => setIsRawVisible((previous) => !previous)}
                    type="button"
                >
                    {isRawVisible ? 'Hide report' : 'Show report'}
                </button>
            </div>

            {isRawVisible ? (
                <Markdown
                    className="prose-xs prose-fixed wrap-break-word"
                    searchValue={searchValue}
                >
                    {raw}
                </Markdown>
            ) : (
                <>
                    {conclusion.kind === 'diagnosis' ? (
                        <>
                            <Row
                                label="根因"
                                value={conclusion.rootCause}
                            />
                            <Row
                                label="影响面"
                                value={conclusion.blastRadius}
                            />
                            <Row
                                label="下一步动作"
                                value={conclusion.action}
                            />
                            <Row
                                label="风险"
                                value={conclusion.risk}
                            />
                            <Row
                                label="回滚"
                                value={conclusion.rollback}
                            />
                        </>
                    ) : (
                        <>
                            <Row
                                label="瓶颈"
                                value={conclusion.bottleneck}
                            />
                            <Row
                                label="基线"
                                value={conclusion.baseline}
                            />
                            <Row
                                label="建议"
                                value={conclusion.proposal}
                            />
                            <Row
                                label="预期收益"
                                value={conclusion.expectedGain}
                            />
                            <Row
                                label="实测收益"
                                value={conclusion.observedGain ?? 'not re-measured'}
                            />
                            <Row
                                label="回滚"
                                value={conclusion.rollback}
                            />
                        </>
                    )}
                    <EvidenceList items={conclusion.evidence} />
                </>
            )}
            {footer}
        </div>
    );
}

function Row({ label, value }: { label: string; value?: string }) {
    if (!value) {
        return null;
    }

    return (
        <div className="grid grid-cols-[7.5rem_1fr] gap-2 text-sm">
            <span className="text-muted-foreground">{label}</span>
            <span className="wrap-break-word whitespace-pre-wrap">{value}</span>
        </div>
    );
}

export default memo(FlowMessageConclusion);
