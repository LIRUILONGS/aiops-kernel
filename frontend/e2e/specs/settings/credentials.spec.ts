import { expect, test } from '../../fixtures/test.ts';
import { expectCleanPage } from '../../helpers/errors.ts';
import {
    CREATED_CREDENTIAL,
    credentialsCassette,
    credentialsList,
    SEED_CREDENTIAL,
} from '../../mocks/cassettes/settings-credentials.ts';

const createdCassette = () =>
    credentialsCassette({
        mutations: {
            createCredential: [{ data: { createCredential: CREATED_CREDENTIAL } }],
        },
        queries: {
            credentials: [
                { data: credentialsList(SEED_CREDENTIAL) },
                { data: credentialsList(CREATED_CREDENTIAL, SEED_CREDENTIAL) },
            ],
        },
    });

test.describe('settings credentials', { tag: '@coverage' }, () => {
    test.use({ cassette: credentialsCassette() });

    test('lists the stored credentials with their kind and host', async ({ page, pageErrorLog }) => {
        await page.goto('/settings/credentials');

        await expect(page.getByRole('table', { name: '凭据' })).toBeVisible();

        const row = page.getByRole('row', { name: /E2E lab key/ });

        await expect(row).toBeVisible();
        await expect(row).toContainText('SSH 密钥');
        await expect(row).toContainText('10.0.0.5');

        expectCleanPage(pageErrorLog);
    });

    test('opens the create form without ever sending a readable secret back', async ({ page }) => {
        await page.goto('/settings/credentials');

        await page.getByRole('button', { name: '新建凭据' }).first().click();

        const dialog = page.getByRole('dialog');

        await expect(dialog.getByRole('heading', { name: '新建凭据' })).toBeVisible();
        await expect(
            dialog.getByText('密钥加密后存库，不会通过接口回读；保存后只能轮换，不能取回原文。'),
        ).toBeVisible();
        await expect(dialog.getByLabel('名称')).toHaveValue('');
        await expect(dialog.getByLabel('密钥', { exact: true })).toHaveValue('');
    });
});

test.describe('settings credentials, creating one', { tag: '@coverage' }, () => {
    test.use({ cassette: createdCassette() });

    test('shows the new credential without ever rendering the readable secret', async ({ page, pageErrorLog }) => {
        await page.goto('/settings/credentials');

        await expect(page.getByRole('row', { name: /E2E lab key/ })).toBeVisible();

        await page.getByRole('button', { name: '新建凭据' }).first().click();

        const dialog = page.getByRole('dialog');

        await dialog.getByLabel('名称').fill('E2E new key');
        await dialog.getByLabel('密钥', { exact: true }).fill('-----BEGIN OPENSSH PRIVATE KEY-----');
        await dialog.getByLabel('主机（可选）').fill('10.0.0.7');
        await dialog.getByRole('button', { name: '保存' }).click();

        await expect(dialog).toHaveCount(0);
        await expect(page.getByRole('row', { name: /E2E new key/ })).toBeVisible();
        // The secret never comes back through the API, so it cannot appear anywhere on the page.
        await expect(page.getByText('-----BEGIN OPENSSH PRIVATE KEY-----')).toHaveCount(0);

        expectCleanPage(pageErrorLog);
    });
});
