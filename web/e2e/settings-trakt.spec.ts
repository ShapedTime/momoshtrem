import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Settings › Trakt (#134, flow 3) against the MSW mocks' Trakt state machine
 * (src/mocks/fixtures/trakt.ts): the second read after Connect approves the
 * code, then each read imports one step. The account lives per page, so a
 * reload starts over from the scenario (src/mocks/scenario.ts): trakt-off,
 * trakt-pending (a code that's never approved), trakt-reauth,
 * trakt-unconfigured, trakt-denied, trakt-syncing.
 */

const SECTION = '/settings?section=trakt';
const CODE = 'A1B2-C3D4';
const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';

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
const toast = (page: Page, title: string) => page.getByRole('status').filter({ hasText: title });
const connectButton = (page: Page) => page.getByRole('button', { name: 'Connect Trakt' });
const codeScreen = (page: Page) => page.getByRole('region', { name: 'Connect with a code' });
const waitingScreen = (page: Page) => page.getByRole('region', { name: 'Waiting for Trakt' });
const account = (page: Page) => page.getByRole('heading', { name: 'Trakt account teymur' });

/** The section list's Trakt line: beside the section on desktop; on phone, Back to it (client-side, so the mock keeps its state). */
async function navTrakt(page: Page) {
  const back = page.getByRole('link', { name: 'Settings', exact: true });
  if (await back.isVisible()) await back.click();
  return page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: /^Trakt/ });
}

test.describe('Settings › Trakt', () => {
  test('connect: off → code → waiting → connected, Sync now, a switch, Disconnect → off', async ({ page }) => {
    // The mock polls every 3 s: approval, five import steps and a sync take about half a minute
    test.setTimeout(120_000);
    await scenario(page, 'trakt-off');
    await signIn(page, SECTION);

    await expect(page.getByRole('heading', { name: 'Connect Trakt' })).toBeVisible();
    await expect(page.getByRole('list', { name: 'What momoshtrem reads' })).toContainText('History');
    await expect(page.getByText('You’ll get a code to enter on trakt.tv. No password here.')).toBeVisible();
    await connectButton(page).click();

    // The code, with the link the response named and its countdown
    await expect(codeScreen(page).getByText(CODE)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Connect Trakt' })).toBeHidden();
    await expect(page.getByRole('link', { name: 'Open trakt.tv/activate' })).toHaveAttribute('href', 'https://trakt.tv/activate');
    await expect(codeScreen(page).getByText(/Expires in \d+:\d\d/)).toBeVisible();
    await page.getByRole('button', { name: 'I’ve approved it' }).click();

    await expect(waitingScreen(page)).toContainText('Waiting for you to approve on Trakt…');
    await expect(waitingScreen(page)).toContainText(`Checking every 5 seconds · code ${CODE} expires in`);
    await expect(waitingScreen(page).getByRole('listitem').filter({ hasText: 'Approval on trakt.tv' })).toHaveAttribute('aria-current', 'step');

    // The backend notices the approval; the first import starts
    await expect(toast(page, 'Trakt connected')).toBeVisible({ timeout: 15_000 });
    await expect(account(page)).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: 'First sync in progress' })).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: 'First sync in progress' })).toBeHidden({ timeout: 30_000 });
    await expect(toast(page, 'Synced with Trakt')).toContainText('Imported 269 plays, 7 ratings and 4 watchlist titles.');
    await expect(page.getByText('Last sync just now · syncs every 30 min')).toBeVisible();

    // Sync now: one step in the mock, nothing new
    await page.getByRole('button', { name: 'Sync now' }).click();
    await expect(page.getByRole('button', { name: 'Syncing…' })).toBeDisabled();
    await expect(toast(page, 'Synced with Trakt')).toContainText('Nothing new since the last sync.', { timeout: 15_000 });

    // A switch saves at once
    const paused = page.getByRole('switch', { name: 'Up next includes paused shows' });
    await expect(paused).not.toBeChecked();
    const saved = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/api/integrations/trakt/settings'));
    await paused.check();
    expect((await saved).postDataJSON()).toEqual({ include_paused: true });
    await expect(paused).toBeChecked();
    await expect(paused).toHaveAccessibleDescription('Shows you haven’t touched in 6 months are hidden.');

    // Disconnect is quiet and confirmed, and names whose Trakt it is (the mock household has two people, #239)
    await page.getByRole('button', { name: 'Disconnect' }).click();
    const dialog = page.getByRole('alertdialog', { name: 'Disconnect Teymur’s Trakt?' });
    await expect(dialog).toContainText('Teymur’s imported history, ratings and watchlist will be deleted from momoshtrem. Nothing changes on Trakt.');
    await dialog.getByRole('button', { name: 'Disconnect' }).click();
    await expect(toast(page, 'Trakt disconnected')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Connect Trakt' })).toBeFocused();
    await expect(await navTrakt(page)).toContainText('Not connected');
  });

  test('the code holds while you approve: waiting, Show the code again, Cancel', async ({ page }) => {
    await scenario(page, 'trakt-pending');
    await signIn(page, SECTION);
    await connectButton(page).click();
    await expect(codeScreen(page).getByText(CODE)).toBeVisible();
    await expect(codeScreen(page).getByText('Your code: A 1 B 2, C 3 D 4')).toBeAttached();
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Copy code' }).click();
    await expect(toast(page, 'Copied')).toContainText('Paste it at trakt.tv/activate.');

    await page.getByRole('button', { name: 'I’ve approved it' }).click();
    await expect(waitingScreen(page)).toBeVisible();
    // The button pressed is gone: focus moves to what replaced it
    await expect(waitingScreen(page).locator('[data-screen-focus]')).toBeFocused();
    await expectAccessibleAndContained(page);

    // Waiting is this page's flag over the live code
    await page.getByRole('button', { name: 'Show the code again' }).click();
    await expect(codeScreen(page).getByText(CODE)).toBeVisible();
    await page.getByRole('button', { name: 'I’ve approved it' }).click();
    await waitingScreen(page).getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: 'Connect Trakt' })).toBeFocused();

    // A new code starts on the code, not on waiting
    await connectButton(page).click();
    await expect(codeScreen(page)).toBeVisible();
    await expect(await navTrakt(page)).toContainText('Waiting for approval');
  });

  test('connected: accessible, and the Status row agrees', async ({ page }) => {
    await signIn(page, SECTION);
    await expect(account(page)).toBeVisible();
    await expect(page.getByText('Connected', { exact: true })).toBeVisible();
    await expect(page.getByText('Last sync 4 min ago · syncs every 30 min')).toBeVisible();
    await expect(page.getByText('4 titles · 2 already in your library')).toBeVisible();
    await expect(page.getByRole('switch', { name: 'Ratings shape your picks' })).toBeChecked();
    await expectAccessibleAndContained(page);

    const trakt = await navTrakt(page);
    await expect(trakt).toContainText('Connected as teymur');
    await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: /System status/ }).click();
    const row = page.getByRole('list', { name: 'Services' }).getByRole('listitem').filter({ hasText: 'Trakt' });
    await expect(row).toContainText('Connected as teymur · synced 4 min ago');
    await expect(row).toContainText('OK');
  });

  test('reauth: the data stays, Reconnect shows a code, Cancel goes back', async ({ page }) => {
    await scenario(page, 'trakt-reauth');
    await signIn(page, SECTION);
    const banner = page.getByRole('alert').filter({ hasText: 'Trakt needs you to sign in again' });
    await expect(banner).toContainText('Connect again to keep syncing.');
    const card = page.getByRole('region', { name: 'Trakt account teymur' });
    await expect(card.getByText('Sign in again', { exact: true })).toBeVisible();
    await expect(card.getByText('Last sync 2 days ago')).toBeVisible();
    await expect(card).toContainText('269 plays');
    await expect(page.getByRole('button', { name: 'Sync now' })).toHaveCount(0);
    await expectAccessibleAndContained(page);

    await banner.getByRole('button', { name: 'Reconnect' }).click();
    await expect(codeScreen(page).getByText(CODE)).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'Trakt needs you to sign in again' })).toBeVisible();
    await expect(account(page)).toBeVisible();
  });

  test('unconfigured: names what the server needs instead of Connect', async ({ page }) => {
    await scenario(page, 'trakt-unconfigured');
    await signIn(page, SECTION);
    const note = page.getByText('Trakt isn’t set up on this server').locator('..');
    for (const name of ['TRAKT_CLIENT_ID', 'SECRET_KEY']) await expect(note.getByText(name, { exact: true })).toBeVisible();
    await expect(connectButton(page)).toHaveCount(0);
    await expectAccessibleAndContained(page);
    await expect(await navTrakt(page)).toContainText('Not set up');
  });

  test('denied: a calm banner that mentions the 2-app limit', async ({ page }) => {
    await scenario(page, 'trakt-denied');
    await signIn(page, SECTION);
    await connectButton(page).click();
    await expect(codeScreen(page)).toBeVisible();
    const banner = page.getByRole('status').filter({ hasText: 'Trakt didn’t let momoshtrem in' });
    await expect(banner).toContainText('Free Trakt accounts can connect only 2 apps', { timeout: 15_000 });
    await expect(connectButton(page)).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('account in use: the account is another member’s, so a warning on the off screen (#239)', async ({ page }) => {
    await scenario(page, 'trakt-account-in-use');
    await signIn(page, SECTION);
    await expect(page.getByText('This is Teymur’s Trakt.')).toBeVisible();
    await connectButton(page).click();
    await expect(codeScreen(page)).toBeVisible();
    const banner = page.getByRole('status').filter({ hasText: 'That Trakt account is already connected' });
    await expect(banner).toContainText('Someone else in this household connected it. Each person connects their own', { timeout: 15_000 });
    await expect(connectButton(page)).toBeVisible();
    await expectAccessibleAndContained(page);
  });
});

test.describe('command palette › Trakt', () => {
  async function openPalette(page: Page, testInfo: TestInfo) {
    if (isDesktop(testInfo)) {
      await page.keyboard.press('ControlOrMeta+k');
    } else {
      await page.locator('header.phone-bar').getByRole('button', { name: /Account menu/ }).click();
      await page.getByRole('dialog', { name: 'Account' }).getByRole('button', { name: 'Command palette' }).click();
    }
    await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeVisible();
  }

  test('Sync Trakt now runs in place', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('trakt');
    const option = page.getByRole('option', { name: /^Sync Trakt now/ });
    await expect(option).toContainText('Last synced 4 min ago');
    const started = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith('/api/integrations/trakt/sync'));
    await option.click();
    await started;
    await expect(toast(page, 'Syncing with Trakt')).toContainText('Up next updates in a moment.');
    await expect(page).toHaveURL(/\/library$/);
  });

  test('Connect Trakt opens its settings', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-off');
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('trakt');
    const option = page.getByRole('option', { name: /^Connect Trakt/ });
    await expect(option).toContainText('Personal picks and Up next');
    await option.click();
    await expect(page).toHaveURL(/\/settings\?section=trakt$/);
    await expect(connectButton(page)).toBeVisible();
  });
});

/** PR screenshots into docs/screenshots/v2/settings, as e2e/screenshots.spec.ts takes them. Set SCREENSHOTS=1. */
test.describe('screenshots', () => {
  test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

  async function shoot(page: Page, testInfo: TestInfo, name: string, { fullPage = false } = {}) {
    const width = testInfo.project.name === 'mobile' ? 390 : 1440;
    await page.evaluate(() => document.fonts.ready);
    await page.evaluate(() => window.scrollTo(0, 0));
    if (fullPage) {
      // A viewport as tall as the page, so fixed parts (the tab bar) sit at the bottom as in the design exports
      const height = await page.evaluate(() => document.documentElement.scrollHeight);
      await page.setViewportSize({ width, height });
      await page.waitForTimeout(300);
    }
    await page.screenshot({ path: path.resolve(__dirname, '../../docs/screenshots/v2/settings', `${name}-${width}.png`), animations: 'disabled' });
  }

  test('trakt off', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-off');
    await signIn(page, SECTION);
    await expect(connectButton(page)).toBeVisible();
    await shoot(page, testInfo, 'trakt-off');
  });

  test('trakt code', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-pending');
    await signIn(page, SECTION);
    await connectButton(page).click();
    await expect(codeScreen(page)).toBeVisible();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'trakt-code', { fullPage: true });
  });

  test('trakt waiting', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-pending');
    await signIn(page, SECTION);
    await connectButton(page).click();
    await page.getByRole('button', { name: 'I’ve approved it' }).click();
    await expect(waitingScreen(page)).toBeVisible();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'trakt-waiting');
  });

  test('trakt connected', async ({ page }, testInfo) => {
    await signIn(page, SECTION);
    await expect(account(page)).toBeVisible();
    await shoot(page, testInfo, 'trakt-connected', { fullPage: true });
  });

  test('trakt first sync', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-syncing');
    await signIn(page, SECTION);
    await expect(page.getByRole('status').filter({ hasText: 'First sync in progress' })).toBeVisible();
    await shoot(page, testInfo, 'trakt-syncing', { fullPage: true });
  });
});
