import { useCallback, useState } from 'react';
import { toast } from 'sonner';
import { z } from 'zod';

import type { OverwriteOutcome } from '@/components/shared/overwrite';

import { pluralizeItems } from '@/components/shared/file-manager';
import { api, getApiErrorMessage, getApiErrorStatusCode } from '@/lib/axios';

import { RESOURCES_COPY_API_PATH } from './resources-constants';

export const resourcesCopyFormSchema = z.object({
    destination: z
        .string()
        .trim()
        .min(1, { message: '目标目录不能为空' })
        .refine((value) => !value.startsWith('/'), { message: '目标必须是相对路径' })
        .refine((value) => !value.split('/').includes('..'), { message: '目标路径不能包含 ".."' }),
});

export type ResourcesCopyFormValues = z.infer<typeof resourcesCopyFormSchema>;

interface CopyRequestBody {
    destination: string;
    force: boolean;
    sources: readonly string[];
}

interface UseResourcesCopyResult {
    /**
     * Issue a batch copy in a single atomic request and return a discriminated outcome:
     *   - `ok`        — every source was copied (success toast already fired),
     *   - `conflict`  — at least one destination is occupied (no toast, caller
     *                   resolves via the shared overwrite workflow),
     *   - `error`     — anything else (failure toast already fired).
     *
     * Backend semantics: with one source, `destination` is the exact target
     * path; with multiple sources, `destination` is a base directory and each
     * source lands at `destination/<basename>`. Either way, the whole batch
     * executes inside one DB transaction.
     */
    copy: (sources: readonly string[], destination: string, force: boolean) => Promise<OverwriteOutcome>;
    isCopying: boolean;
}

/** Wraps `POST /resources/copy` for single and batch copy operations. */
export function useResourcesCopy(): UseResourcesCopyResult {
    const [isCopying, setIsCopying] = useState(false);

    const copy = useCallback(
        async (sources: readonly string[], destination: string, force: boolean): Promise<OverwriteOutcome> => {
            if (sources.length === 0) {
                return { kind: 'error' };
            }

            setIsCopying(true);

            try {
                await api.post<void, CopyRequestBody>(RESOURCES_COPY_API_PATH, {
                    destination,
                    force,
                    sources,
                });

                const description =
                    sources.length === 1
                        ? `已复制到 /${destination}`
                        : `已复制 ${sources.length} ${pluralizeItems(sources.length)}到 /${destination}`;

                toast.success('资源已复制', { description });

                return { kind: 'ok' };
            } catch (error) {
                if (!force && getApiErrorStatusCode(error) === 409) {
                    return { kind: 'conflict' };
                }

                const description = getApiErrorMessage(error, '复制资源失败');

                toast.error('复制失败', { description });

                return { kind: 'error' };
            } finally {
                setIsCopying(false);
            }
        },
        [],
    );

    return {
        copy,
        isCopying,
    };
}
