import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { E2E_USER, signIn } from './helpers/auth';

/**
 * PR screenshots at the design's check widths (390 phone, 1440 desktop),
 * written to docs/screenshots/v2/. Skipped in normal runs:
 *   SCREENSHOTS=1 npx playwright test screenshots && node scripts/optimize-screenshots.mjs
 */
test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

const OUT = path.resolve(__dirname, '../../docs/screenshots/v2');

async function shoot(page: Page, testInfo: TestInfo, name: string) {
  const width = testInfo.project.name === 'mobile' ? 390 : 1440;
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: path.join(OUT, `${name}-${width}.png`), animations: 'disabled' });
}

test.describe('sign in', () => {
  test('default', async ({ page }, testInfo) => {
    await page.goto('/login');
    await shoot(page, testInfo, 'signin/default');
  });

  test('error', async ({ page }, testInfo) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill(E2E_USER.username);
    await page.getByLabel('Password', { exact: true }).fill('correct-horse-battery');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText('Wrong username or password')).toBeVisible();
    await shoot(page, testInfo, 'signin/error');
  });

  test('rate limited', async ({ page }, testInfo) => {
    await page.goto('/login?error=rate_limited&retry=272');
    await expect(page.getByText('Too many attempts')).toBeVisible();
    await shoot(page, testInfo, 'signin/ratelimited');
  });
});

test.describe('app shell', () => {
  test('frame', async ({ page }, testInfo) => {
    await signIn(page, '/torrents');
    await expect(page.getByText('Frieren.S02.1080p.WEB-DL.x264-SubsPlease')).toBeVisible();
    await shoot(page, testInfo, 'shell/frame');
  });

  test('account menu', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const scope = testInfo.project.name === 'desktop' ? page.locator('header.topbar') : page.locator('header.phone-bar');
    await scope.getByRole('button', { name: /Account menu/ }).click();
    await page.getByText('Signed in').waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'shell/menu');
  });

  test('command palette', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'Desktop affordance');
    await signIn(page, '/library');
    await page.keyboard.press('ControlOrMeta+k');
    await page.getByRole('dialog', { name: 'Search or jump to' }).waitFor();
    await page.waitForTimeout(300);
    await shoot(page, testInfo, 'shell/palette');
  });

  test('infuse sheet', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.goto('/library?sheet=infuse-setup');
    await page.getByLabel('WebDAV address').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'shell/sheet');
  });
});
