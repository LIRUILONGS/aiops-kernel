import { useState } from 'react';
import { toast } from 'sonner';
import * as z from 'zod';

import { Button } from '@/components/ui/button';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { FormSubmitButton } from '@/components/ui/form-submit-button';
import { Input } from '@/components/ui/input';
import { InputPassword } from '@/components/ui/input-password';
import { useAppForm } from '@/hooks/use-app-form';
import { api, resolveApiErrorMessage } from '@/lib/axios';
import { useUser } from '@/providers/user-provider';

const emailChangeFormSchema = z.object({
    currentPassword: z.string().min(1, { message: '请输入当前密码' }),
    newEmail: z
        .string()
        .trim()
        .toLowerCase()
        .min(1, { message: '请输入邮箱' })
        .email({ message: '邮箱格式不正确' })
        .max(50, { message: '邮箱不能超过 50 个字符' }),
});

const ERROR_BY_CODE: Record<string, string> = {
    'Users.ChangeEmailCurrentUser.EmailAlreadyExists': '该邮箱已被使用',
    'Users.ChangeEmailCurrentUser.InvalidCurrentPassword': '当前密码不正确',
    'Users.ChangeEmailCurrentUser.InvalidEmail': '新邮箱不符合要求',
    'Users.NotFound': '用户不存在',
};

interface EmailChangeFormProps {
    onCancel?: () => void;
    onSuccess?: () => void;
}

type EmailChangeFormValues = z.infer<typeof emailChangeFormSchema>;

export function EmailChangeForm({ onCancel, onSuccess }: EmailChangeFormProps) {
    const [error, setError] = useState<null | string>(null);
    const { patchUser, refreshAuthInfo } = useUser();

    const form = useAppForm<EmailChangeFormValues>({
        defaultValues: {
            currentPassword: '',
            newEmail: '',
        },
        schema: emailChangeFormSchema,
    });

    const handleSubmit = async (values: EmailChangeFormValues) => {
        setError(null);

        try {
            await api.put('/user/email', {
                current_password: values.currentPassword,
                mail: values.newEmail,
            });

            form.reset();
            toast.success('邮箱已更新');

            patchUser({ mail: values.newEmail });
            await refreshAuthInfo();

            onSuccess?.();
        } catch (err: unknown) {
            setError(resolveApiErrorMessage(err, ERROR_BY_CODE, '更新邮箱失败'));
        }
    };

    return (
        <Form {...form}>
            {/* noValidate: the type="email" field would otherwise fire the browser's native (locale-styled)
                validation popup on submit, pre-empting our zod message. Validation runs through zod instead. */}
            <form
                className="flex flex-col gap-4"
                noValidate
                onSubmit={form.handleSubmit(handleSubmit)}
            >
                <FormField
                    control={form.control}
                    name="currentPassword"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>当前密码</FormLabel>
                            <FormControl>
                                <InputPassword
                                    {...field}
                                    placeholder="请输入当前密码"
                                />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormField
                    control={form.control}
                    name="newEmail"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>新邮箱</FormLabel>
                            <FormControl>
                                <Input
                                    {...field}
                                    placeholder="请输入新的邮箱地址"
                                    type="email"
                                />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                {error && <div className="text-destructive text-sm">{error}</div>}

                <div className="flex justify-end gap-2 pt-2">
                    {onCancel && (
                        <Button
                            onClick={onCancel}
                            size="sm"
                            type="button"
                            variant="outline"
                        >
                            取消
                        </Button>
                    )}
                    <FormSubmitButton size="sm">
                        <span>更新邮箱</span>
                    </FormSubmitButton>
                </div>
            </form>
        </Form>
    );
}
