import type { ReactNode } from 'react';

import { skipToken, useQuery } from '@apollo/client/react';
import { ChevronDown, Ellipsis, FileSymlink, FileText, LayoutTemplate, Pencil, Save, Trash } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { z } from 'zod';

import { AppHeader, AppHeaderAction, AppHeaderActions, AppHeaderContent } from '@/components/layouts/app/app-header';
import ConfirmationDialog from '@/components/shared/confirmation-dialog';
import {
    DetailNavigationButtons,
    DetailNavigationSheet,
    DetailNavigationToolbar,
} from '@/components/shared/detail-navigation';
import { DetailSplitLayout } from '@/components/shared/detail-split-layout';
import { ErrorState } from '@/components/shared/error-state';
import { InlineEditInput, useInlineEdit } from '@/components/shared/inline-edit';
import { type EditorViewMode, EditorViewModeToggle, MarkdownEditorField } from '@/components/shared/markdown-editor';
import { UnsavedChangesDialog, useUnsavedChangesGuard } from '@/components/shared/unsaved-changes';
import { Badge } from '@/components/ui/badge';
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Spinner } from '@/components/ui/spinner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useTemplateDetailNavigation } from '@/features/templates/use-template-detail-navigation';
import { FlowTemplateDocument } from '@/graphql/types';
import { useAppForm } from '@/hooks/use-app-form';
import { useBreakpoint } from '@/hooks/use-breakpoint';
import { isNotFoundError } from '@/lib/errors';
import { routes } from '@/lib/routes';
import { cn } from '@/lib/utils';
import { type Template, type TemplatePreset, useTemplates } from '@/providers/templates-provider';

// Mirrors the limits in backend/pkg/graph/context.go.
const MAX_TEMPLATE_TITLE_LENGTH = 255;
const MAX_TEMPLATE_TEXT_LENGTH = 65536;

export const formSchema = z.object({
    // The text is a document: it is sent as it was written, the line break it ends with included, and the
    // limit counts all of it, as the mutation does.
    text: z
        .string()
        .refine((value) => value.trim() !== '', { message: '请输入正文' })
        .refine((value) => [...value].length <= MAX_TEMPLATE_TEXT_LENGTH, {
            message: `正文不能超过 ${MAX_TEMPLATE_TEXT_LENGTH} 个字符`,
        }),
    title: z
        .string()
        .trim()
        .min(1, { message: '请输入标题' })
        .refine((value) => [...value].length <= MAX_TEMPLATE_TITLE_LENGTH, {
            message: `标题不能超过 ${MAX_TEMPLATE_TITLE_LENGTH} 个字符`,
        }),
});

type FormValues = z.infer<typeof formSchema>;

const PRESETS_TITLE = '内置模板';

const renderTemplateItem = (item: Template, isCurrent: boolean): ReactNode => (
    <span className={cn('min-w-0 flex-1 truncate', isCurrent && 'font-medium')}>{item.title}</span>
);

// One React element serves every `/templates/:templateId`, so without a key the form instance — which sets
// `keepDirtyValues` so a subscription resync cannot wipe an unsaved body — carried one template's edited text
// onto the next template and Save wrote it to the wrong row. Keying by id gives each entity its own form, the
// way knowledge.tsx already keys <KnowledgeForm>. It also stops `/templates/new` inheriting an abandoned draft.
function Template() {
    const { templateId } = useParams<{ templateId?: string }>();

    return (
        <TemplateForm
            key={templateId ?? 'new'}
            templateId={templateId}
        />
    );
}

function TemplateForm({ templateId }: { templateId?: string }) {
    const navigate = useNavigate();
    const { createTemplate, deleteTemplate, presets, updateTemplate } = useTemplates();

    const { isDesktop, isMobile } = useBreakpoint();
    const isNew = templateId === 'new';

    const templateNav = useTemplateDetailNavigation(isNew ? null : templateId);

    const [expandedPresetIndex, setExpandedPresetIndex] = useState<null | number>(null);
    const [isPresetsOpen, setIsPresetsOpen] = useState(false);
    const [isReplaceConfirmOpen, setIsReplaceConfirmOpen] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [pendingPreset, setPendingPreset] = useState<null | TemplatePreset>(null);
    const [isRenaming, setIsRenaming] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);
    const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
    const [viewMode, setViewMode] = useState<EditorViewMode>('rich');

    const {
        handleDropdownCloseAutoFocus,
        inputRef: editingInputRef,
        isEditing: isEditingTitle,
        startEdit: handleTemplateRenameStart,
        stopEdit: handleTemplateRenameCancel,
    } = useInlineEdit({ resetKey: templateId });

    const {
        data: templateData,
        error: templateError,
        loading: isLoadingTemplate,
        refetch: refetchTemplate,
    } = useQuery(FlowTemplateDocument, templateId && !isNew ? { variables: { templateId } } : skipToken);

    const template = templateData?.flowTemplate;
    // A real load failure that left nothing to show, as opposed to a genuine not-found: the page
    // renders it as an in-page ErrorState + Retry instead of the "not found" card. Mirrors flow.
    const templateLoadError = templateError && !template && !isNotFoundError(templateError) ? templateError : undefined;

    // `values` re-syncs the form whenever the cache refreshes (an inline rename, a refetch), while
    // `keepDirtyValues` preserves the user's in-flight edits — without it an external re-emit would
    // silently wipe an unsaved body. Mirrors knowledge-form.
    const initialValues = useMemo<FormValues>(
        () => ({
            text: templateData?.flowTemplate?.text ?? '',
            title: templateData?.flowTemplate?.title ?? '',
        }),
        [templateData?.flowTemplate],
    );

    const form = useAppForm<FormValues>({
        defaultValues: initialValues,
        resetOptions: { keepDirtyValues: true },
        schema: formSchema,
        values: initialValues,
    });

    const { control, formState, getValues, handleSubmit: handleFormSubmit, reset, setValue } = form;
    const { isDirty, isValid } = formState;

    const hasUnsavedChanges = isDirty;
    const templateName = templateData?.flowTemplate?.title ?? null;

    const handleTemplateRenameSave = useCallback(async () => {
        const newTitle = editingInputRef.current?.value.trim();
        const template = templateData?.flowTemplate;

        if (!templateId || !newTitle || !template) {
            return;
        }

        if (newTitle === template.title) {
            handleTemplateRenameCancel();

            return;
        }

        setIsRenaming(true);

        try {
            // Send the server's current `text`, not the form's, so renaming the title never persists the
            // user's unsaved body edits — those stay dirty in the form (kept by `keepDirtyValues`) until they save.
            await updateTemplate(templateId, { text: template.text, title: newTitle });
            toast.success('已重命名');
            handleTemplateRenameCancel();
        } catch {
            // Error already handled in provider with toast
        } finally {
            setIsRenaming(false);
        }
    }, [editingInputRef, handleTemplateRenameCancel, templateId, templateData?.flowTemplate, updateTemplate]);

    const handleTemplateDelete = useCallback(async () => {
        if (!templateId) {
            return;
        }

        setIsDeleting(true);

        try {
            await deleteTemplate(templateId);
            navigate(routes.templates, { replace: true });
        } catch {
            // Error already handled in provider with toast
        } finally {
            setIsDeleting(false);
        }
    }, [templateId, deleteTemplate, navigate]);

    const performSave = useCallback(
        async (values: FormValues): Promise<boolean> => {
            setIsSaving(true);

            try {
                if (isNew) {
                    await createTemplate(values.title, values.text);
                } else if (templateId) {
                    await updateTemplate(templateId, { text: values.text, title: values.title });
                    // See knowledge-form.tsx: the form-level `resetOptions` are merged into every manual
                    // reset, so a post-save reset inherits `keepDirtyValues` unless it opts out.
                    reset(values, { keepDefaultValues: false, keepDirtyValues: false });
                }

                return true;
            } catch {
                // Error already handled in provider with toast
                return false;
            } finally {
                setIsSaving(false);
            }
        },
        [isNew, templateId, createTemplate, updateTemplate, reset],
    );

    const handleSaveFromGuard = useCallback(async (): Promise<boolean> => {
        if (isSaving || !isValid) {
            return false;
        }

        const parsed = formSchema.safeParse(getValues());

        return parsed.success ? performSave(parsed.data) : false;
    }, [getValues, isSaving, isValid, performSave]);

    const guard = useUnsavedChangesGuard({
        isDirty,
        isFormValid: isValid,
        onSave: handleSaveFromGuard,
    });

    const handleSubmit = async (values: FormValues) => {
        if (isSaving) {
            return;
        }

        if ((await performSave(values)) && isNew) {
            // A fresh template stays dirty until we leave; skip the guard's blocker so this post-save
            // navigation doesn't trap the user in the unsaved-changes dialog.
            guard.skipNextBlock();
            navigate(routes.templates);
        }
    };

    const handleApplyPreset = useCallback(
        (preset: TemplatePreset) => {
            const current = getValues();
            const hasContent = (current.title?.trim().length ?? 0) > 0 || (current.text?.trim().length ?? 0) > 0;

            if (hasContent) {
                setPendingPreset(preset);
                setIsReplaceConfirmOpen(true);
            } else {
                setValue('title', preset.title, { shouldDirty: true, shouldValidate: true });
                setValue('text', preset.text, { shouldDirty: true, shouldValidate: true });
            }
        },
        [getValues, setValue],
    );

    const handleConfirmReplacePreset = useCallback(() => {
        if (pendingPreset) {
            setValue('title', pendingPreset.title, { shouldDirty: true, shouldValidate: true });
            setValue('text', pendingPreset.text, { shouldDirty: true, shouldValidate: true });
            setPendingPreset(null);
        }
    }, [pendingPreset, setValue]);

    const hasTemplate = !!templateData?.flowTemplate;
    const isTemplatePending = !isNew && !hasTemplate;
    const isTemplateMissing = !isNew && !isLoadingTemplate && !hasTemplate;

    const pageHeader = (
        <>
            <AppHeader>
                <AppHeaderContent>
                    <Breadcrumb className="min-w-0 flex-1">
                        <BreadcrumbList className="min-w-0 flex-nowrap">
                            <BreadcrumbItem className="min-w-0 gap-2">
                                {isEditingTitle && hasTemplate ? (
                                    <InlineEditInput
                                        busy={isRenaming}
                                        className="w-64 max-w-full min-w-0 flex-1"
                                        defaultValue={templateName ?? ''}
                                        inputRef={editingInputRef}
                                        onCancel={handleTemplateRenameCancel}
                                        onSave={handleTemplateRenameSave}
                                        placeholder="模板标题"
                                    />
                                ) : hasTemplate ? (
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <BreadcrumbPage
                                                className="max-w-64 min-w-0 cursor-text truncate select-none"
                                                onDoubleClick={handleTemplateRenameStart}
                                            >
                                                {templateName ?? '模板'}
                                            </BreadcrumbPage>
                                        </TooltipTrigger>
                                        <TooltipContent>双击重命名</TooltipContent>
                                    </Tooltip>
                                ) : (
                                    <BreadcrumbPage className="min-w-0 truncate">
                                        {isNew ? '新建模板' : (templateName ?? '模板')}
                                    </BreadcrumbPage>
                                )}
                            </BreadcrumbItem>
                        </BreadcrumbList>
                    </Breadcrumb>
                </AppHeaderContent>
                {!isTemplateMissing && (
                    <AppHeaderActions>
                        {!isNew && !isMobile && (
                            <DetailNavigationToolbar<Template>
                                controller={templateNav}
                                renderItem={renderTemplateItem}
                                sheetIcon={<FileText className="size-4" />}
                                sheetTitle="Templates"
                            />
                        )}
                        <AppHeaderAction
                            disabled={isTemplatePending || (!isNew && !hasUnsavedChanges)}
                            form="template-form"
                            icon={<Save />}
                            label={isNew ? '创建' : '保存'}
                            loading={isSaving}
                            type="submit"
                        />
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <Button
                                    aria-label="模板操作"
                                    className="size-8 p-0"
                                    variant="ghost"
                                >
                                    <Ellipsis />
                                </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent
                                align="end"
                                className="min-w-24"
                                onCloseAutoFocus={handleDropdownCloseAutoFocus}
                            >
                                {!isNew && (
                                    <>
                                        {isMobile && (
                                            <>
                                                <DropdownMenuItem
                                                    className="cursor-default hover:bg-transparent focus:bg-transparent"
                                                    onSelect={(event) => event.preventDefault()}
                                                >
                                                    <FileText />
                                                    模板
                                                    <div className="-my-1.5 -mr-2 ml-auto flex items-center">
                                                        <DetailNavigationButtons<Template>
                                                            controller={templateNav}
                                                            sheetTitle="Templates"
                                                            size="sm"
                                                        />
                                                    </div>
                                                </DropdownMenuItem>
                                                <DropdownMenuSeparator />
                                            </>
                                        )}
                                        <DropdownMenuItem
                                            disabled={isTemplatePending}
                                            onClick={handleTemplateRenameStart}
                                        >
                                            <Pencil />
                                            重命名
                                        </DropdownMenuItem>
                                        <DropdownMenuSeparator />
                                    </>
                                )}
                                <DropdownMenuItem
                                    className="cursor-default gap-4 hover:bg-transparent focus:bg-transparent"
                                    onSelect={(event) => event.preventDefault()}
                                >
                                    视图
                                    <EditorViewModeToggle
                                        className="-my-1.5 -mr-2 ml-auto"
                                        mode={viewMode}
                                        onModeChange={setViewMode}
                                        rawTooltip="编辑模板源码"
                                    />
                                </DropdownMenuItem>
                                {!isNew && (
                                    <>
                                        <DropdownMenuSeparator />
                                        <DropdownMenuItem
                                            disabled={isDeleting || isTemplatePending}
                                            onClick={() => setIsDeleteDialogOpen(true)}
                                        >
                                            {isDeleting ? (
                                                <>
                                                    <Spinner variant="circle" />
                                                    删除中…
                                                </>
                                            ) : (
                                                <>
                                                    <Trash />
                                                    删除
                                                </>
                                            )}
                                        </DropdownMenuItem>
                                    </>
                                )}
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </AppHeaderActions>
                )}
            </AppHeader>
            {isMobile && !isNew && (
                <DetailNavigationSheet<Template>
                    controller={templateNav}
                    renderItem={renderTemplateItem}
                    sheetIcon={<FileText className="size-4" />}
                    sheetTitle="Templates"
                />
            )}
        </>
    );

    const presetsList = (onApplied?: () => void) => (
        <div className="flex w-full min-w-0 flex-col gap-2 p-2">
            {presets.map((preset, index) => (
                <Collapsible
                    className="w-full min-w-0"
                    key={preset.title}
                    onOpenChange={(open) => setExpandedPresetIndex(open ? index : null)}
                    open={expandedPresetIndex === index}
                >
                    <Card className="w-full min-w-0">
                        <div className="flex w-full min-w-0">
                            <Button
                                className={cn(
                                    'h-auto min-w-0 flex-1 justify-start gap-2 rounded-none rounded-tl-[0.6875rem] px-3 py-2 text-left text-start',
                                    expandedPresetIndex !== index ? 'rounded-bl-[0.6875rem]' : 'whitespace-normal',
                                )}
                                onClick={() => {
                                    handleApplyPreset(preset);
                                    onApplied?.();
                                }}
                                variant="ghost"
                            >
                                <span className={cn('min-w-0 flex-1', expandedPresetIndex !== index && 'truncate')}>
                                    {preset.title}
                                </span>
                                <Badge
                                    className="shrink-0 font-normal"
                                    variant="outline"
                                >
                                    {preset.domain}
                                </Badge>
                            </Button>
                            <CollapsibleTrigger asChild>
                                <Button
                                    className={cn(
                                        'h-auto shrink-0 rounded-none rounded-tr-[0.6875rem] border-l px-2 py-2',
                                        expandedPresetIndex !== index && 'rounded-br-[0.6875rem]',
                                    )}
                                    variant="ghost"
                                >
                                    <ChevronDown
                                        className={cn(
                                            'transition-transform',
                                            expandedPresetIndex === index && 'rotate-180',
                                        )}
                                    />
                                </Button>
                            </CollapsibleTrigger>
                        </div>
                        <CollapsibleContent>
                            <CardContent className="border-t px-3 py-2">
                                <p className="text-muted-foreground text-sm break-words whitespace-pre-wrap">
                                    {preset.text}
                                </p>
                            </CardContent>
                        </CollapsibleContent>
                    </Card>
                </Collapsible>
            ))}
        </div>
    );

    const presetsPanel = isDesktop ? (
        <div className="bg-card overflow-hidden rounded-lg border">
            <div className="border-b px-4 py-3">
                <h4 className="flex items-center gap-2 text-sm font-medium">
                    {PRESETS_TITLE}
                    <Badge
                        className="ml-auto font-normal tabular-nums"
                        variant="secondary"
                    >
                        {presets.length}
                    </Badge>
                </h4>
                <p className="text-muted-foreground mt-1 text-xs">点击预设填入表单，展开可预览内容。</p>
            </div>
            {presetsList()}
        </div>
    ) : (
        <Popover
            onOpenChange={setIsPresetsOpen}
            open={isPresetsOpen}
        >
            <PopoverTrigger asChild>
                <Button
                    className="w-full justify-start"
                    size="sm"
                    variant="secondary"
                >
                    <LayoutTemplate />
                    {PRESETS_TITLE}
                    <Badge
                        className="ml-auto h-5 font-normal tabular-nums"
                        variant="outline"
                    >
                        {presets.length}
                    </Badge>
                </Button>
            </PopoverTrigger>
            <PopoverContent
                align="start"
                className="max-h-(--radix-popover-content-available-height) w-(--radix-popover-trigger-width) overflow-y-auto overscroll-contain p-0"
            >
                {presetsList(() => setIsPresetsOpen(false))}
            </PopoverContent>
        </Popover>
    );

    const introBlock = (
        <div className="flex flex-col gap-2 text-center">
            <h2 className="text-2xl font-semibold">{isNew ? '新建模板' : '编辑模板'}</h2>
            <p className="text-muted-foreground">填写标题与内容，或从预设开始。</p>
        </div>
    );

    const titleField = (
        <FormField
            control={control}
            name="title"
            render={({ field }) => (
                <FormItem>
                    <FormLabel>标题</FormLabel>
                    <FormControl>
                        <Input
                            autoFocus={isNew}
                            disabled={isSaving}
                            placeholder="这个模板的简短名称"
                            {...field}
                        />
                    </FormControl>
                    <FormMessage />
                </FormItem>
            )}
        />
    );

    const textEditor = (
        <FormField
            control={control}
            name="text"
            render={({ field }) => (
                <FormItem className="flex min-h-0 flex-1 flex-col">
                    <FormControl>
                        <MarkdownEditorField
                            aria-label="模板内容"
                            disabled={isSaving}
                            mode={viewMode}
                            onBlur={field.onBlur}
                            onChange={field.onChange}
                            placeholder="描述任务，或从预设开始"
                            ref={field.ref}
                            value={field.value}
                        />
                    </FormControl>
                    {/* Full-height field: the invalid state shows as the editor's red border (via aria-invalid),
                        not text below it (no room in the flex layout). Kept sr-only for screen readers. */}
                    <FormMessage className="sr-only" />
                </FormItem>
            )}
        />
    );

    if (!isNew && isLoadingTemplate && !template) {
        return (
            <div className={isDesktop ? 'flex h-[100dvh] min-h-0 flex-col' : 'flex min-h-[100dvh] flex-col'}>
                {pageHeader}
                <div className="flex flex-1 items-center justify-center">
                    <Spinner variant="circle" />
                </div>
            </div>
        );
    }

    if (templateLoadError) {
        return (
            <div className={isDesktop ? 'flex h-[100dvh] min-h-0 flex-col' : 'flex min-h-[100dvh] flex-col'}>
                {pageHeader}
                <div className="flex flex-1 flex-col gap-4 p-4">
                    <ErrorState
                        message={templateLoadError.message}
                        onRetry={() => refetchTemplate()}
                        title="模板加载失败"
                    />
                </div>
            </div>
        );
    }

    if (!isNew && !isLoadingTemplate && !template) {
        return (
            <div className={isDesktop ? 'flex h-[100dvh] min-h-0 flex-col' : 'flex min-h-[100dvh] flex-col'}>
                {pageHeader}
                <div className="flex flex-1 items-center justify-center p-4">
                    <Card className="w-full max-w-2xl">
                        <CardContent className="flex flex-col items-center gap-4 pt-6 text-center">
                            <h2 className="text-xl font-semibold">模板不存在</h2>
                            <p className="text-muted-foreground">找不到你要的模板。</p>
                            <Button onClick={() => navigate(routes.templates)}>返回模板列表</Button>
                        </CardContent>
                    </Card>
                </div>
            </div>
        );
    }

    return (
        <div className={isDesktop ? 'flex h-[100dvh] min-h-0 flex-col' : 'flex min-h-[100dvh] flex-col'}>
            {pageHeader}
            <Form {...form}>
                <form
                    className="flex min-h-0 flex-1 flex-col"
                    id="template-form"
                    noValidate
                    onSubmit={handleFormSubmit(handleSubmit)}
                >
                    {isDesktop ? (
                        <DetailSplitLayout
                            content={textEditor}
                            panel={
                                <>
                                    {introBlock}
                                    {titleField}
                                    {presets.length > 0 ? presetsPanel : null}
                                </>
                            }
                        />
                    ) : (
                        <div className="flex min-h-0 flex-1 flex-col gap-4 p-4">
                            {introBlock}
                            {titleField}
                            {presets.length > 0 ? presetsPanel : null}
                            {textEditor}
                        </div>
                    )}
                </form>
            </Form>
            <ConfirmationDialog
                confirmIcon={<FileSymlink />}
                confirmText="替换"
                confirmVariant="default"
                description="当前表单已有内容，确定用所选预设替换吗？"
                handleConfirm={handleConfirmReplacePreset}
                handleOpenChange={(open) => {
                    if (!open) {
                        setPendingPreset(null);
                    }

                    setIsReplaceConfirmOpen(open);
                }}
                isOpen={isReplaceConfirmOpen}
                title="替换内容？"
            />
            <ConfirmationDialog
                cancelText="取消"
                confirmText="删除"
                handleConfirm={handleTemplateDelete}
                handleOpenChange={setIsDeleteDialogOpen}
                isOpen={isDeleteDialogOpen}
                itemName={templateName ?? undefined}
                itemType="模板"
            />
            <UnsavedChangesDialog
                canSave={isValid}
                handleCancel={guard.handleCancel}
                handleDiscard={guard.handleDiscard}
                handleOpenChange={guard.handleOpenChange}
                handleSaveAndLeave={guard.handleSaveAndLeave}
                isOpen={guard.isOpen}
                isSavingFromDialog={guard.isSavingFromDialog}
            />
        </div>
    );
}

export default Template;
