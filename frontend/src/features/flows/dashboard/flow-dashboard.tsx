import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FlowDashboardOverview } from '@/features/flows/dashboard/flow-dashboard-overview';
import FlowPerformance from '@/features/flows/performance/flow-performance';
import { useFlow } from '@/providers/flow-provider';

function FlowDashboard() {
    const { flowId } = useFlow();

    if (!flowId) {
        return (
            <div className="text-muted-foreground flex items-center justify-center py-12">选择一个会话以查看仪表盘</div>
        );
    }

    return (
        <Tabs defaultValue="overview">
            <TabsList className="hidden">
                <TabsTrigger value="overview">概览</TabsTrigger>
            </TabsList>

            <TabsContent value="overview">
                <div className="flex flex-col gap-4">
                    <FlowDashboardOverview flowId={flowId} />
                    <FlowPerformance flowId={flowId} />
                </div>
            </TabsContent>
        </Tabs>
    );
}

export default FlowDashboard;
