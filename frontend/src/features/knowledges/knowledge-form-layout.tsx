import type { Control } from 'react-hook-form';

import type { KnowledgeDocumentFragmentFragment } from '@/graphql/types';

import { DetailSplitLayout } from '@/components/shared/detail-split-layout';
import { type EditorViewMode } from '@/components/shared/markdown-editor';
import { Badge } from '@/components/ui/badge';

import type { FormValues } from './knowledge-form';

import { KnowledgeContentField, KnowledgeMetaFields } from './knowledge-form-controls';

interface KnowledgeFormLayoutProps {
    control: Control<FormValues>;
    isNew: boolean;
    isSaving: boolean;
    knowledge?: KnowledgeDocumentFragmentFragment | null;
    viewMode?: EditorViewMode;
}

interface KnowledgeIntroBlockProps {
    isNew: boolean;
    knowledge?: KnowledgeDocumentFragmentFragment | null;
}

export function KnowledgeFormLayoutDesktop({
    control,
    isNew,
    isSaving,
    knowledge,
    viewMode,
}: KnowledgeFormLayoutProps) {
    return (
        <DetailSplitLayout
            content={
                <KnowledgeContentField
                    control={control}
                    fillParent
                    isSaving={isSaving}
                    viewMode={viewMode}
                />
            }
            contentClassName="flex h-full min-h-0 flex-col overflow-hidden p-4"
            panel={
                <>
                    <KnowledgeIntroBlock
                        isNew={isNew}
                        knowledge={knowledge}
                    />
                    <KnowledgeMetaFields
                        control={control}
                        isNew={isNew}
                        isSaving={isSaving}
                    />
                </>
            }
        />
    );
}

export function KnowledgeFormLayoutMobile({ control, isNew, isSaving, knowledge, viewMode }: KnowledgeFormLayoutProps) {
    return (
        <div className="flex min-h-0 flex-1 flex-col gap-4 p-4">
            <KnowledgeIntroBlock
                isNew={isNew}
                knowledge={knowledge}
            />
            <KnowledgeMetaFields
                control={control}
                isNew={isNew}
                isSaving={isSaving}
            />
            <KnowledgeContentField
                control={control}
                hasLabel
                isSaving={isSaving}
                viewMode={viewMode}
            />
        </div>
    );
}

function KnowledgeIntroBlock({ isNew, knowledge }: KnowledgeIntroBlockProps) {
    return (
        <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2 text-center">
                <h2 className="text-2xl font-semibold">{isNew ? '新建知识文档' : '编辑知识文档'}</h2>
                <p className="text-muted-foreground">
                    {isNew ? '向向量知识库添加一条记录' : '修改内容或元数据会触发重新嵌入'}
                </p>
            </div>

            {!isNew && knowledge ? (
                <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
                    <Badge variant={knowledge.manual ? 'secondary' : 'outline'}>
                        {knowledge.manual ? '手动' : 'Agent'}
                    </Badge>
                    {knowledge.flowId ? <Badge variant="outline">会话 #{knowledge.flowId}</Badge> : null}
                    {knowledge.taskId ? <Badge variant="outline">任务 #{knowledge.taskId}</Badge> : null}
                    {knowledge.subtaskId ? <Badge variant="outline">子任务 #{knowledge.subtaskId}</Badge> : null}
                    <span>·</span>
                    <span>
                        分块 {knowledge.partSize} / {knowledge.totalSize}
                    </span>
                </div>
            ) : null}
        </div>
    );
}
