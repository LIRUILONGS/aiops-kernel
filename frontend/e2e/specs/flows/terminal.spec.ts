import { expect, test } from '../../fixtures/test.ts';
import { expectCleanPage } from '../../helpers/errors.ts';
import { readTerminalBuffer } from '../../helpers/terminal.ts';
import { flowsCassette, TERMINAL_OUTPUT } from '../../mocks/cassettes/flows.ts';

test.describe('flow terminal', { tag: '@flows' }, () => {
    test.use({ cassette: flowsCassette() });

    test('renders sandbox output readable through the xterm buffer', async ({ page, pageErrorLog }) => {
        await page.goto('/flows/5?tab=terminal');
        await expect(page.getByText(/E2E Alpha/).first()).toBeVisible();

        await expect(page.locator('.xterm')).toBeVisible();
        await expect(async () => {
            expect(await readTerminalBuffer(page)).toContain(TERMINAL_OUTPUT);
        }).toPass();
        expect(await readTerminalBuffer(page)).toContain('uname -a');
        expectCleanPage(pageErrorLog);
    });
});
