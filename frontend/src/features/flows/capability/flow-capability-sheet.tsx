import { useQuery } from '@apollo/client/react';
import { ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';

import type { CapabilityDecisionFragmentFragment } from '@/graphql/types';

import { Badge } from '@/components/ui/badge';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Spinner } from '@/components/ui/spinner';
import { CapabilityMode, CapabilityVerdict, FlowCapabilityDecisionsDocument, StatusType } from '@/graphql/types';
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

function FlowCapabilitySheet({ mode, onOpenChange, open }: FlowCapabilitySheetProps) {
    const { flowId, flowStatus } = useFlow();
    const [verdict, setVerdict] = useState<CapabilityVerdict | undefined>();

    const isLive = !!flowStatus && [StatusType.Created, StatusType.Running, StatusType.Waiting].includes(flowStatus);

    const { data, loading } = useQuery(FlowCapabilityDecisionsDocument, {
        pollInterval: isLive ? 5_000 : 0,
        skip: !open || !flowId,
        variables: { flowId: flowId ?? '', verdict },
    });

    const decisions = useMemo(
        () => (data?.capabilityDecisions ?? []) as CapabilityDecisionFragmentFragment[],
        [data?.capabilityDecisions],
    );

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
                                        {decision.target && <span className="font-mono">{decision.target}</span>}
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
            </SheetContent>
        </Sheet>
    );
}

export default FlowCapabilitySheet;
