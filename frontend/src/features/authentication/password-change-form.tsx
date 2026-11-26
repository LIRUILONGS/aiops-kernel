import { type ComponentProps, useState } from 'react';
import { toast } from 'sonner';
import * as z from 'zod';

import { Button } from '@/components/ui/button';
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { FormSubmitButton } from '@/components/ui/form-submit-button';
import { InputPassword } from '@/components/ui/input-password';
import { useAppForm } from '@/hooks/use-app-form';
import { api, resolveApiErrorMessage } from '@/lib/axios';
import { cn } from '@/lib/utils';

const passwordChangeFormSchema = z
    .object({
        confirmPassword: z.string().min(1, { message: '请再次输入密码' }),
        currentPassword: z.string().min(1, { message: '请输入当前密码' }),
        newPassword: z
            .string()
            .min(8, { message: '密码至少 8 个字符' })
            // bcrypt, which hashes it server-side, refuses anything longer than 72 bytes.
            .refine((password) => new TextEncoder().encode(password).length <= 72, {
                message: '密码不能超过 72 个字符',
            })
            .refine(
                (password) => {
                    const length = [...password].length;

                    if (length > 15) {
                        return true;
                    }

                    return (
                        length >= 8 &&
                        /[0-9]/.test(password) &&
                        /[a-z]/.test(password) &&
                        /[A-Z]/.test(password) &&
                        /[!@#$&*]/.test(password)
                    );
                },
                {
                    message:
                        '密码要么超过 15 个字符，要么至少 8 个字符且同时包含数字、小写字母、大写字母和特殊字符（!@#$&*）',
                },
            ),
    })
    .refine((data) => data.newPassword === data.confirmPassword, {
        message: '两次输入的密码不一致',
        path: ['confirmPassword'],
    })
    .refine((data) => data.currentPassword !== data.newPassword, {
        message: '新密码不能与当前密码相同',
        path: ['newPassword'],
    });

const ERROR_BY_CODE: Record<string, string> = {
    'Users.ChangePasswordCurrentUser.InvalidCurrentPassword': '当前密码不正确',
    'Users.ChangePasswordCurrentUser.InvalidNewPassword': '新密码不符合要求',
    'Users.ChangePasswordCurrentUser.InvalidPassword': '密码校验失败',
    'Users.NotFound': '用户不存在',
};

interface PasswordChangeFormProps {
    buttonSize?: ComponentProps<typeof Button>['size'];
    layout?: 'horizontal' | 'vertical';
    onCancel?: () => void;
    onSkip?: () => void;
    onSuccess?: () => void;
}

type PasswordChangeFormValues = z.infer<typeof passwordChangeFormSchema>;

export function PasswordChangeForm({
    buttonSize = 'default',
    layout = 'horizontal',
    onCancel,
    onSkip,
    onSuccess,
}: PasswordChangeFormProps) {
    const [error, setError] = useState<null | string>(null);

    const form = useAppForm<PasswordChangeFormValues>({
        defaultValues: {
            confirmPassword: '',
            currentPassword: '',
            newPassword: '',
        },
        schema: passwordChangeFormSchema,
    });

    const handleSubmit = async (values: PasswordChangeFormValues) => {
        setError(null);

        try {
            await api.put('/user/password', {
                confirm_password: values.confirmPassword,
                current_password: values.currentPassword,
                password: values.newPassword,
            });

            form.reset();
            toast.success('密码已修改');

            onSuccess?.();
        } catch (err: unknown) {
            setError(resolveApiErrorMessage(err, ERROR_BY_CODE, '修改密码失败'));
        }
    };

    const isVertical = layout === 'vertical';

    const skipButton = onSkip && (
        <Button
            className={cn('text-muted-foreground', isVertical && 'w-full')}
            onClick={onSkip}
            size={buttonSize}
            type="button"
            variant="ghost"
        >
            暂时跳过
        </Button>
    );
    const cancelButton = onCancel && (
        <Button
            className={cn(isVertical && 'w-full')}
            onClick={onCancel}
            size={buttonSize}
            type="button"
            variant="outline"
        >
            取消
        </Button>
    );
    const submitButton = (
        <FormSubmitButton
            className={cn(isVertical && 'w-full')}
            size={buttonSize}
        >
            <span>更新密码</span>
        </FormSubmitButton>
    );

    return (
        <Form {...form}>
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
                    name="newPassword"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>新密码</FormLabel>
                            <FormControl>
                                <InputPassword
                                    {...field}
                                    placeholder="请输入新密码"
                                />
                            </FormControl>
                            <FormDescription className="text-xs">
                                至少 16 个字符；8–15 个字符时需包含数字、小写字母、大写字母以及特殊字符 (!@#$&*)
                            </FormDescription>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormField
                    control={form.control}
                    name="confirmPassword"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>确认新密码</FormLabel>
                            <FormControl>
                                <InputPassword
                                    {...field}
                                    placeholder="请再次输入新密码"
                                />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                {error && <div className="text-destructive text-sm">{error}</div>}

                {isVertical ? (
                    <div className="flex flex-col gap-2 pt-2">
                        {submitButton}
                        {cancelButton}
                        {skipButton}
                    </div>
                ) : (
                    <div className="flex justify-end gap-2 pt-2">
                        {skipButton}
                        {cancelButton}
                        {submitButton}
                    </div>
                )}
            </form>
        </Form>
    );
}
