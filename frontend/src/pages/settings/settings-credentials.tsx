import type { ColumnDef } from '@tanstack/react-table';

import { useMutation, useQuery, useSubscription } from '@apollo/client/react';
import { Ellipsis, KeyRound, Pencil, Plus, RotateCcw, ShieldCheck, Trash } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { toast } from 'sonner';

import type { CredentialFragmentFragment } from '@/graphql/types';

import {
    AppHeader,
    AppHeaderAction,
    AppHeaderActions,
    AppHeaderContent,
    AppHeaderTitle,
} from '@/components/layouts/app/app-header';
import ConfirmationDialog from '@/components/shared/confirmation-dialog';
import { ErrorState } from '@/components/shared/error-state';
import { LoadingState } from '@/components/shared/loading-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { DataTable, DataTableColumnHeader } from '@/components/ui/data-table';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Spinner } from '@/components/ui/spinner';
import { Textarea } from '@/components/ui/textarea';
import {
    CreateCredentialDocument,
    CredentialAddedDocument,
    CredentialDeletedDocument,
    CredentialKind,
    CredentialsDocument,
    CredentialUpdatedDocument,
    DeleteCredentialDocument,
    RotateCredentialDocument,
} from '@/graphql/types';
import { useTableState } from '@/hooks/use-table-state';
import { formatTableDate } from '@/lib/utils/format';

type Credential = CredentialFragmentFragment;

const KIND_LABEL: Record<CredentialKind, string> = {
    [CredentialKind.DbPassword]: '数据库密码',
    [CredentialKind.K8sToken]: 'K8s 令牌',
    [CredentialKind.SshKey]: 'SSH 密钥',
    [CredentialKind.SshPassword]: 'SSH 密码',
};

// The SSH forms are the two the remote tools dial with; the other two are stored
// so a deployment keeps one inventory, and the label says so rather than
// pretending `ops_exec` can use them.
const KIND_NOTE: Partial<Record<CredentialKind, string>> = {
    [CredentialKind.DbPassword]: '仅保存，暂无工具引用',
    [CredentialKind.K8sToken]: '仅保存，暂无工具引用',
};

interface CreateFormState {
    host: string;
    hostKeyFingerprint: string;
    kind: CredentialKind;
    name: string;
    port: string;
    secret: string;
    username: string;
}

const EMPTY_CREATE_FORM: CreateFormState = {
    host: '',
    hostKeyFingerprint: '',
    kind: CredentialKind.SshPassword,
    name: '',
    port: '',
    secret: '',
    username: '',
};

const PASSWORD_SUBMITTED = '••••••••';

const optional = (value: string): null | string => {
    const trimmed = value.trim();

    return trimmed === '' ? null : trimmed;
};

function SettingsCredentials() {
    const { data, error, loading: isLoading, refetch } = useQuery(CredentialsDocument);
    const [createCredential, { loading: isCreating }] = useMutation(CreateCredentialDocument);
    const [rotateCredential, { loading: isRotating }] = useMutation(RotateCredentialDocument);
    const [deleteCredential] = useMutation(DeleteCredentialDocument);

    const [createOpen, setCreateOpen] = useState(false);
    const [createForm, setCreateForm] = useState<CreateFormState>(EMPTY_CREATE_FORM);
    const [rotatingCredential, setRotatingCredential] = useState<Credential | null>(null);
    const [newSecret, setNewSecret] = useState('');
    const [deletingCredential, setDeletingCredential] = useState<Credential | null>(null);

    const { filter, pageIndex, setFilter, setPage } = useTableState();

    useSubscription(CredentialAddedDocument, {
        onData: ({ client }) => client.refetchQueries({ include: ['credentials'] }),
    });
    useSubscription(CredentialUpdatedDocument, {
        onData: ({ client }) => client.refetchQueries({ include: ['credentials'] }),
    });
    useSubscription(CredentialDeletedDocument, {
        onData: ({ client }) => client.refetchQueries({ include: ['credentials'] }),
    });

    const openCreate = useCallback(() => {
        setCreateForm(EMPTY_CREATE_FORM);
        setCreateOpen(true);
    }, []);

    const submitCreate = useCallback(async () => {
        const name = createForm.name.trim();
        const secret = createForm.secret.trim();

        if (name === '' || secret === '') {
            toast.error('名称与密钥都不能为空');

            return;
        }

        const port = createForm.port.trim() === '' ? null : Number(createForm.port.trim());

        if (port !== null && (!Number.isInteger(port) || port < 0 || port > 65535)) {
            toast.error('端口必须是 0 到 65535 之间的整数');

            return;
        }

        try {
            await createCredential({
                refetchQueries: ['credentials'],
                variables: {
                    input: {
                        host: optional(createForm.host),
                        hostKeyFingerprint: optional(createForm.hostKeyFingerprint),
                        kind: createForm.kind,
                        name,
                        port,
                        secret,
                        username: optional(createForm.username),
                    },
                },
            });

            toast.success('凭据已保存');
            setCreateOpen(false);
            setCreateForm(EMPTY_CREATE_FORM);
        } catch (error) {
            toast.error('保存凭据失败', {
                description: error instanceof Error ? error.message : undefined,
            });
        }
    }, [createCredential, createForm]);

    const submitRotate = useCallback(async () => {
        if (!rotatingCredential) {
            return;
        }

        const secret = newSecret.trim();

        if (secret === '') {
            toast.error('新密钥不能为空');

            return;
        }

        try {
            await rotateCredential({
                refetchQueries: ['credentials'],
                variables: {
                    credentialId: rotatingCredential.id,
                    input: { secret },
                },
            });

            toast.success('密钥已轮换');
            setRotatingCredential(null);
            setNewSecret('');
        } catch (error) {
            toast.error('轮换密钥失败', {
                description: error instanceof Error ? error.message : undefined,
            });
        }
    }, [newSecret, rotatingCredential, rotateCredential]);

    const submitDelete = useCallback(async () => {
        if (!deletingCredential) {
            return;
        }

        try {
            await deleteCredential({
                refetchQueries: ['credentials'],
                variables: { credentialId: deletingCredential.id },
            });

            toast.success('凭据已删除');
            setDeletingCredential(null);
        } catch (error) {
            toast.error('删除凭据失败', {
                description: error instanceof Error ? error.message : undefined,
            });
        }
    }, [deleteCredential, deletingCredential]);

    const columns: ColumnDef<Credential>[] = useMemo(
        () => [
            {
                accessorKey: 'name',
                cell: ({ row }) => <div className="truncate font-medium">{row.original.name}</div>,
                enableHiding: false,
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="名称"
                    />
                ),
                meta: { searchable: true },
                size: 220,
            },
            {
                accessorKey: 'kind',
                cell: ({ row }) => {
                    const note = KIND_NOTE[row.original.kind];

                    return (
                        <div className="flex items-center gap-2">
                            <Badge variant="secondary">{KIND_LABEL[row.original.kind] ?? row.original.kind}</Badge>
                            {note ? <span className="text-muted-foreground text-xs">{note}</span> : null}
                        </div>
                    );
                },
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="类型"
                    />
                ),
                meta: { searchable: true },
                size: 200,
            },
            {
                accessorKey: 'host',
                cell: ({ row }) => {
                    const { host, port } = row.original;

                    if (!host) {
                        return <span className="text-muted-foreground text-sm">—</span>;
                    }

                    return (
                        <code className="text-sm">
                            {host}
                            {port ? `:${port}` : ''}
                        </code>
                    );
                },
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="主机"
                    />
                ),
                meta: { searchable: true },
                size: 180,
            },
            {
                accessorKey: 'username',
                cell: ({ row }) =>
                    row.original.username ? (
                        <span className="text-sm">{row.original.username}</span>
                    ) : (
                        <span className="text-muted-foreground text-sm">—</span>
                    ),
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="用户名"
                    />
                ),
                meta: { searchable: true },
                size: 140,
            },
            {
                accessorKey: 'rotatedAt',
                cell: ({ row }) => (
                    <div className="text-sm whitespace-nowrap">{formatTableDate(new Date(row.original.rotatedAt))}</div>
                ),
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="轮换时间"
                    />
                ),
                size: 150,
                sortingFn: (rowA, rowB) =>
                    new Date(rowA.original.rotatedAt).getTime() - new Date(rowB.original.rotatedAt).getTime(),
            },
            {
                accessorKey: 'lastUsedAt',
                cell: ({ row }) =>
                    row.original.lastUsedAt ? (
                        <div className="text-sm whitespace-nowrap">
                            {formatTableDate(new Date(row.original.lastUsedAt))}
                        </div>
                    ) : (
                        <span className="text-muted-foreground text-sm">从未使用</span>
                    ),
                header: ({ column }) => (
                    <DataTableColumnHeader
                        column={column}
                        title="最近使用"
                    />
                ),
                size: 150,
                sortingFn: (rowA, rowB) => {
                    const left = rowA.original.lastUsedAt ? new Date(rowA.original.lastUsedAt).getTime() : 0;
                    const right = rowB.original.lastUsedAt ? new Date(rowB.original.lastUsedAt).getTime() : 0;

                    return left - right;
                },
            },
            {
                cell: ({ row }) => (
                    <div className="flex justify-end opacity-0 transition-opacity group-hover:opacity-100">
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <Button
                                    aria-label="打开菜单"
                                    size="icon-sm"
                                    variant="ghost"
                                >
                                    <Ellipsis />
                                </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent
                                align="end"
                                className="min-w-28"
                            >
                                <DropdownMenuItem onClick={() => setRotatingCredential(row.original)}>
                                    <RotateCcw />
                                    轮换密钥
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => setDeletingCredential(row.original)}>
                                    <Trash />
                                    删除
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </div>
                ),
                enableHiding: false,
                header: () => null,
                id: 'actions',
                meta: { preventRowClick: true },
                size: 48,
            },
        ],
        [],
    );

    const pageHeader = (
        <AppHeader>
            <AppHeaderContent>
                <AppHeaderTitle icon={<KeyRound className="size-4 shrink-0" />}>凭据</AppHeaderTitle>
            </AppHeaderContent>
            <AppHeaderActions>
                <AppHeaderAction
                    icon={<Plus />}
                    label="新建凭据"
                    onClick={openCreate}
                    variant="secondary"
                />
            </AppHeaderActions>
        </AppHeader>
    );

    const renderBody = () => {
        if (isLoading && !data) {
            return (
                <div className="flex flex-1 flex-col gap-4 p-4">
                    <LoadingState
                        description="正在读取凭据"
                        title="加载中…"
                    />
                </div>
            );
        }

        if (error && !data) {
            return (
                <div className="flex flex-1 flex-col gap-4 p-4">
                    <ErrorState
                        message={error.message}
                        onRetry={refetch}
                        title="凭据加载失败"
                    />
                </div>
            );
        }

        const credentials = data?.credentials ?? [];

        if (credentials.length === 0) {
            return (
                <div className="flex flex-1 flex-col gap-4 p-4">
                    <Empty>
                        <EmptyHeader>
                            <EmptyMedia variant="icon">
                                <ShieldCheck />
                            </EmptyMedia>
                            <EmptyTitle>还没有凭据</EmptyTitle>
                            <EmptyDescription>
                                凭据加密后存库，所有会话共用；目标主机通过凭据 ID 引用它，明文只在连接时于内存中解开。
                            </EmptyDescription>
                        </EmptyHeader>
                        <EmptyContent>
                            <Button
                                onClick={openCreate}
                                variant="secondary"
                            >
                                <Plus />
                                新建凭据
                            </Button>
                        </EmptyContent>
                    </Empty>
                </div>
            );
        }

        return (
            <div className="flex flex-1 flex-col gap-4 p-4">
                <DataTable<Credential>
                    columns={columns}
                    data={credentials}
                    empty={{ entityName: '凭据' }}
                    filterPlaceholder="搜索凭据…"
                    filterValue={filter}
                    label="凭据"
                    onFilterChange={setFilter}
                    onPageChange={setPage}
                    pageIndex={pageIndex}
                />
            </div>
        );
    };

    return (
        <>
            {pageHeader}
            {renderBody()}

            <Dialog
                onOpenChange={setCreateOpen}
                open={createOpen}
            >
                <DialogContent className="sm:max-w-lg">
                    <DialogHeader>
                        <DialogTitle>新建凭据</DialogTitle>
                        <DialogDescription>
                            密钥加密后存库，不会通过接口回读；保存后只能轮换，不能取回原文。
                        </DialogDescription>
                    </DialogHeader>
                    <div className="grid gap-3">
                        <div className="grid gap-1.5">
                            <Label htmlFor="credential-name">名称</Label>
                            <Input
                                autoComplete="off"
                                id="credential-name"
                                onChange={(event) => setCreateForm((prev) => ({ ...prev, name: event.target.value }))}
                                placeholder="例如 lab-root"
                                value={createForm.name}
                            />
                        </div>
                        <div className="grid gap-1.5">
                            <Label htmlFor="credential-kind">类型</Label>
                            <Select
                                onValueChange={(value) =>
                                    setCreateForm((prev) => ({ ...prev, kind: value as CredentialKind }))
                                }
                                value={createForm.kind}
                            >
                                <SelectTrigger
                                    aria-label="凭据类型"
                                    id="credential-kind"
                                >
                                    <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                    <SelectGroup>
                                        {Object.values(CredentialKind).map((kind) => (
                                            <SelectItem
                                                key={kind}
                                                value={kind}
                                            >
                                                {KIND_LABEL[kind]}
                                            </SelectItem>
                                        ))}
                                    </SelectGroup>
                                </SelectContent>
                            </Select>
                        </div>
                        <div className="grid gap-1.5">
                            <Label htmlFor="credential-secret">密钥</Label>
                            <Textarea
                                autoComplete="off"
                                className="min-h-24 font-mono text-xs"
                                id="credential-secret"
                                onChange={(event) => setCreateForm((prev) => ({ ...prev, secret: event.target.value }))}
                                placeholder={PASSWORD_SUBMITTED}
                                value={createForm.secret}
                            />
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                            <div className="grid gap-1.5">
                                <Label htmlFor="credential-host">主机（可选）</Label>
                                <Input
                                    autoComplete="off"
                                    id="credential-host"
                                    onChange={(event) =>
                                        setCreateForm((prev) => ({ ...prev, host: event.target.value }))
                                    }
                                    placeholder="10.0.0.5"
                                    value={createForm.host}
                                />
                            </div>
                            <div className="grid gap-1.5">
                                <Label htmlFor="credential-port">端口（可选）</Label>
                                <Input
                                    autoComplete="off"
                                    id="credential-port"
                                    inputMode="numeric"
                                    onChange={(event) =>
                                        setCreateForm((prev) => ({ ...prev, port: event.target.value }))
                                    }
                                    placeholder="22"
                                    value={createForm.port}
                                />
                            </div>
                        </div>
                        <div className="grid gap-1.5">
                            <Label htmlFor="credential-username">用户名（可选）</Label>
                            <Input
                                autoComplete="off"
                                id="credential-username"
                                onChange={(event) =>
                                    setCreateForm((prev) => ({ ...prev, username: event.target.value }))
                                }
                                placeholder="root"
                                value={createForm.username}
                            />
                        </div>
                        <div className="grid gap-1.5">
                            <Label htmlFor="credential-fingerprint">主机密钥指纹（可选）</Label>
                            <Input
                                autoComplete="off"
                                className="font-mono text-xs"
                                id="credential-fingerprint"
                                onChange={(event) =>
                                    setCreateForm((prev) => ({ ...prev, hostKeyFingerprint: event.target.value }))
                                }
                                placeholder="SHA256:…"
                                value={createForm.hostKeyFingerprint}
                            />
                        </div>
                    </div>
                    <DialogFooter>
                        <Button
                            onClick={() => setCreateOpen(false)}
                            variant="outline"
                        >
                            取消
                        </Button>
                        <Button
                            disabled={isCreating}
                            onClick={submitCreate}
                            variant="secondary"
                        >
                            {isCreating ? <Spinner variant="circle" /> : <Plus />}
                            保存
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Dialog
                onOpenChange={(open) => {
                    if (!open) {
                        setRotatingCredential(null);
                        setNewSecret('');
                    }
                }}
                open={rotatingCredential !== null}
            >
                <DialogContent>
                    <DialogHeader>
                        <DialogTitle>轮换密钥</DialogTitle>
                        <DialogDescription>
                            为「{rotatingCredential?.name}」换一份新密钥。原密钥立即失效，不会保留副本。
                        </DialogDescription>
                    </DialogHeader>
                    <div className="grid gap-1.5">
                        <Label htmlFor="credential-new-secret">新密钥</Label>
                        <Textarea
                            autoComplete="off"
                            className="min-h-24 font-mono text-xs"
                            id="credential-new-secret"
                            onChange={(event) => setNewSecret(event.target.value)}
                            value={newSecret}
                        />
                    </div>
                    <DialogFooter>
                        <Button
                            onClick={() => {
                                setRotatingCredential(null);
                                setNewSecret('');
                            }}
                            variant="outline"
                        >
                            取消
                        </Button>
                        <Button
                            disabled={isRotating}
                            onClick={submitRotate}
                            variant="secondary"
                        >
                            {isRotating ? <Spinner variant="circle" /> : <Pencil />}
                            轮换
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <ConfirmationDialog
                cancelText="取消"
                confirmText="删除"
                handleConfirm={submitDelete}
                handleOpenChange={(open) => {
                    if (!open) {
                        setDeletingCredential(null);
                    }
                }}
                isOpen={deletingCredential !== null}
                itemName={deletingCredential?.name}
                itemType="凭据"
            />
        </>
    );
}

export default SettingsCredentials;
