import { RefreshCw } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { Button } from '@/components/ui/button';
import { routes } from '@/lib/routes';
import { useFlow } from '@/providers/flow-provider';
import { useFlows } from '@/providers/flows-provider';

// A verdict of "no effect" or "not re-measured" is a statement about the
// measurement, not about the idea: the honest next step is to run it again with
// the same goal, which is what this button does.
const RETRYABLE = new Set(['degraded', 'no_measurable_effect', 'unverified']);

function RetryMeasurement({ verdict }: { verdict: string }) {
    const { flowData } = useFlow();
    const { createFlow } = useFlows();
    const navigate = useNavigate();
    const [isRetrying, setIsRetrying] = useState(false);

    if (!RETRYABLE.has(verdict)) {
        return null;
    }

    const input = flowData?.tasks?.[0]?.input ?? '';
    const providerName = flowData?.flow?.provider?.name ?? '';

    const handleRetry = async () => {
        if (!input || !providerName || isRetrying) {
            return;
        }

        setIsRetrying(true);

        try {
            const flowId = await createFlow({
                capabilityMode: 'off',
                message: input,
                providerName,
                resourceIds: [],
                targetType: 'optimization',
            });

            if (flowId) {
                navigate(routes.flow(flowId));
            }
        } finally {
            setIsRetrying(false);
        }
    };

    return (
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
            <Button
                disabled={!input || !providerName || isRetrying}
                onClick={handleRetry}
                size="sm"
                variant="outline"
            >
                <RefreshCw className={isRetrying ? 'animate-spin' : undefined} />
                Measure again
            </Button>
            <span className="text-muted-foreground text-xs">用相同的输入和目标新建会话</span>
        </div>
    );
}

export { RETRYABLE };
export default RetryMeasurement;
