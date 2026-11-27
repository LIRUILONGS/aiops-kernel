import {
    ArrowUp,
    Check,
    ChevronDown,
    Ellipsis,
    FileSymlink,
    FileText,
    Folder,
    Paperclip,
    Plus,
    Square,
    X,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useRef } from 'react';
import { useController } from 'react-hook-form';
import { z } from 'zod';

import type { UserResourceFragmentFragment } from '@/graphql/types';

import { ProviderIcon } from '@/components/icons/provider-icon';
import ConfirmationDialog from '@/components/shared/confirmation-dialog';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuGroup,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Form, FormControl, FormField, FormItem, FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import {
    InputGroup,
    InputGroupAddon,
    InputGroupButton,
    InputGroupInput,
    InputGroupTextareaAutosize,
} from '@/components/ui/input-group';
import { Spinner } from '@/components/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useResourcesUpload } from '@/features/resources/use-resources-upload';
import { useAppForm } from '@/hooks/use-app-form';
import { cn } from '@/lib/utils';
import { getProviderDisplayName } from '@/models/provider';
import { useProviders } from '@/providers/providers-provider';
import { useResources } from '@/providers/resources-provider';
import { type Template, useTemplates } from '@/providers/templates-provider';

const TARGET_TYPES = [
    { hint: '由规划器根据描述决定', label: '自动', value: 'auto' as const },
    { hint: '定位故障根因', label: '故障诊断', value: 'diagnosis' as const },
    { hint: '找出瓶颈并度量优化效果', label: '性能优化', value: 'optimization' as const },
];

// The lease switch is chosen once per flow. `off` runs the runtime untouched;
// `shadow` records what a lease would refuse; `enforce` refuses.
const CAPABILITY_MODES = [
    { hint: '不启用能力租约，工具调用与之前完全相同', label: '关闭', value: 'off' as const },
    { hint: '只记录会被拦下的调用，实际不拦，用于先看策略的影响', label: '观察', value: 'shadow' as const },
    { hint: '越出子任务租约的调用直接拒绝', label: '强制', value: 'enforce' as const },
];

const GOAL_FIELDS = [
    {
        label: '指标',
        name: 'goalMetric' as const,
        placeholder: 'p99 延迟、重启次数…',
    },
    { label: '目标', name: 'goalTarget' as const, placeholder: '< 200ms、0 次重启…' },
    { label: '约束', name: 'goalConstraint' as const, placeholder: '不要重启 caddy…' },
];

const formSchema = z.object({
    capabilityMode: z.enum(['off', 'shadow', 'enforce']),
    goalConstraint: z.string().optional(),
    goalMetric: z.string().optional(),
    goalTarget: z.string().optional(),
    message: z.string().trim().min(1, { message: '消息不能为空' }),
    providerName: z.string().trim().min(1, { message: '请选择提供商' }),
    resourceIds: z.array(z.string()),
    targetType: z.enum(['auto', 'diagnosis', 'optimization']),
});

export interface FlowFormProps {
    defaultValues?: Partial<FlowFormValues>;
    isCanceling?: boolean;
    isDisabled?: boolean;
    isLoading?: boolean;
    isProviderDisabled?: boolean;
    isSubmitting?: boolean;
    onCancel?: () => Promise<void> | void;
    onSubmit: (values: FlowFormValues) => boolean | Promise<boolean>;
    placeholder?: string;
}

export type FlowFormValues = z.infer<typeof formSchema>;

export function FlowForm({
    defaultValues,
    isCanceling,
    isDisabled,
    isLoading,
    isProviderDisabled,
    isSubmitting,
    onCancel,
    onSubmit,
    placeholder = '描述要诊断的故障，或要优化的服务…',
}: FlowFormProps) {
    const { providers, setSelectedProvider } = useProviders();
    const { templates } = useTemplates();
    const { resources } = useResources();
    const [isReplaceConfirmOpen, setIsReplaceConfirmOpen] = useState(false);
    const [pendingTemplate, setPendingTemplate] = useState<null | Template>(null);
    const [providerSearch, setProviderSearch] = useState('');
    const [templateSearch, setTemplateSearch] = useState('');
    const [resourceSearch, setResourceSearch] = useState('');
    // Lifted to form state so the tab choice survives re-renders triggered by
    // `setTemplateSearch` / `setResourceSearch` inside the inner pickers.
    const [pickerTab, setPickerTab] = useState<'resources' | 'templates'>('templates');

    const fileInputRef = useRef<HTMLInputElement>(null);

    // Alphabetical sort by full path yields hierarchical order: parents before
    // their descendants, peers alphabetically at every depth.
    const sortedResources = useMemo(
        () => [...resources].sort((a, b) => a.path.localeCompare(b.path, undefined, { sensitivity: 'base' })),
        [resources],
    );

    const filteredResources = useMemo(() => {
        const queryValue = resourceSearch.trim().toLowerCase();

        if (!queryValue) {
            return sortedResources;
        }

        return sortedResources.filter(
            (resource) =>
                resource.name.toLowerCase().includes(queryValue) || resource.path.toLowerCase().includes(queryValue),
        );
    }, [sortedResources, resourceSearch]);

    const isResourceSearchActive = resourceSearch.trim().length > 0;

    const filteredTemplates = useMemo(() => {
        if (!templateSearch.trim()) {
            return templates;
        }

        const searchLower = templateSearch.toLowerCase();

        return templates.filter(
            (template) =>
                template.title.toLowerCase().includes(searchLower) || template.text.toLowerCase().includes(searchLower),
        );
    }, [templates, templateSearch]);

    const filteredProviders = useMemo(() => {
        if (!providerSearch.trim()) {
            return providers;
        }

        const searchLower = providerSearch.toLowerCase();

        return providers.filter((provider) => {
            const displayName = getProviderDisplayName(provider).toLowerCase();

            return displayName.includes(searchLower) || provider.name.toLowerCase().includes(searchLower);
        });
    }, [providers, providerSearch]);

    const form = useAppForm<FlowFormValues>({
        defaultValues: {
            capabilityMode: defaultValues?.capabilityMode ?? 'off',
            goalConstraint: defaultValues?.goalConstraint ?? '',
            goalMetric: defaultValues?.goalMetric ?? '',
            goalTarget: defaultValues?.goalTarget ?? '',
            message: defaultValues?.message ?? '',
            providerName: defaultValues?.providerName ?? '',
            resourceIds: defaultValues?.resourceIds ?? [],
            targetType: defaultValues?.targetType ?? 'auto',
        },
        schema: formSchema,
    });

    const {
        control,
        formState: { dirtyFields, isValid },
        getValues,
        handleSubmit: handleFormSubmit,
        resetField,
        setValue,
    } = form;

    // useController registers the field; with useWatch alone `resetField('resourceIds')` is a no-op.
    const {
        field: { value: resourceIds },
    } = useController({ control, name: 'resourceIds' });

    const updateResourceIds = useCallback(
        (updater: ((current: string[]) => string[]) | Array<number | string>) => {
            const current = getValues('resourceIds') ?? [];
            const raw = typeof updater === 'function' ? updater(current) : updater;
            // Canonical cache shape carries `id` as a number (see `resources-rest.ts`),
            // while zod enforces `z.array(z.string())` on the form state. String-coerce
            // every incoming value so the form always holds a consistent string array.
            const next = raw.map((id) => String(id));
            setValue('resourceIds', next, { shouldDirty: true, shouldValidate: true });
        },
        [getValues, setValue],
    );

    const flowResources = useMemo<UserResourceFragmentFragment[]>(() => {
        const byId = new Map(resources.map((item) => [String(item.id), item]));

        return resourceIds
            .map((id) => byId.get(id))
            .filter((item): item is UserResourceFragmentFragment => Boolean(item));
    }, [resourceIds, resources]);

    const upload = useResourcesUpload({
        onSuccess: (uploaded) => {
            const ids = uploaded.items.map((item) => String(item.id));

            if (ids.length === 0) {
                return;
            }

            updateResourceIds((current) => {
                const merged = new Set(current);
                ids.forEach((id) => merged.add(id));

                return Array.from(merged);
            });
        },
    });

    useEffect(() => {
        if (!defaultValues) {
            return;
        }

        const currentValues = getValues();

        // Arrays are compared shallowly so a new-but-identical `resourceIds` reference doesn't
        // trigger an unnecessary setValue (and the re-render it causes).
        Object.entries(defaultValues)
            .filter(([fieldName, defaultValue]) => {
                const typedFieldName = fieldName as keyof FlowFormValues;

                if (defaultValue === undefined || dirtyFields[typedFieldName]) {
                    return false;
                }

                const currentValue = currentValues[typedFieldName];

                if (Array.isArray(defaultValue) && Array.isArray(currentValue)) {
                    return (
                        currentValue.length !== defaultValue.length ||
                        currentValue.some((item, index) => item !== defaultValue[index])
                    );
                }

                return currentValue !== defaultValue;
            })
            .forEach(([fieldName, defaultValue]) => {
                const typedFieldName = fieldName as keyof FlowFormValues;
                // resetField, not setValue: it moves react-hook-form's own baseline too, so a later
                // pick that happens to equal the mount-time default still counts as dirty and this
                // effect leaves it alone.
                resetField(typedFieldName, { defaultValue: defaultValue as never });
            });
    }, [defaultValues, dirtyFields, resetField, getValues]);

    const isFormDisabled = isDisabled || isLoading || isSubmitting || isCanceling;

    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const previousFormDisabledRef = useRef(isFormDisabled);

    useEffect(() => {
        const wasDisabled = previousFormDisabledRef.current;
        previousFormDisabledRef.current = isFormDisabled;

        if (wasDisabled && !isFormDisabled) {
            textareaRef.current?.focus();
        }
    }, [isFormDisabled]);

    const handleSubmit = async (values: FlowFormValues) => {
        if (!(await onSubmit(values))) {
            return;
        }

        resetField('message');
        resetField('resourceIds');
    };

    const handleAttachClick = () => {
        fileInputRef.current?.click();
    };

    const handleFileInputChange = useCallback(
        async (event: React.ChangeEvent<HTMLInputElement>) => {
            const selectedFiles = Array.from(event.target.files ?? []);

            // Reset native input so re-uploading the same file fires `change` again.
            event.target.value = '';

            if (selectedFiles.length === 0) {
                return;
            }

            await upload.uploadFiles(selectedFiles);
        },
        [upload],
    );

    const handleRemoveAttachment = (id: string) => {
        updateResourceIds((current) => current.filter((fileId) => fileId !== id));
    };

    const handleToggleAttachment = (id: string) => {
        updateResourceIds((current) =>
            current.includes(id) ? current.filter((fileId) => fileId !== id) : [...current, id],
        );
    };

    const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
        const { ctrlKey, key, metaKey, shiftKey } = event;

        if (isFormDisabled || key !== 'Enter' || shiftKey || ctrlKey || metaKey) {
            return;
        }

        event.preventDefault();
        handleFormSubmit(handleSubmit)();
    };

    const handleApplyTemplate = useCallback(
        (template: Template) => {
            const currentMessage = getValues('message')?.trim() ?? '';

            if (currentMessage.length > 0) {
                setPendingTemplate(template);
                setIsReplaceConfirmOpen(true);
            } else {
                setValue('message', template.text, { shouldValidate: true });
                setTemplateSearch('');
            }
        },
        [getValues, setValue],
    );

    const handleConfirmReplaceTemplate = useCallback(() => {
        if (pendingTemplate) {
            setValue('message', pendingTemplate.text, { shouldValidate: true });
            setTemplateSearch('');
            setPendingTemplate(null);
        }
    }, [pendingTemplate, setValue]);

    const renderTemplatePickerInner = () => (
        <>
            <DropdownMenuGroup className="-m-1 rounded-none p-0">
                <InputGroup className="-mb-1 rounded-none border-0 shadow-none [&:has([data-slot=input-group-control]:focus-visible)]:border-0 [&:has([data-slot=input-group-control]:focus-visible)]:ring-0">
                    <InputGroupInput
                        onChange={(event) => setTemplateSearch(event.target.value)}
                        onClick={(event) => event.stopPropagation()}
                        onKeyDown={(event) => event.stopPropagation()}
                        placeholder="搜索…"
                        value={templateSearch}
                    />
                    {templateSearch && (
                        <InputGroupAddon align="inline-end">
                            <InputGroupButton
                                aria-label="清空模板搜索"
                                onClick={(event) => {
                                    event.stopPropagation();
                                    setTemplateSearch('');
                                }}
                            >
                                <X />
                            </InputGroupButton>
                        </InputGroupAddon>
                    )}
                </InputGroup>
                <DropdownMenuSeparator />
            </DropdownMenuGroup>
            <DropdownMenuGroup className="max-h-64 overflow-y-auto">
                {!filteredTemplates.length ? (
                    <DropdownMenuItem
                        className="min-h-16 justify-center"
                        disabled
                    >
                        {templateSearch ? '没有结果' : '暂无可用模板'}
                    </DropdownMenuItem>
                ) : (
                    filteredTemplates.map((template) => (
                        <DropdownMenuItem
                            key={template.id}
                            onSelect={() => {
                                if (isFormDisabled) {
                                    return;
                                }

                                handleApplyTemplate(template);
                            }}
                        >
                            <span className="max-w-80 flex-1 truncate">{template.title}</span>
                        </DropdownMenuItem>
                    ))
                )}
            </DropdownMenuGroup>
        </>
    );

    const renderResourcePickerInner = () => (
        <>
            <DropdownMenuGroup className="-m-1 rounded-none p-0">
                <InputGroup className="-mb-1 rounded-none border-0 shadow-none [&:has([data-slot=input-group-control]:focus-visible)]:border-0 [&:has([data-slot=input-group-control]:focus-visible)]:ring-0">
                    <InputGroupInput
                        onChange={(event) => setResourceSearch(event.target.value)}
                        onClick={(event) => event.stopPropagation()}
                        onKeyDown={(event) => event.stopPropagation()}
                        placeholder="搜索…"
                        value={resourceSearch}
                    />
                    {resourceSearch && (
                        <InputGroupAddon align="inline-end">
                            <InputGroupButton
                                aria-label="清空资源搜索"
                                onClick={(event) => {
                                    event.stopPropagation();
                                    setResourceSearch('');
                                }}
                            >
                                <X />
                            </InputGroupButton>
                        </InputGroupAddon>
                    )}
                </InputGroup>
                <DropdownMenuSeparator />
            </DropdownMenuGroup>
            <DropdownMenuGroup className="max-h-64 overflow-y-auto">
                {!filteredResources.length ? (
                    <DropdownMenuItem
                        className="min-h-16 justify-center"
                        disabled
                    >
                        {resourceSearch ? '没有结果' : '暂无可用资源'}
                    </DropdownMenuItem>
                ) : (
                    filteredResources.map((resource) => {
                        const resourceId = String(resource.id);
                        const isSelected = resourceIds.includes(resourceId);
                        const Icon = resource.isDir ? Folder : FileText;
                        // Zeroed while a search query is active so matches don't appear
                        // orphaned beneath hidden ancestors.
                        const depth = isResourceSearchActive ? 0 : resource.path.split('/').length - 1;

                        return (
                            <DropdownMenuItem
                                key={resourceId}
                                onSelect={(event) => {
                                    event.preventDefault();

                                    if (isFormDisabled) {
                                        return;
                                    }

                                    handleToggleAttachment(resourceId);
                                }}
                                style={{ paddingLeft: `${0.5 + depth * 0.875}rem` }}
                            >
                                <div className="flex w-full min-w-0 items-center gap-2">
                                    <Icon className="text-muted-foreground size-4 shrink-0" />
                                    <div className="flex min-w-0 flex-1 flex-col">
                                        <span className="truncate">{resource.name}</span>
                                        {isResourceSearchActive && resource.path !== resource.name && (
                                            <span className="text-muted-foreground truncate text-xs">
                                                {resource.path}
                                            </span>
                                        )}
                                    </div>
                                    {isSelected && <Check className="ml-auto size-4 shrink-0" />}
                                </div>
                            </DropdownMenuItem>
                        );
                    })
                )}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
                disabled={upload.isUploading}
                onSelect={(event) => {
                    event.preventDefault();

                    if (isFormDisabled) {
                        return;
                    }

                    handleAttachClick();
                }}
            >
                {upload.isUploading ? <Spinner variant="circle" /> : <Plus />}
                {upload.isUploading ? 'Uploading…' : '上传文件'}
            </DropdownMenuItem>
        </>
    );

    return (
        <Form {...form}>
            <form
                noValidate
                onSubmit={handleFormSubmit(handleSubmit)}
            >
                <FormField
                    control={control}
                    name="message"
                    render={({ field }) => (
                        <FormControl>
                            <InputGroup className="block">
                                {flowResources.length > 0 && (
                                    <InputGroupAddon
                                        align="block-start"
                                        className="flex-wrap gap-1.5"
                                    >
                                        {flowResources.map((resource) => {
                                            const Icon = resource.isDir ? Folder : FileText;
                                            const resourceId = String(resource.id);

                                            return (
                                                <div
                                                    className="bg-muted/50 flex max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-xs"
                                                    key={resourceId}
                                                    title={resource.path}
                                                >
                                                    <Icon className="text-muted-foreground size-3.5 shrink-0" />
                                                    <span className="text-foreground max-w-40 truncate">
                                                        {resource.name}
                                                    </span>
                                                    <Button
                                                        aria-label={`Remove ${resource.name}`}
                                                        className="text-muted-foreground hover:text-destructive -my-[5px] -mr-1.5 -ml-1 size-[26px] shrink-0 [&_svg]:size-3.5"
                                                        disabled={isFormDisabled}
                                                        onClick={() => handleRemoveAttachment(resourceId)}
                                                        size="icon-xs"
                                                        variant="ghost"
                                                    >
                                                        <X />
                                                    </Button>
                                                </div>
                                            );
                                        })}
                                    </InputGroupAddon>
                                )}
                                <InputGroupAddon align="block-start">
                                    <FormField
                                        control={control}
                                        name="targetType"
                                        render={({ field: targetField }) => (
                                            <div className="flex flex-wrap items-center gap-1">
                                                {TARGET_TYPES.map((option) => (
                                                    <InputGroupButton
                                                        aria-pressed={targetField.value === option.value}
                                                        className={cn(
                                                            'text-xs',
                                                            targetField.value === option.value &&
                                                                'bg-accent text-accent-foreground',
                                                        )}
                                                        disabled={isFormDisabled}
                                                        key={option.value}
                                                        onClick={() => targetField.onChange(option.value)}
                                                        title={option.hint}
                                                        variant="ghost"
                                                    >
                                                        {option.label}
                                                    </InputGroupButton>
                                                ))}
                                                <FormField
                                                    control={control}
                                                    name="capabilityMode"
                                                    render={({ field: leaseField }) => (
                                                        <div className="ml-auto flex items-center gap-1">
                                                            <span className="text-muted-foreground text-[10px] leading-none">
                                                                能力租约
                                                            </span>
                                                            {CAPABILITY_MODES.map((option) => (
                                                                <InputGroupButton
                                                                    aria-pressed={leaseField.value === option.value}
                                                                    className={cn(
                                                                        'text-xs',
                                                                        leaseField.value === option.value &&
                                                                            'bg-accent text-accent-foreground',
                                                                    )}
                                                                    disabled={isFormDisabled}
                                                                    key={option.value}
                                                                    onClick={() => leaseField.onChange(option.value)}
                                                                    title={option.hint}
                                                                    variant="ghost"
                                                                >
                                                                    {option.label}
                                                                </InputGroupButton>
                                                            ))}
                                                        </div>
                                                    )}
                                                />
                                            </div>
                                        )}
                                    />
                                </InputGroupAddon>
                                <InputGroupTextareaAutosize
                                    {...field}
                                    autoFocus
                                    className="min-h-0"
                                    disabled={isFormDisabled}
                                    maxRows={9}
                                    minRows={1}
                                    onKeyDown={handleKeyDown}
                                    placeholder={placeholder}
                                    ref={(element) => {
                                        field.ref(element);
                                        textareaRef.current = element;
                                    }}
                                />
                                <InputGroupAddon align="block-end">
                                    <FormField
                                        control={control}
                                        name="providerName"
                                        render={({ field: providerField }) => {
                                            const currentProvider = providers.find(
                                                (p) => p.name === providerField.value,
                                            );

                                            return (
                                                <DropdownMenu>
                                                    <DropdownMenuTrigger asChild>
                                                        <InputGroupButton
                                                            disabled={isFormDisabled || isProviderDisabled}
                                                            variant="ghost"
                                                        >
                                                            {currentProvider && (
                                                                <ProviderIcon provider={currentProvider} />
                                                            )}
                                                            <span className="max-w-40 truncate">
                                                                {currentProvider
                                                                    ? getProviderDisplayName(currentProvider)
                                                                    : '选择提供商'}
                                                            </span>
                                                            <ChevronDown />
                                                        </InputGroupButton>
                                                    </DropdownMenuTrigger>
                                                    <DropdownMenuContent
                                                        align="start"
                                                        side="top"
                                                    >
                                                        <DropdownMenuGroup className="-m-1 rounded-none p-0">
                                                            <InputGroup className="-mb-1 rounded-none border-0 shadow-none [&:has([data-slot=input-group-control]:focus-visible)]:border-0 [&:has([data-slot=input-group-control]:focus-visible)]:ring-0">
                                                                <InputGroupInput
                                                                    onChange={(event) =>
                                                                        setProviderSearch(event.target.value)
                                                                    }
                                                                    onClick={(event) => event.stopPropagation()}
                                                                    onKeyDown={(event) => event.stopPropagation()}
                                                                    placeholder="搜索…"
                                                                    value={providerSearch}
                                                                />
                                                                {providerSearch && (
                                                                    <InputGroupAddon align="inline-end">
                                                                        <InputGroupButton
                                                                            aria-label="清空提供商搜索"
                                                                            onClick={(event) => {
                                                                                event.stopPropagation();
                                                                                setProviderSearch('');
                                                                            }}
                                                                        >
                                                                            <X />
                                                                        </InputGroupButton>
                                                                    </InputGroupAddon>
                                                                )}
                                                            </InputGroup>
                                                            <DropdownMenuSeparator />
                                                        </DropdownMenuGroup>
                                                        <DropdownMenuGroup className="max-h-64 overflow-y-auto">
                                                            {!filteredProviders.length ? (
                                                                <DropdownMenuItem
                                                                    className="min-h-16 justify-center"
                                                                    disabled
                                                                >
                                                                    {providerSearch ? '没有结果' : '暂无可用提供商'}
                                                                </DropdownMenuItem>
                                                            ) : (
                                                                filteredProviders.map((provider) => (
                                                                    <DropdownMenuItem
                                                                        key={provider.name}
                                                                        onSelect={() => {
                                                                            if (isFormDisabled || isProviderDisabled) {
                                                                                return;
                                                                            }

                                                                            providerField.onChange(provider.name);
                                                                            setSelectedProvider(provider);
                                                                            setProviderSearch('');
                                                                        }}
                                                                    >
                                                                        <div className="flex w-full min-w-0 items-center gap-2">
                                                                            <ProviderIcon
                                                                                className="size-4 shrink-0"
                                                                                provider={provider}
                                                                            />

                                                                            <span className="flex-1 truncate">
                                                                                {getProviderDisplayName(provider)}
                                                                            </span>
                                                                            {providerField.value === provider.name && (
                                                                                <Check className="ml-auto size-4 shrink-0" />
                                                                            )}
                                                                        </div>
                                                                    </DropdownMenuItem>
                                                                ))
                                                            )}
                                                        </DropdownMenuGroup>
                                                    </DropdownMenuContent>
                                                </DropdownMenu>
                                            );
                                        }}
                                    />

                                    <FormField
                                        control={control}
                                        name="targetType"
                                        render={({ field: modeField }) =>
                                            modeField.value === 'optimization' ? (
                                                <div className="border-muted-foreground/20 rounded-md border p-2">
                                                    <div className="text-muted-foreground mb-2 text-xs">
                                                        What this performance goal measures — appended to the message so
                                                        the planner can turn it into a hypothesis
                                                    </div>
                                                    <div className="grid gap-2 sm:grid-cols-3">
                                                        {GOAL_FIELDS.map((goal) => (
                                                            <FormField
                                                                control={control}
                                                                key={goal.name}
                                                                name={goal.name}
                                                                render={({ field }) => (
                                                                    <FormItem>
                                                                        <FormLabel className="text-xs">
                                                                            {goal.label}
                                                                        </FormLabel>
                                                                        <FormControl>
                                                                            <Input
                                                                                {...field}
                                                                                disabled={isFormDisabled}
                                                                                placeholder={goal.placeholder}
                                                                                value={field.value ?? ''}
                                                                            />
                                                                        </FormControl>
                                                                    </FormItem>
                                                                )}
                                                            />
                                                        ))}
                                                    </div>
                                                </div>
                                            ) : (
                                                <></>
                                            )
                                        }
                                    />

                                    <DropdownMenu
                                        onOpenChange={(open) => {
                                            if (!open) {
                                                setTemplateSearch('');
                                                setResourceSearch('');
                                            }
                                        }}
                                    >
                                        <DropdownMenuTrigger asChild>
                                            <InputGroupButton
                                                aria-label="模板与资源"
                                                className="ml-auto shrink-0"
                                                disabled={isFormDisabled}
                                                size="icon-xs"
                                                variant="ghost"
                                            >
                                                <Ellipsis className="shrink-0" />
                                            </InputGroupButton>
                                        </DropdownMenuTrigger>
                                        {/* Sub-menus would get clipped on the narrowest screens (~390px), so
                                            Templates and Resources share one upward-opening dropdown. The tab
                                            strip is rendered last so it lands closest to the trigger button. */}
                                        <DropdownMenuContent
                                            align="end"
                                            className="w-72"
                                            side="top"
                                        >
                                            <Tabs
                                                onValueChange={(value) => {
                                                    // Defer the content swap to the next task so it lands
                                                    // *after* the pointerup that the click triggered.
                                                    // Radix `DropdownMenuItem` listens to pointerup directly,
                                                    // so if we swap synchronously, the pointerup at the tab
                                                    // coordinates lands on the freshly-mounted "上传文件"
                                                    // item in the Resources panel and fires its onSelect.
                                                    setTimeout(
                                                        () => setPickerTab(value as 'resources' | 'templates'),
                                                        0,
                                                    );
                                                }}
                                                value={pickerTab}
                                            >
                                                <TabsContent
                                                    className="mt-0 focus-visible:ring-0"
                                                    value="templates"
                                                >
                                                    {renderTemplatePickerInner()}
                                                </TabsContent>
                                                <TabsContent
                                                    className="mt-0 focus-visible:ring-0"
                                                    value="resources"
                                                >
                                                    {renderResourcePickerInner()}
                                                </TabsContent>
                                                <TabsList className="mt-1 grid w-full grid-cols-2">
                                                    <TabsTrigger
                                                        className="gap-1.5"
                                                        value="templates"
                                                    >
                                                        <FileText className="size-3.5" />
                                                        模板
                                                    </TabsTrigger>
                                                    <TabsTrigger
                                                        className="gap-1.5"
                                                        value="resources"
                                                    >
                                                        <Paperclip className="size-3.5" />
                                                        资源
                                                        {flowResources.length > 0 && (
                                                            <span className="bg-muted-foreground/20 text-foreground flex h-4 min-w-4 items-center justify-center rounded px-1 text-[10px] font-medium tabular-nums">
                                                                {flowResources.length}
                                                            </span>
                                                        )}
                                                    </TabsTrigger>
                                                </TabsList>
                                            </Tabs>
                                        </DropdownMenuContent>
                                    </DropdownMenu>

                                    {!isLoading || isSubmitting ? (
                                        <InputGroupButton
                                            aria-label={isSubmitting ? 'Submitting…' : '提交'}
                                            className="shrink-0"
                                            disabled={isSubmitting || !isValid || upload.isUploading}
                                            size="icon-xs"
                                            type="submit"
                                            variant="default"
                                        >
                                            {isSubmitting ? <Spinner variant="circle" /> : <ArrowUp />}
                                        </InputGroupButton>
                                    ) : (
                                        <InputGroupButton
                                            aria-label={isCanceling ? 'Stopping…' : 'Stop'}
                                            className="shrink-0"
                                            disabled={isCanceling || !onCancel}
                                            onClick={() => onCancel?.()}
                                            size="icon-xs"
                                            type="button"
                                            variant="destructive"
                                        >
                                            {isCanceling ? <Spinner variant="circle" /> : <Square />}
                                        </InputGroupButton>
                                    )}
                                </InputGroupAddon>
                            </InputGroup>
                        </FormControl>
                    )}
                />
            </form>
            <input
                aria-hidden="true"
                className="hidden"
                multiple
                name="flow-form-attachment"
                onChange={handleFileInputChange}
                ref={fileInputRef}
                tabIndex={-1}
                type="file"
            />
            <ConfirmationDialog
                confirmIcon={<FileSymlink />}
                confirmText="替换"
                confirmVariant="default"
                description="当前消息已有内容，确定用所选模板替换吗？"
                handleConfirm={handleConfirmReplaceTemplate}
                handleOpenChange={(open) => {
                    if (!open) {
                        setPendingTemplate(null);
                    }

                    setIsReplaceConfirmOpen(open);
                }}
                isOpen={isReplaceConfirmOpen}
                title="替换内容？"
            />
        </Form>
    );
}
