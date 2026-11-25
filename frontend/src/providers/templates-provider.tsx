import { skipToken, useMutation, useQuery, useSubscription } from '@apollo/client/react';
import { createContext, type ReactNode, useCallback, useContext, useMemo } from 'react';
import { toast } from 'sonner';

import {
    CreateFlowTemplateDocument,
    DeleteFlowTemplateDocument,
    FlowTemplateCreatedDocument,
    FlowTemplateDeletedDocument,
    FlowTemplatePresetsDocument,
    FlowTemplatesDocument,
    FlowTemplateUpdatedDocument,
    UpdateFlowTemplateDocument,
} from '@/graphql/types';
import { Log } from '@/lib/log';
import { useUser } from '@/providers/user-provider';

export interface Template {
    createdAt: Date;
    id: string;
    text: string;
    title: string;
    updatedAt: Date;
    userId: string;
}

/** One template this build ships: the same catalogue the server seeds into a new account. */
export interface TemplatePreset {
    domain: string;
    text: string;
    title: string;
}

interface TemplatesContextValue {
    createTemplate: (title: string, text: string) => Promise<void>;
    deleteTemplate: (id: string) => Promise<void>;
    error?: Error;
    getTemplate: (id: string) => Template | undefined;
    isLoading: boolean;
    presets: TemplatePreset[];
    refetch: () => unknown;
    templates: Template[];
    updateTemplate: (id: string, payload: { text: string; title: string }) => Promise<void>;
}

interface TemplatesProviderProps {
    children: ReactNode;
}

const TemplatesContext = createContext<TemplatesContextValue | undefined>(undefined);

export function TemplatesProvider({ children }: TemplatesProviderProps) {
    const { authInfo, isAuthenticated } = useUser();

    const shouldFetchTemplates = Boolean(authInfo && authInfo.type !== 'guest' && isAuthenticated());

    const {
        data: templatesData,
        error: templatesError,
        loading: isLoadingTemplates,
        refetch,
    } = useQuery(FlowTemplatesDocument, shouldFetchTemplates ? { fetchPolicy: 'cache-and-network' } : skipToken);

    const { data: presetsData } = useQuery(
        FlowTemplatePresetsDocument,
        shouldFetchTemplates ? { fetchPolicy: 'cache-and-network' } : skipToken,
    );

    const [createTemplateMutation] = useMutation(CreateFlowTemplateDocument);
    const [updateTemplateMutation] = useMutation(UpdateFlowTemplateDocument);
    const [deleteTemplateMutation] = useMutation(DeleteFlowTemplateDocument);

    useSubscription(FlowTemplateCreatedDocument, {
        skip: !shouldFetchTemplates,
    });

    useSubscription(FlowTemplateUpdatedDocument, {
        skip: !shouldFetchTemplates,
    });

    useSubscription(FlowTemplateDeletedDocument, {
        skip: !shouldFetchTemplates,
    });

    const templates = useMemo(() => {
        const rawTemplates = templatesData?.flowTemplates ?? [];

        return rawTemplates.map((t) => ({
            createdAt: new Date(t.createdAt),
            id: t.id,
            text: t.text,
            title: t.title,
            updatedAt: new Date(t.updatedAt),
            userId: t.userId,
        }));
    }, [templatesData?.flowTemplates]);

    const getTemplate = useCallback(
        (id: string): Template | undefined => {
            return templates.find((t) => t.id === id);
        },
        [templates],
    );

    const presets = useMemo<TemplatePreset[]>(
        () =>
            (presetsData?.flowTemplatePresets ?? []).map((preset) => ({
                domain: preset.domain,
                text: preset.text,
                title: preset.title,
            })),
        [presetsData?.flowTemplatePresets],
    );

    const createTemplate = useCallback(
        async (title: string, text: string) => {
            try {
                await createTemplateMutation({
                    variables: {
                        input: {
                            text,
                            title,
                        },
                    },
                });
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : '创建模板失败';
                toast.error('创建模板失败', {
                    description: errorMessage,
                });
                Log.error('Error creating template:', error);
                throw error;
            }
        },
        [createTemplateMutation],
    );

    const updateTemplate = useCallback(
        async (id: string, payload: { text: string; title: string }) => {
            try {
                await updateTemplateMutation({
                    variables: {
                        input: {
                            text: payload.text,
                            title: payload.title,
                        },
                        templateId: id,
                    },
                });
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : '更新模板失败';
                toast.error('更新模板失败', {
                    description: errorMessage,
                });
                Log.error('Error updating template:', error);
                throw error;
            }
        },
        [updateTemplateMutation],
    );

    const deleteTemplate = useCallback(
        async (id: string) => {
            try {
                await deleteTemplateMutation({
                    variables: {
                        templateId: id,
                    },
                });
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : '删除模板失败';
                toast.error('删除模板失败', {
                    description: errorMessage,
                });
                Log.error('Error deleting template:', error);
                throw error;
            }
        },
        [deleteTemplateMutation],
    );

    const value = useMemo(
        () => ({
            createTemplate,
            deleteTemplate,
            error: templatesError,
            getTemplate,
            isLoading: isLoadingTemplates,
            presets,
            refetch,
            templates,
            updateTemplate,
        }),
        [
            createTemplate,
            deleteTemplate,
            templatesError,
            getTemplate,
            isLoadingTemplates,
            presets,
            refetch,
            templates,
            updateTemplate,
        ],
    );

    return <TemplatesContext.Provider value={value}>{children}</TemplatesContext.Provider>;
}

export function useTemplates() {
    const context = useContext(TemplatesContext);

    if (context === undefined) {
        throw new Error('useTemplates must be used within TemplatesProvider');
    }

    return context;
}
