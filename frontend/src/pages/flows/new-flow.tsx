import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { AppHeader, AppHeaderContent, AppHeaderTitle } from '@/components/layouts/app/app-header';
import { Card, CardContent } from '@/components/ui/card';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FlowForm, type FlowFormValues } from '@/features/flows/flow-form';
import { routes } from '@/lib/routes';
import { useFlows } from '@/providers/flows-provider';
import { useProviders } from '@/providers/providers-provider';

function NewFlow() {
    const navigate = useNavigate();

    const { selectedProvider } = useProviders();
    const { createFlow, createFlowWithAssistant } = useFlows();

    const [isLoading, setIsLoading] = useState(false);
    const [flowType, setFlowType] = useState<'assistant' | 'automation'>('automation');

    const handleSubmit = async (values: FlowFormValues) => {
        if (isLoading) {
            return false;
        }

        setIsLoading(true);

        try {
            const flowId = flowType === 'automation' ? await createFlow(values) : await createFlowWithAssistant(values);

            if (!flowId) {
                return false;
            }

            navigate(routes.flow(flowId, { tab: flowType }));

            return true;
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <>
            <AppHeader>
                <AppHeaderContent>
                    <AppHeaderTitle>新建会话</AppHeaderTitle>
                </AppHeaderContent>
            </AppHeader>
            <div className="flex min-h-[calc(100dvh-3rem)] items-center justify-center p-4">
                <Card className="w-full max-w-2xl">
                    <CardContent className="flex flex-col gap-4 pt-6">
                        <div className="flex flex-col gap-2 text-center">
                            <h2 className="text-2xl font-semibold">新建会话</h2>
                            <p className="text-muted-foreground">描述要诊断的故障，或要优化的服务</p>
                        </div>
                        <Tabs
                            onValueChange={(value) => setFlowType(value as 'assistant' | 'automation')}
                            value={flowType}
                        >
                            <TabsList className="grid w-full grid-cols-2">
                                <TabsTrigger
                                    disabled={isLoading}
                                    value="automation"
                                >
                                    Automation
                                </TabsTrigger>
                                <TabsTrigger
                                    disabled={isLoading}
                                    value="assistant"
                                >
                                    Assistant
                                </TabsTrigger>
                            </TabsList>
                        </Tabs>
                        <FlowForm
                            defaultValues={{
                                providerName: selectedProvider?.name ?? '',
                            }}
                            isSubmitting={isLoading}
                            onSubmit={handleSubmit}
                            placeholder={
                                !isLoading
                                    ? flowType === 'automation'
                                        ? 'Describe the incident to diagnose, or the service to optimise...'
                                        : 'What would you like me to help you with?'
                                    : 'Creating a new flow...'
                            }
                        />
                    </CardContent>
                </Card>
            </div>
        </>
    );
}

export default NewFlow;
