import { useMutation, useQuery } from '@apollo/client/react';
import { ChevronDown, ChevronRight, ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';

import type {
    CapabilityDecisionFragmentFragment,
    CapabilityLeaseFragmentFragment,
    CapabilityRequestFragmentFragment,
} from '@/graphql/types';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Spinner } from '@/components/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
    CapabilityMode,
    CapabilityRequestStatus,
    CapabilityVerdict,
    DecideCapabilityRequestDocument,
    FlowCapabilityDecisionsDocument,
    FlowCapabilityLeasesDocument,
    FlowCapabilityRequestsDocument,
    StatusType,
} from '@/graphql/types';
import { cn } from '@/lib/utils';
import { formatName } from '@/lib/utils/format';
import { useFlow } from '@/providers/flow-provider';

const modeHints: Record<CapabilityMode, string> = {
    [CapabilityMode.Enforce]: '租约外的调用被拦下，并把拦截原因交回给模型。',
    [CapabilityMode.Off]: '租约关闭：不签发、不检查、不记录，取证工具与未开启时完全一致。',
    [CapabilityMode.Shadow]: '只记录不拦截：越界的调用照常执行，在这里记为“将被拦截”。',
};

const verdictLabels: Record<CapabilityVerdict, string> = {
    [CapabilityVerdict.Allow]: '放行',
    [CapabilityVerdict.Deny]: '拦截',
    [CapabilityVerdict.WouldDeny]: '将被拦截',
};

const verdictStyles: Record<CapabilityVerdict, string> = {
    [CapabilityVerdict.Allow]: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
    [CapabilityVerdict.Deny]: 'border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400',
    [CapabilityVerdict.WouldDeny]: 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
};

const filters: { label: string; value?: CapabilityVerdict }[] = [
    { label: '全部' },
    { label: '拦截', value: CapabilityVerdict.Deny },
    { label: '将被拦截', value: CapabilityVerdict.WouldDeny },
    { label: '放行', value: CapabilityVerdict.Allow },
];

interface FlowCapabilitySheetProps {
    mode: CapabilityMode;
    onOpenChange: (open: boolean) => void;
    open: boolean;
}

const formatTime = (value: string): string => {
    const parsed = new Date(value);

    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
};

const decisionTitle = (decision: CapabilityDecisionFragmentFragment): string =>
    decision.command ? `${decision.tool} · ${decision.command}` : decision.tool;

const requestTitle = (request: CapabilityRequestFragmentFragment): string =>
    request.command ? `${request.tool} · ${request.command}` : request.tool;

const scopeSummary = (lease: CapabilityLeaseFragmentFragment): string => {
    const parts = [`主机 ${lease.targets.join('、') || '无'}`, `工具 ${lease.tools.length} 个`];

    if (lease.actions.length > 0) {
        parts.push(`写操作 ${lease.actions.join('、')}`);
    }

    if (lease.shapes.length > 0) {
        parts.push(`形状 ${lease.shapes.join('、')}`);
    }

    return parts.join(' · ');
};

const expiresIn = (value: string): string => {
    const parsed = new Date(value).getTime();

    if (Number.isNaN(parsed)) {
        return value;
    }

    const minutes = Math.round((parsed - Date.now()) / 60_000);

    if (minutes <= 0) {
        return '已过期';
    }

    if (minutes < 60) {
        return `${minutes} 分钟后到期`;
    }

    return `${Math.round(minutes / 60)} 小时后到期`;
};

function FlowCapabilitySheet({ mode, onOpenChange, open }: FlowCapabilitySheetProps) {
    const { flowId, flowStatus } = useFlow();
    const [verdict, setVerdict] = useState<CapabilityVerdict | undefined>();
    const [deciding, setDeciding] = useState<string | undefined>();
    const [decideError, setDecideError] = useState<string | undefined>();

    const isLive = !!flowStatus && [StatusType.Created, StatusType.Running, StatusType.Waiting].includes(flowStatus);

    const pollInterval = isLive ? 5_000 : 0;

    const { data, loading } = useQuery(FlowCapabilityDecisionsDocument, {
        pollInterval,
        skip: !open || !flowId,
        variables: { flowId: flowId ?? '', verdict },
    });

    const { data: requestData } = useQuery(FlowCapabilityRequestsDocument, {
        pollInterval,
        skip: !open || !flowId,
        variables: { flowId: flowId ?? '', status: CapabilityRequestStatus.Pending },
    });

    const { data: leaseData } = useQuery(FlowCapabilityLeasesDocument, {
        pollInterval,
        skip: !open || !flowId,
        variables: { flowId: flowId ?? '', limit: 20 },
    });

    const [decide] = useMutation(DecideCapabilityRequestDocument);

    const decisions = useMemo(
        () => (data?.capabilityDecisions ?? []) as CapabilityDecisionFragmentFragment[],
        [data?.capabilityDecisions],
    );

    const pending = useMemo(
        () => (requestData?.capabilityRequests ?? []) as CapabilityRequestFragmentFragment[],
        [requestData?.capabilityRequests],
    );

    const leases = useMemo(
        () => (leaseData?.capabilityLeases ?? []) as CapabilityLeaseFragmentFragment[],
        [leaseData?.capabilityLeases],
    );

    const decideRequest = async (request: CapabilityRequestFragmentFragment, approve: boolean): Promise<void> => {
        setDeciding(request.id);
        setDecideError(undefined);

        try {
            await decide({
                refetchQueries: [FlowCapabilityDecisionsDocument, FlowCapabilityRequestsDocument],
                variables: { approve, requestId: request.id },
            });
        } catch (error) {
            setDecideError(error instanceof Error ? error.message : String(error));
        } finally {
            setDeciding(undefined);
        }
    };

    return (
        <Sheet
            onOpenChange={onOpenChange}
            open={open}
        >
            <SheetContent
                className="flex w-full flex-col gap-3 sm:max-w-xl"
                side="right"
            >
                <SheetHeader>
                    <SheetTitle className="flex items-center gap-2">
                        <ShieldCheck className="size-4" />
                        能力租约
                    </SheetTitle>
                    <SheetDescription>{modeHints[mode]}</SheetDescription>
                </SheetHeader>

                {pending.length > 0 && (
                    <section className="flex flex-col gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2.5">
                        <div className="flex items-center justify-between">
                            <span className="text-xs font-medium">待批准（{pending.length}）</span>
                        </div>
                        <ul className="flex flex-col gap-2">
                            {pending.map((request) => (
                                <li
                                    className="bg-background rounded-md border p-2 text-xs"
                                    key={request.id}
                                >
                                    <div className="flex items-center gap-2">
                                        <span className="min-w-0 flex-1 truncate font-mono">
                                            {requestTitle(request)}
                                        </span>
                                    </div>
                                    <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5">
                                        <span>{formatName(request.role)}</span>
                                        {request.target && <span className="font-mono">{request.target}</span>}
                                        {request.subtaskId && <span>子任务 #{request.subtaskId}</span>}
                                    </div>
                                    {request.reason && (
                                        <p className="mt-1 leading-relaxed break-words">{request.reason}</p>
                                    )}
                                    <div className="mt-2 flex items-center justify-end gap-2">
                                        <Button
                                            disabled={!!deciding}
                                            onClick={() => decideRequest(request, false)}
                                            size="sm"
                                            variant="ghost"
                                        >
                                            拒绝
                                        </Button>
                                        <Button
                                            disabled={!!deciding}
                                            onClick={() => decideRequest(request, true)}
                                            size="sm"
                                        >
                                            {deciding === request.id ? '提交中…' : '批准 4 小时'}
                                        </Button>
                                    </div>
                                </li>
                            ))}
                        </ul>
                        {decideError && <p className="text-destructive text-xs">{decideError}</p>}
                    </section>
                )}

                <Tabs
                    className="flex min-h-0 flex-1 flex-col"
                    defaultValue="decisions"
                >
                    <TabsList className="self-start">
                        <TabsTrigger value="decisions">判定（{decisions.length}）</TabsTrigger>
                        <TabsTrigger value="leases">租约（{leases.length}）</TabsTrigger>
                    </TabsList>

                    <TabsContent
                        className="mt-2 flex min-h-0 flex-1 flex-col gap-2"
                        value="decisions"
                    >
                        <div className="flex flex-wrap items-center gap-1.5">
                            {filters.map((filter) => (
                                <button
                                    className={cn(
                                        'rounded-full border px-2.5 py-1 text-xs transition-colors',
                                        verdict === filter.value
                                            ? 'border-primary bg-primary/10 text-primary'
                                            : 'text-muted-foreground hover:bg-muted',
                                    )}
                                    key={filter.label}
                                    onClick={() => setVerdict(filter.value)}
                                    type="button"
                                >
                                    {filter.label}
                                </button>
                            ))}
                        </div>

                        {loading && !data ? (
                            <div className="flex flex-1 items-center justify-center">
                                <Spinner />
                            </div>
                        ) : decisions.length === 0 ? (
                            <Empty className="flex-1">
                                <EmptyHeader>
                                    <EmptyMedia variant="icon">
                                        <ShieldCheck />
                                    </EmptyMedia>
                                    <EmptyTitle>这个会话还没有判定记录</EmptyTitle>
                                    <EmptyDescription>
                                        {mode === CapabilityMode.Off
                                            ? '租约关闭时不写任何记录。'
                                            : '待下一次子任务调用取证工具后出现。'}
                                    </EmptyDescription>
                                </EmptyHeader>
                            </Empty>
                        ) : (
                            <ScrollArea className="min-h-0 flex-1 pr-3">
                                <ul className="flex flex-col gap-2">
                                    {decisions.map((decision) => (
                                        <li
                                            className="rounded-md border p-2.5 text-xs"
                                            key={decision.id}
                                        >
                                            <div className="flex items-center gap-2">
                                                <Badge
                                                    className={cn(
                                                        'shrink-0 border text-[10px]',
                                                        verdictStyles[decision.verdict],
                                                    )}
                                                    variant="outline"
                                                >
                                                    {verdictLabels[decision.verdict]}
                                                </Badge>
                                                <span className="min-w-0 flex-1 truncate font-mono">
                                                    {decisionTitle(decision)}
                                                </span>
                                                <span className="text-muted-foreground shrink-0">
                                                    {formatTime(decision.createdAt)}
                                                </span>
                                            </div>
                                            <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5">
                                                <span>{formatName(decision.role)}</span>
                                                {decision.target && (
                                                    <span className="font-mono">{decision.target}</span>
                                                )}
                                                {decision.subtaskId && <span>子任务 #{decision.subtaskId}</span>}
                                            </div>
                                            {decision.reason && (
                                                <p className="mt-1 leading-relaxed break-words">{decision.reason}</p>
                                            )}
                                        </li>
                                    ))}
                                </ul>
                            </ScrollArea>
                        )}
                    </TabsContent>

                    <TabsContent
                        className="mt-2 flex min-h-0 flex-1 flex-col"
                        value="leases"
                    >
                        {leases.length === 0 ? (
                            <Empty className="flex-1">
                                <EmptyHeader>
                                    <EmptyMedia variant="icon">
                                        <ShieldCheck />
                                    </EmptyMedia>
                                    <EmptyTitle>还没有签发租约</EmptyTitle>
                                    <EmptyDescription>子任务开始时签发一张，关闭租约的会话不会签发。</EmptyDescription>
                                </EmptyHeader>
                            </Empty>
                        ) : (
                            <ScrollArea className="min-h-0 flex-1 pr-3">
                                <ul className="flex flex-col gap-2">
                                    {leases.map((lease) => (
                                        <LeaseRow
                                            key={lease.id}
                                            lease={lease}
                                        />
                                    ))}
                                </ul>
                            </ScrollArea>
                        )}
                    </TabsContent>
                </Tabs>
            </SheetContent>
        </Sheet>
    );
}

function LeaseRow({ lease }: { lease: CapabilityLeaseFragmentFragment }) {
    const [expanded, setExpanded] = useState(false);

    const limits = useMemo(() => new Map(lease.budgets.map((budget) => [budget.tool, budget.limit])), [lease.budgets]);

    const remaining = expiresIn(lease.expiresAt);
    const used = lease.used.filter((use) => use.used > 0);
    const lines = used.length > 0 ? used : lease.budgets.map((budget) => ({ tool: budget.tool, used: 0 }));

    return (
        <li className="rounded-md border p-2.5 text-xs">
            <button
                className="flex w-full items-center gap-2 text-left"
                onClick={() => setExpanded((value) => !value)}
                type="button"
            >
                {expanded ? (
                    <ChevronDown className="size-3.5 shrink-0" />
                ) : (
                    <ChevronRight className="size-3.5 shrink-0" />
                )}
                <span className="min-w-0 flex-1 truncate">
                    <span className="font-mono">子任务 #{lease.subtaskId}</span>
                    <span className="text-muted-foreground"> · {formatName(lease.role)}</span>
                </span>
                <span className="text-muted-foreground shrink-0">{formatTime(lease.createdAt)}</span>
            </button>
            <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <Badge
                    className="shrink-0 border text-[10px]"
                    variant="outline"
                >
                    {lease.planMode}
                </Badge>
                <span>{remaining}</span>
            </div>
            {expanded && (
                <div className="mt-2 flex flex-col gap-1.5">
                    <p className="leading-relaxed break-words">{scopeSummary(lease)}</p>
                    <p className="font-mono leading-relaxed break-words">{lease.commands.join('、') || '无命令'}</p>
                    {lines.length > 0 && (
                        <div className="flex flex-col gap-0.5">
                            {lines.map((line) => {
                                const limit = limits.get(line.tool);

                                return (
                                    <div
                                        className="flex items-center gap-2"
                                        key={line.tool}
                                    >
                                        <span className="font-mono">{line.tool}</span>
                                        <span
                                            className={cn(
                                                limit !== undefined &&
                                                    line.used >= limit &&
                                                    'text-rose-600 dark:text-rose-400',
                                            )}
                                        >
                                            {line.used}
                                            {limit !== undefined ? ` / ${limit}` : ''}
                                        </span>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            )}
        </li>
    );
}

export default FlowCapabilitySheet;
