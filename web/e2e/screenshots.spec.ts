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

async function shoot(page: Page, testInfo: TestInfo, name: string, { fullPage = false } = {}) {
  const width = testInfo.project.name === 'mobile' ? 390 : 1440;
  await page.evaluate(() => document.fonts.ready);
  if (fullPage) {
    // A viewport as tall as the page, so fixed parts (dock, tab bar) sit at the bottom as in the design exports
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    await page.setViewportSize({ width, height });
    await page.waitForTimeout(300);
  }
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

test.describe('system states and settings', () => {
  test('settings playback', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=playback');
    await page.locator('code[aria-label="WebDAV address"]').waitFor();
    await shoot(page, testInfo, 'settings/playback');
  });

  test('settings status', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=status');
    await page.getByRole('list', { name: 'Services' }).waitFor();
    await shoot(page, testInfo, 'settings/status');
  });

  test('settings list (phone)', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile', 'Phone only');
    await signIn(page, '/settings');
    await page.getByText('WebDAV ready').waitFor();
    await shoot(page, testInfo, 'settings/list');
  });

  test('not found', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.goto('/no/such/page');
    await page.getByRole('heading', { name: 'Nothing here but the cat' }).waitFor();
    await shoot(page, testInfo, 'system/notfound');
  });

  test('offline', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'backend-down'));
    await signIn(page, '/library');
    await page.goto('/offline?from=%2Flibrary');
    await page.getByText(/Trying again in/).waitFor();
    await shoot(page, testInfo, 'system/offline');
  });

  test('offline banner', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'backend-down'));
    await signIn(page, '/library');
    await page.getByText('Can’t reach momoshtrem').waitFor();
    await shoot(page, testInfo, 'system/banner');
  });
});

test.describe('movie page', () => {
  for (const [state, tmdbId, panel] of [
    ['none', 666277, 'Not in your library'],
    ['needs_torrent', 1054867, 'In your library · no torrent yet'],
    ['downloading', 157336, 'Downloading'],
  ] as const) {
    test(state, async ({ page }, testInfo) => {
      await signIn(page, `/movie/${tmdbId}`);
      await page.getByRole('heading', { name: panel }).waitFor();
      await shoot(page, testInfo, `movie/${state}`, { fullPage: true });
    });
  }

  test('ready', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    const subtitles = page.getByRole('region', { name: 'Subtitles' });
    await subtitles.getByRole('button', { name: 'Search' }).click();
    await subtitles.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' }).waitFor();
    await shoot(page, testInfo, 'movie/ready', { fullPage: true });
  });

  test('how to play', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    await page.getByRole('heading', { name: 'Ready in Infuse' }).waitFor();
    await page.getByRole('button', { name: testInfo.project.name === 'mobile' ? 'Play in Infuse' : 'How to play in Infuse' }).click();
    await page.getByLabel('WebDAV path of this title').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'movie/howto');
  });
});
