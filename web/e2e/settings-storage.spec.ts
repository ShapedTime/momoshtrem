import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Settings › Storage (#205), against the MSW mocks: a 1 TB disk with the limit
 * on auto (750 GB), smart downloads on and the seeded downloads (86.4 GB kept).
 * Scenarios in src/mocks/scenario.ts: storage-config (config.yaml sets 56.4 GB),
 * downloads-empty (a fresh install: nothing kept or picked yet).
 */

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
  await expectNoA11yViolations(page);
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
const saveBar = (page: Page) => page.getByRole('region', { name: 'Unsaved changes' });
const save = (page: Page) => saveBar(page).getByRole('button', { name: 'Save' });
const limitMode = (page: Page, option: 'Auto' | 'Custom') =>
  page.getByRole('group', { name: 'Storage limit' }).getByRole('button', { name: option, exact: true });
const customLimit = (page: Page) => page.getByRole('textbox', { name: 'Custom limit' });
const keptCap = (page: Page) => page.getByRole('textbox', { name: 'Space for kept downloads' });
const diskBar = (page: Page) => page.getByRole('img', { name: /^Disk of / });
const putStorage = (page: Page) => page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/api/settings/storage'));

/** Count PUTs from here on, to check that what can't be sent isn't. */
function countPuts(page: Page) {
  const puts = { count: 0 };
  page.on('request', (request) => {
    if (request.method() === 'PUT' && request.url().endsWith('/api/settings/storage')) puts.count += 1;
  });
  return puts;
}

/** The section list: beside the section on desktop; on phone, Back to it (client-side, so the mocks keep what was saved). */
async function sectionNav(page: Page) {
  const back = page.getByRole('link', { name: 'Settings', exact: true });
  if (await back.isVisible()) await back.click();
  return page.getByRole('navigation', { name: 'Settings sections' });
}

async function openSection(page: Page) {
  await signIn(page, '/settings?section=storage');
  await expect(page.getByRole('heading', { name: 'Storage', level: 2 })).toBeVisible();
  await expect(diskBar(page)).toBeVisible();
}

test.describe('Settings › Storage', () => {
  test('the disk, the limit and the downloads; Save sends only what changed, never the limit', async ({ page }) => {
    await openSection(page);
    await expect(page.getByText('How much of the disk momoshtrem may use, and what it downloads in the background.')).toBeVisible();
    await expect(diskBar(page)).toHaveAttribute(
      'aria-label',
      'Disk of 1.0 TB: Kept 48.8 GB, Smart 1.9 GB, Cache 11.7 GB, Other files 126 GB, Free 812 GB. Limit 750 GB.'
    );
    await expect(page.getByText('62.4 GB', { exact: true })).toBeVisible();
    await expect(page.getByText('The cache always leaves 5.4 GB of the disk free.')).toBeVisible();
    await expect(limitMode(page, 'Auto')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#st-limit-help')).toHaveText('Auto (75% of the disk) · 750 GB. It follows the disk when it grows.');
    await expect(customLimit(page)).toBeHidden();
    await expect(keptCap(page)).toHaveValue('');
    await expect(keptCap(page)).toHaveAccessibleDescription('Empty: half the limit. Now 375 GB; kept downloads need 86.4 GB.');
    await expect(page.getByText('always leave 21.5 GB of it for streaming', { exact: false })).toBeVisible();
    await expect(page.getByRole('combobox', { name: 'Keep for at most' })).toHaveValue('30');
    await expect(page.getByText('New keeps last until you watch them, 30 days at most.', { exact: false })).toBeVisible();
    await expect(page.getByRole('switch', { name: 'Smart downloads' })).toBeChecked();
    await expect(page.getByRole('switch', { name: 'Pause while streaming' })).not.toBeChecked();
    await expect(page.getByText('Any time. Add a window to download only then, in the server’s time zone.')).toBeVisible();
    await expect(saveBar(page)).toBeHidden();
    await expectAccessibleAndContained(page);

    await page.getByRole('switch', { name: 'Pause while streaming' }).check();
    await page.getByRole('group', { name: 'Episodes ahead' }).getByRole('button', { name: '3', exact: true }).click();
    await page.getByRole('combobox', { name: 'Keep for at most' }).selectOption('90');
    await expect(page.getByText('New keeps last until you watch them, 90 days at most.', { exact: false })).toBeVisible();
    await expect(saveBar(page)).toBeVisible();
    await expectAccessibleAndContained(page);

    const put = putStorage(page);
    await save(page).click();
    expect((await put).postDataJSON()).toEqual({ kept: { default_days: 90 }, smart: { episodes_ahead: 3 }, background: { pause_while_streaming: true } });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(saveBar(page)).toBeHidden();
    await expect(page.getByRole('switch', { name: 'Pause while streaming' })).toBeChecked();
    await expect((await sectionNav(page)).getByRole('link', { name: /Storage/ })).toContainText('750 GB · auto');
  });

  test('a custom limit: out of range stays here, then saves; the floor then holds the cache', async ({ page }) => {
    await openSection(page);
    const puts = countPuts(page);
    await limitMode(page, 'Custom').click();
    await expect(customLimit(page)).toHaveValue('750');
    await expect(customLimit(page)).toHaveAccessibleDescription('From 4.3 GB to 900 GB (90% of the disk).');

    await customLimit(page).fill('3');
    await save(page).click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Custom limit” above.');
    await expect(customLimit(page)).toHaveAttribute('aria-invalid', 'true');
    await expect(customLimit(page)).toHaveAccessibleDescription(/^Enter a size from 4\.3 GB to 900 GB\./);
    await expect(customLimit(page)).toBeFocused();
    await expectAccessibleAndContained(page);
    expect(puts.count).toBe(0);

    await customLimit(page).fill('900');
    await expect(customLimit(page)).not.toHaveAttribute('aria-invalid');
    const put = putStorage(page);
    await save(page).click();
    expect((await put).postDataJSON()).toEqual({ limit_bytes: 900e9 });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(page.locator('#st-limit-help')).toHaveText('Custom: the cache stays within the size below.');
    await expect(page.getByText('Holding the cache at 869 GB to keep 5.4 GB of the disk free.')).toBeVisible();
    await expect(diskBar(page)).toHaveAttribute('aria-label', /Limit 900 GB\.$/);
    await expect((await sectionNav(page)).getByRole('link', { name: /Storage/ })).toContainText('900 GB · custom');
  });

  test('a limit below what kept downloads need answers 409 in words', async ({ page }) => {
    await openSection(page);
    await limitMode(page, 'Custom').click();
    await customLimit(page).fill('100');
    await save(page).click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Kept downloads need 86.4 GB. Stop keeping some, or set a limit that leaves room for them.');
    await expect(limitMode(page, 'Custom')).toHaveAttribute('aria-pressed', 'true');
    await expectAccessibleAndContained(page);
  });

  test('a 422 on a cap shows on its field in words, and focus moves there', async ({ page }) => {
    await openSection(page);
    await keptCap(page).fill('740');
    await save(page).click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Space for kept downloads” above.');
    await expect(keptCap(page)).toHaveAttribute('aria-invalid', 'true');
    await expect(keptCap(page)).toHaveAccessibleDescription(
      /^Kept and smart downloads together get the limit less the streaming reserve\. Enter less here, or lower the space for smart downloads\./
    );
    await expect(page.getByText('kept.max_bytes', { exact: false })).toHaveCount(0);
    await expect(keptCap(page)).toBeFocused();
    await expect(keptCap(page)).toBeInViewport();
    await expectAccessibleAndContained(page);

    // Empty is the default share again: nothing left to save
    await keptCap(page).fill('');
    await expect(saveBar(page)).toBeHidden();
  });

  test('download windows: up to 4, across midnight; a bad one stays here, and Save sends them whole', async ({ page }) => {
    await openSection(page);
    const puts = countPuts(page);
    const add = page.getByRole('button', { name: 'Add a window' });
    await add.click();
    await expect(page.getByLabel('Window 1 starts')).toHaveValue('01:00');
    await expect(page.getByLabel('Window 1 ends')).toHaveValue('07:00');
    await page.getByLabel('Window 1 starts').fill('23:00');
    await page.getByLabel('Window 1 ends').fill('06:00');
    await expect(page.getByText('Ends the next day')).toBeVisible();
    await expect(page.getByText('Downloads run only inside these times, in the server’s time zone. A window can cross midnight.')).toBeVisible();

    await add.click();
    await page.getByLabel('Window 2 starts').fill('09:00');
    await page.getByLabel('Window 2 ends').fill('09:00');
    await add.click();
    await add.click();
    await expect(add).toBeDisabled();
    await expectAccessibleAndContained(page);

    await save(page).click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Download windows” above.');
    await expect(page.getByLabel('Window 2 starts')).toHaveAttribute('aria-invalid', 'true');
    await expect(page.getByLabel('Window 2 starts')).toHaveAccessibleDescription(/^Window 2 starts when it ends\. Pick two different times\./);
    await expect(page.getByLabel('Window 2 starts')).toBeFocused();
    await expect(page.getByLabel('Window 1 starts')).not.toHaveAttribute('aria-invalid');
    expect(puts.count).toBe(0);

    await page.getByRole('button', { name: 'Remove window 4' }).click();
    await page.getByRole('button', { name: 'Remove window 3' }).click();
    await page.getByRole('button', { name: 'Remove window 2' }).click();
    await expect(add).toBeEnabled();
    const put = putStorage(page);
    await save(page).click();
    expect((await put).postDataJSON()).toEqual({ background: { windows: [{ start: '23:00', end: '06:00' }] } });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(page.getByLabel('Window 1 starts')).toHaveValue('23:00');
  });

  test('config.yaml sets the limit (storage-config): other changes leave it, Use auto sends null', async ({ page }) => {
    await scenario(page, 'storage-config');
    await openSection(page);
    const banner = page.locator('.banner').filter({ hasText: 'From config.yaml · 56.4 GB' });
    await expect(banner).toContainText('global_cache_size sets this limit until you choose one here. Auto follows the disk.');
    await expect(limitMode(page, 'Auto')).toHaveAttribute('aria-pressed', 'false');
    await expect(limitMode(page, 'Custom')).toHaveAttribute('aria-pressed', 'false');
    await expect(page.locator('#st-limit-help')).toHaveText('Choose Auto (75% of the disk · 750 GB) or a custom size.');
    await expect(diskBar(page)).toHaveAttribute('aria-label', /Limit 56\.4 GB\.$/);
    await expectAccessibleAndContained(page);

    // Its kept cap (half of 56.4 GB) is below what kept downloads need; a change that doesn't touch it still saves
    await page.getByRole('switch', { name: 'Smart downloads' }).uncheck();
    const first = putStorage(page);
    await save(page).click();
    expect((await first).postDataJSON()).toEqual({ smart: { enabled: false } });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(banner).toBeVisible();

    await banner.getByRole('button', { name: 'Use auto' }).click();
    await expect(banner).toBeHidden();
    await expect(limitMode(page, 'Auto')).toHaveAttribute('aria-pressed', 'true');
    const second = putStorage(page);
    await save(page).click();
    expect((await second).postDataJSON()).toEqual({ limit_bytes: null });
    await expect(page.locator('#st-limit-help')).toHaveText('Auto (75% of the disk) · 750 GB. It follows the disk when it grows.');
    await expect(diskBar(page)).toHaveAttribute('aria-label', /Limit 750 GB\.$/);
    await expect((await sectionNav(page)).getByRole('link', { name: /Storage/ })).toContainText('750 GB · auto');
  });

  test('the nav says config.yaml while it sets the limit', async ({ page }) => {
    await scenario(page, 'storage-config');
    await openSection(page);
    await expect((await sectionNav(page)).getByRole('link', { name: /Storage/ })).toContainText('56.4 GB · config.yaml');
  });

  test('a fresh install (downloads-empty): nothing kept or picked yet reads plainly', async ({ page }) => {
    await scenario(page, 'downloads-empty');
    await openSection(page);
    await expect(diskBar(page)).toHaveAttribute('aria-label', /^Disk of 1\.0 TB: Kept 0 B, Smart 0 B, Cache 62\.4 GB, /);
    await expect(keptCap(page)).toHaveAccessibleDescription('Empty: half the limit. Now 375 GB.');
    await expect(page.getByRole('switch', { name: 'Smart downloads' })).not.toBeChecked();
    await expect(page.getByText('Off: only what you keep downloads in the background.')).toBeVisible();
    await expect(page.getByRole('switch', { name: 'Pause while streaming' })).toBeChecked();
    await expectAccessibleAndContained(page);
  });

  test('Discard puts back what was loaded', async ({ page }) => {
    await openSection(page);
    await limitMode(page, 'Custom').click();
    await page.getByRole('switch', { name: 'Background downloads' }).uncheck();
    await expect(page.getByText('Off: kept and smart downloads wait. Streaming still works.')).toBeVisible();
    await saveBar(page).getByRole('button', { name: 'Discard' }).click();
    await expect(saveBar(page)).toBeHidden();
    await expect(limitMode(page, 'Auto')).toHaveAttribute('aria-pressed', 'true');
    await expect(customLimit(page)).toBeHidden();
    await expect(page.getByRole('switch', { name: 'Background downloads' })).toBeChecked();
  });
});

/** PR screenshots into docs/screenshots/v2/settings, as e2e/screenshots.spec.ts takes them. Set SCREENSHOTS=1. */
test.describe('screenshots', () => {
  test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

  async function shoot(page: Page, testInfo: TestInfo, name: string, { fullPage = false } = {}) {
    const width = testInfo.project.name === 'mobile' ? 390 : 1440;
    await page.evaluate(() => document.fonts.ready);
    if (fullPage) {
      // A viewport as tall as the page, so fixed parts (save bar, tab bar) sit at the bottom as in the design exports
      const height = await page.evaluate(() => document.documentElement.scrollHeight);
      await page.setViewportSize({ width, height });
      await page.waitForTimeout(300);
    }
    await page.screenshot({ path: path.resolve(__dirname, '../../docs/screenshots/v2/settings', `${name}-${width}.png`), animations: 'disabled' });
  }

  test('storage', async ({ page }, testInfo) => {
    await openSection(page);
    await shoot(page, testInfo, 'storage', { fullPage: true });
  });

  test('storage, unsaved changes', async ({ page }, testInfo) => {
    await openSection(page);
    await limitMode(page, 'Custom').click();
    await customLimit(page).fill('300');
    await page.getByRole('button', { name: 'Add a window' }).click();
    await page.getByLabel('Window 1 starts').fill('23:00');
    await page.getByLabel('Window 1 ends').fill('06:00');
    await expect(saveBar(page)).toBeVisible();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'storage-dirty', { fullPage: true });
  });

  test('storage, from config.yaml', async ({ page }, testInfo) => {
    await scenario(page, 'storage-config');
    await openSection(page);
    await shoot(page, testInfo, 'storage-config');
  });

  test('storage, a fresh install', async ({ page }, testInfo) => {
    await scenario(page, 'downloads-empty');
    await openSection(page);
    await shoot(page, testInfo, 'storage-empty');
  });
});
