import { FlowStatusIcon } from '@/components/icons/flow-status-icon';
import { Badge } from '@/components/ui/badge';
import { StatusType } from '@/graphql/types';

const STATUS_LABELS: Record<StatusType, string> = {
    [StatusType.Created]: '已创建',
    [StatusType.Failed]: '失败',
    [StatusType.Finished]: '已完成',
    [StatusType.Running]: '运行中',
    [StatusType.Waiting]: '等待中',
};

export function FlowStatusBadge({ className, status }: { className?: string; status: StatusType }) {
    return (
        <Badge
            className={className}
            variant="outline"
        >
            <FlowStatusIcon
                className="size-3"
                status={status}
            />
            {STATUS_LABELS[status]}
        </Badge>
    );
}
