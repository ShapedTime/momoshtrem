import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Settings › Quality profile and Jackett's indexers in Settings › Status (#94),
 * against the MSW mocks: the fixture is the design's profile (2160p, ≤ 8 GB/h).
 * Scenarios in src/mocks/scenario.ts (quality-invalid, indexers-degraded).
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
const preview = (page: Page) => page.getByRole('status', { name: 'Best match would prefer' });
const saveBar = (page: Page) => page.getByRole('region', { name: 'Unsaved changes' });
const segment = (page: Page, group: string, option: string) => page.getByRole('group', { name: group }).getByRole('button', { name: option, exact: true });

/** The section list: beside the section on desktop; on phone, Back to it (client-side, so the mocks keep what was saved). */
async function sectionNav(page: Page) {
  const back = page.getByRole('link', { name: 'Settings', exact: true });
  if (await back.isVisible()) await back.click();
  return page.getByRole('navigation', { name: 'Settings sections' });
}

/** The range moves by keyboard: one step is 0.5 GB/h. */
async function nudgeSize(page: Page, steps: number) {
  const range = page.getByRole('slider', { name: 'Max size per hour' });
  await range.focus();
  for (let i = 0; i < Math.abs(steps); i++) await range.press(steps > 0 ? 'ArrowRight' : 'ArrowLeft');
}

test.describe('Settings › Quality profile', () => {
  test('change resolution and size, Save, and the nav summary follows', async ({ page }) => {
    await signIn(page, '/settings?section=quality');
    await expect(page.getByRole('heading', { name: 'Quality profile' })).toBeVisible();
    await expect(page.getByText('What “Best match” means when you search for a torrent.')).toBeVisible();
    await expect(preview(page)).toContainText('2160p · DV · ≤ 8 GB/h');
    await expect(preview(page)).toContainText('x265 when there’s a choice · from FLUX or NTb +2 · never CAM, TS, HDCAM +3');
    await expect(segment(page, 'Maximum resolution', '1080p')).toBeDisabled();
    await expect(saveBar(page)).toBeHidden();
    await expectAccessibleAndContained(page);

    await segment(page, 'Preferred resolution', '1080p').click();
    await nudgeSize(page, 3);
    await expect(page.getByRole('slider', { name: 'Max size per hour' })).toHaveAttribute('aria-valuetext', '9.5 GB/h');
    await expect(page.getByText('9.5 GB/h ≈ 26 GB for a 2 h 46 m film')).toBeVisible();
    await expect(preview(page)).toContainText('1080p · DV · ≤ 9.5 GB/h');
    await expect(saveBar(page)).toBeVisible();
    await expectAccessibleAndContained(page);

    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(saveBar(page)).toBeHidden();

    // The section list reads the saved profile
    await expect((await sectionNav(page)).getByRole('link', { name: /Quality profile/ })).toContainText('1080p preferred · ≤ 9.5 GB/h');
  });

  test('Discard puts back what was loaded', async ({ page }) => {
    await signIn(page, '/settings?section=quality');
    await page.getByRole('switch', { name: 'Prefer Dolby Vision' }).uncheck();
    await expect(preview(page)).toContainText('2160p · HDR · ≤ 8 GB/h');
    await nudgeSize(page, -2);
    await expect(saveBar(page)).toBeVisible();

    await saveBar(page).getByRole('button', { name: 'Discard' }).click();
    await expect(saveBar(page)).toBeHidden();
    await expect(page.getByRole('switch', { name: 'Prefer Dolby Vision' })).toBeChecked();
    await expect(page.getByRole('slider', { name: 'Max size per hour' })).toHaveAttribute('aria-valuetext', '8 GB/h');
    await expect(preview(page)).toContainText('2160p · DV · ≤ 8 GB/h');
  });

  test('add and remove a term to avoid', async ({ page }) => {
    await signIn(page, '/settings?section=quality');
    const terms = page.getByRole('list', { name: 'Terms to avoid' });
    const input = page.getByRole('textbox', { name: 'Add to Terms to avoid' });

    await input.fill('HDTS');
    await input.press('Enter');
    await expect(terms.getByText('HDTS', { exact: true })).toBeVisible();
    await expect(input).toHaveValue('');
    await expect(input).toBeFocused();

    await input.fill('cam');
    await input.press('Enter');
    await expect(page.getByRole('alert').filter({ hasText: 'CAM is already on the list.' })).toBeVisible();

    await terms.getByRole('button', { name: 'Remove TELESYNC' }).click();
    await expect(terms.getByText('TELESYNC', { exact: true })).toBeHidden();
    await expect(terms.getByRole('button', { name: 'Remove HC' })).toBeFocused();
    await expect(preview(page)).toContainText('never CAM, TS, HDCAM +3');
    await expect(saveBar(page)).toBeVisible();
    await expectAccessibleAndContained(page);

    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(saveBar(page)).toBeHidden();
    await expect(terms).toContainText('HDTS');
    await expect(terms).not.toContainText('TELESYNC');
  });

  test('a server 422 shows on the field it names', async ({ page }) => {
    await scenario(page, 'quality-invalid');
    await signIn(page, '/settings?section=quality');
    await segment(page, 'Preferred resolution', '1080p').click();
    await saveBar(page).getByRole('button', { name: 'Save' }).click();

    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Terms to avoid” above.');
    const input = page.getByRole('textbox', { name: 'Add to Terms to avoid' });
    await expect(input).toHaveAttribute('aria-invalid', 'true');
    await expect(input).toHaveAccessibleDescription('Each term needs 1 to 30 characters.');
    await expect(input).toBeInViewport();
    await expect(saveBar(page)).toBeVisible();
    await expectAccessibleAndContained(page);
  });
});

test.describe('Settings › Status › Jackett indexers', () => {
  test('lists each indexer under Jackett', async ({ page }) => {
    await signIn(page, '/settings?section=status');
    const jackett = page.getByRole('list', { name: 'Services' }).getByRole('listitem').filter({ hasText: 'Jackett' }).first();
    await expect(jackett).toContainText('2 of 3 indexers OK');
    const indexers = page.getByRole('list', { name: 'Jackett indexers' });
    await expect(indexers.getByRole('listitem')).toHaveCount(3);
    await expect(indexers.getByRole('listitem').filter({ hasText: 'Nyaa' })).toContainText('210 ms');
    await expect(indexers.getByRole('listitem').filter({ hasText: 'The Pirate Bay' })).toContainText('Not searched yet');
    await expect(page.getByText('Open Jackett')).toHaveCount(0);
    await expectAccessibleAndContained(page);
  });

  test('an indexer timing out and one failing make it Degraded', async ({ page }) => {
    await scenario(page, 'indexers-degraded');
    await signIn(page, '/settings?section=status');
    await expect(page.getByRole('status').filter({ hasText: 'Mostly fine' })).toContainText(
      '2 indexers are not answering. Search still works with the others.'
    );
    const jackett = page.getByRole('list', { name: 'Services' }).getByRole('listitem').filter({ hasText: 'Jackett' }).first();
    await expect(jackett).toContainText('2 of 4 indexers OK');
    await expect(jackett).toContainText('Degraded');
    const indexers = page.getByRole('list', { name: 'Jackett indexers' });
    await expect(indexers.getByRole('listitem').filter({ hasText: 'The Pirate Bay' })).toContainText('Timing out for 2 h');
    await expect(indexers.getByRole('listitem').filter({ hasText: 'RuTracker' })).toContainText('Failing for 5 min: jackett error: Login failed');
    await expectAccessibleAndContained(page);
    await expect((await sectionNav(page)).getByRole('link', { name: /System status/ })).toContainText('1 warning');
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

  test('quality', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=quality');
    await expect(preview(page)).toBeVisible();
    await shoot(page, testInfo, 'quality', { fullPage: true });
  });

  test('quality, unsaved changes', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=quality');
    await segment(page, 'Preferred resolution', '1080p').click();
    await nudgeSize(page, 3);
    await expect(saveBar(page)).toBeVisible();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'quality-dirty');
  });

  test('quality, server 422', async ({ page }, testInfo) => {
    await scenario(page, 'quality-invalid');
    await signIn(page, '/settings?section=quality');
    await segment(page, 'Preferred resolution', '1080p').click();
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(saveBar(page).getByRole('alert')).toBeVisible();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'quality-invalid');
  });

  test('status, indexers degraded', async ({ page }, testInfo) => {
    await scenario(page, 'indexers-degraded');
    await signIn(page, '/settings?section=status');
    await expect(page.getByRole('list', { name: 'Jackett indexers' })).toBeVisible();
    await shoot(page, testInfo, 'status-degraded', { fullPage: true });
  });
});
