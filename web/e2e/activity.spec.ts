import { expect, test, type Page, type Request, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Activity and a torrent's detail (PLAN F7, #102) against the MSW mocks:
 * Severance streaming on the Apple TV, Frieren and Interstellar (few seeders)
 * downloading, One Piece paused, and four idle torrents (Bluey complete, an
 * Inspect dry run nothing links, the Frieren batch an open job holds, Dune
 * not loaded).
 */

const SEVERANCE = 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1';
const INTERSTELLAR = 'c9d4a7b3e5f60718293a4b5c6d7e8f90a1b2c3d4';
const BLUEY = 'e8d7c6b5a4938271605f4e3d2c1b0a9f8e7d6c5b';

const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';
const section = (page: Page, name: string) => page.getByRole('region', { name: new RegExp(`^${name} \\d+$`) });
const card = (page: Page, label: string) => page.getByRole('article', { name: label, exact: true });
const toast = (page: Page, title: string) => page.getByRole('status').filter({ hasText: title });

/** Waits for running animations (not the infinite ones) so axe reads final colours. */
async function settle(page: Page) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
}

/** axe, and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page, options: { exclude?: string[] } = {}) {
  await settle(page);
  await expectNoA11yViolations(page, options);
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

/** The torrent DELETEs this page sends. */
function deletes(page: Page): Request[] {
  const sent: Request[] = [];
  page.on('request', (request) => {
    if (request.method() === 'DELETE' && /\/api\/torrents\/[0-9a-f]{40}/.test(request.url())) sent.push(request);
  });
  return sent;
}

test.describe('activity', () => {
  test('says how it’s going, sums it up and lists the torrents in four sections', async ({ page }) => {
    await signIn(page, '/activity');
    await expect(page.getByRole('heading', { name: 'Activity', level: 1 })).toBeVisible();
    await expect(page).toHaveTitle('Activity · momoshtrem');
    await expect(page.getByText('Infuse is streaming smoothly; Interstellar is slow because it has few peers.')).toBeVisible();

    const now = page.getByRole('link', { name: /Severance S02E05 on Apple TV/ });
    await expect(now).toHaveAttribute('href', `/activity/${SEVERANCE}`);
    await expect(now).toContainText('Trojan’s Horse · 18:42 of 52:00 · buffered 33 min ahead');
    const summary = page.getByRole('region', { name: 'Summary' });
    await expect(summary).toContainText('3active');
    await expect(summary).toContainText('1 paused · 4 idle');
    await expect(summary).toContainText('812GB free');
    await expect(summary.getByRole('img', { name: /Total download speed over the last minute/ })).toBeVisible();

    await expect(section(page, 'Streaming now')).toContainText('Streaming now 1');
    await expect(section(page, 'Downloading').getByRole('article')).toHaveCount(2);
    await expect(section(page, 'Paused').getByRole('article')).toHaveCount(1);
    await expect(section(page, 'Idle').getByRole('article')).toHaveCount(4);
    await expect(section(page, 'Idle')).toContainText('Ready in Infuse · fetches when you play');

    const streaming = card(page, 'Severance · Season 2');
    await expect(streaming).toContainText('S02E05 Trojan’s Horse');
    await expect(streaming).toContainText('Apple TV · Infuse · 18:42 of 52:00');
    await expect(streaming.getByRole('link', { name: 'Severance · Season 2', exact: true })).toHaveAttribute('href', '/tv/95396');
    await expect(streaming.getByRole('link', { name: 'Details: Severance · Season 2' })).toHaveAttribute('href', `/activity/${SEVERANCE}`);
    await expect(card(page, 'Interstellar')).toContainText('Few peers');
    await expect(card(page, 'Interstellar')).toContainText('42%');
    await expect(card(page, 'Bluey · Season 3')).toContainText('Complete');
    // Idle torrents can't be paused; nothing hides behind hover
    await expect(card(page, 'Dune: Part Two').getByRole('button', { name: /Pause/ })).toHaveCount(0);
    await expect(card(page, 'Dune: Part Two').getByRole('button', { name: 'Remove Dune: Part Two torrent' })).toBeVisible();
    // The open mapping jobs, as on the old page
    await expect(page.getByRole('region', { name: 'Episode mappings to check' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('pause moves a card to Paused with a toast; resume brings it back; Pause all', async ({ page }) => {
    await signIn(page, '/activity');
    await card(page, 'Interstellar').getByRole('button', { name: 'Pause Interstellar' }).click();
    await expect(toast(page, 'Paused Interstellar')).toBeVisible();
    await expect(section(page, 'Paused').getByRole('article', { name: 'Interstellar' })).toBeVisible();
    await expect(section(page, 'Downloading').getByRole('article')).toHaveCount(1);
    await expect(page.locator('.act-verdict')).toHaveText('Everything’s working. Infuse is streaming smoothly.');

    await card(page, 'Interstellar').getByRole('button', { name: 'Resume Interstellar' }).click();
    await expect(toast(page, 'Resumed Interstellar')).toBeVisible();
    await expect(section(page, 'Downloading').getByRole('article', { name: 'Interstellar' })).toBeVisible();

    // Pausing a stream warns that Infuse stalls
    await card(page, 'Severance · Season 2').getByRole('button', { name: 'Pause Severance · Season 2' }).click();
    await expect(toast(page, 'Paused Severance · Season 2')).toContainText('Infuse will stall once its buffer runs out.');
    await expect(page.getByRole('link', { name: /Severance S02E05/ })).toHaveCount(0);

    await page.getByRole('button', { name: 'Pause all' }).click();
    await expect(toast(page, 'Paused 2 torrents')).toContainText('Infuse keeps playing anything already downloaded.');
    await expect(section(page, 'Paused').getByRole('article')).toHaveCount(4);
    await expect(page.getByRole('button', { name: 'Pause all' })).toHaveCount(0);
  });

  test('remove, then Undo: the torrent comes back and nothing is sent', async ({ page }) => {
    const sent = deletes(page);
    await signIn(page, '/activity');
    await card(page, 'Interstellar').getByRole('button', { name: 'Remove Interstellar torrent' }).click();
    const dialog = page.getByRole('alertdialog', { name: 'Remove this torrent?' });
    await expect(dialog).toContainText('Infuse will no longer be able to play Interstellar. The movie stays in your library and shows “Needs a torrent”.');
    await expect(dialog.getByRole('checkbox', { name: 'Also delete downloaded data (up to 63.1 GB)' })).toBeChecked();
    await expectNoA11yViolations(page);
    await dialog.getByRole('button', { name: 'Remove torrent' }).click();

    const undo = toast(page, 'Removed Interstellar');
    await expect(undo).toContainText('Undo within 10 seconds');
    await expect(card(page, 'Interstellar')).toHaveCount(0);
    await undo.getByRole('button', { name: 'Undo' }).click();
    await expect(card(page, 'Interstellar')).toHaveCount(1);
    await page.waitForTimeout(11_000);
    expect(sent).toHaveLength(0);
  });

  test('remove, then wait: the DELETE goes out with delete_data when the toast closes', async ({ page }) => {
    const sent = deletes(page);
    await signIn(page, '/activity');
    await card(page, 'Interstellar').getByRole('button', { name: 'Remove Interstellar torrent' }).click();
    await page.getByRole('alertdialog', { name: 'Remove this torrent?' }).getByRole('button', { name: 'Remove torrent' }).click();
    await expect(card(page, 'Interstellar')).toHaveCount(0);
    const request = await page.waitForRequest((r) => r.method() === 'DELETE' && r.url().includes(`/api/torrents/${INTERSTELLAR}`), { timeout: 15_000 });
    expect(new URL(request.url()).searchParams.get('delete_data')).toBe('true');
    await expect(toast(page, 'Removed Interstellar')).toBeHidden();
    await expect(card(page, 'Interstellar')).toHaveCount(0);

    // Unticked, and dismissing the toast sends it at once
    await card(page, 'Bluey · Season 3').getByRole('button', { name: 'Remove Bluey · Season 3 torrent' }).click();
    const dialog = page.getByRole('alertdialog', { name: 'Remove this torrent?' });
    await expect(dialog).toContainText('The show stays in your library and those episodes show “Needs a torrent”.');
    await dialog.getByRole('checkbox', { name: 'Also delete downloaded data (21.7 GB)' }).uncheck();
    await dialog.getByRole('button', { name: 'Remove torrent' }).click();
    await toast(page, 'Removed Bluey · Season 3').getByRole('button', { name: 'Dismiss' }).click();
    await expect
      .poll(() => sent.map((r) => new URL(r.url()).pathname + new URL(r.url()).search))
      .toEqual([`/api/torrents/${INTERSTELLAR}?delete_data=true`, `/api/torrents/${BLUEY}?delete_data=false`]);
    await expect(card(page, 'Bluey · Season 3')).toHaveCount(0);
  });

  test('nothing but idle torrents: all quiet, a sleeping cat and the idle section', async ({ page }) => {
    await signIn(page, '/activity');
    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'torrents-quiet'));
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Nothing downloading.' })).toBeVisible();
    await expect(page.getByText('All quiet. Nothing in the queue, and the server is fine.')).toBeVisible();
    await expect(page.getByText('Nothing playing')).toBeVisible();
    await expect(page.getByRole('link', { name: 'See what’s ready' })).toHaveAttribute('href', '/library?filter=ready');
    await expect(section(page, 'Idle').getByRole('article')).toHaveCount(4);
    await expect(page.getByRole('button', { name: 'Pause all' })).toHaveCount(0);
    await expectAccessibleAndContained(page);
    await page.getByRole('button', { name: 'How to play' }).click();
    await expect(page.getByRole('dialog', { name: 'Play it in Infuse' })).toBeVisible();
  });

  test('offline: keeps what it last heard, greyed and inert, and retries on a countdown', async ({ page }) => {
    await signIn(page, '/activity');
    await expect(section(page, 'Downloading')).toBeVisible();
    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'torrents-offline'));
    // The next poll fails (after its retries)
    const banner = page.getByRole('alert').filter({ hasText: 'Can’t reach momoshtrem' });
    await expect(banner).toBeVisible({ timeout: 20_000 });
    await expect(banner).toContainText(/Showing what it said at \d\d:\d\d\. Downloads may still be running on the server; Infuse can still play ready titles if WebDAV is up\./);
    await expect(banner).toContainText(/Retrying in \d+ s/);
    await expect(page.getByText(/^Last known · \d\d:\d\d$/)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Pause all' })).toHaveCount(0);
    const stale = page.getByRole('group', { name: 'Last known torrents (offline)' });
    await expect(stale).toHaveAttribute('inert', '');
    await expect(page.getByRole('region', { name: 'Summary' })).toHaveClass(/is-stale/);
    // The faded last-known content is inert (inactive UI: no contrast requirement)
    await expectAccessibleAndContained(page, { exclude: ['.is-stale'] });

    await banner.getByRole('button', { name: 'Retry now' }).click();
    await expect(banner).toContainText('Still not answering. Retrying in', { timeout: 15_000 });

    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await banner.getByRole('button', { name: 'Retry now' }).click();
    await expect(banner).toBeHidden();
    await expect(page.getByText(/Everything’s working\./)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Pause all' })).toBeVisible();
  });

  test('the old /torrents pages redirect to Activity', async ({ page }) => {
    await signIn(page, '/activity');
    await page.goto('/torrents');
    await expect(page).toHaveURL(/\/activity$/);
    await page.goto(`/torrents/${SEVERANCE}/debug`);
    await expect(page).toHaveURL(new RegExp(`/activity/${SEVERANCE}$`));
    await expect(page.getByRole('heading', { name: 'Severance · Season 2', level: 1 })).toBeVisible();
  });
});

test.describe('torrent detail', () => {
  test('header, health, the file Infuse reads and Advanced from ?advanced=1', async ({ page }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await signIn(page, `/activity/${SEVERANCE}`);
    await expect(page.getByRole('heading', { name: 'Severance · Season 2', level: 1 })).toBeVisible();
    await expect(page).toHaveTitle('Severance · Season 2 · momoshtrem');
    await expect(page.getByRole('link', { name: 'Activity', exact: true }).last()).toHaveAttribute('href', '/activity');
    await expect(page.getByText('Streaming now', { exact: true }).first()).toBeVisible();
    await expect(page.getByRole('region', { name: 'Healthy' })).toContainText('Everything is on disk, so playback never waits for the network.');

    const files = page.getByRole('region', { name: /^Files/ });
    await expect(files).toContainText('10 mapped · in Severance.S02.2160p.ATVP.WEB-DL.DV.H.265/');
    await expect(files.getByRole('listitem')).toHaveCount(10);
    const reading = files.getByRole('listitem').filter({ hasText: 'Infuse is reading' });
    await expect(reading).toContainText('S02E05');
    await expect(reading).toContainText('Trojan’s Horse');

    // Collapsed by default
    const advanced = page.locator('details.td-adv');
    await expect(advanced).not.toHaveAttribute('open', '');
    await expectAccessibleAndContained(page);

    await page.goto(`/activity/${SEVERANCE}?advanced=1`);
    await expect(advanced).toHaveAttribute('open', '');
    await expect(page.getByRole('img', { name: 'Piece map: 2,879 of 2,879 pieces downloaded' })).toBeVisible();
    await expect(advanced).toContainText('1 square = 1 piece (16 MB) · 2,879 of 2,879');
    await expect(advanced).toContainText('Infuse · S02E05 · at 18:42 of 52:00 · 9.1 MB/s');
    await expect(advanced.getByText('Status unknown')).toHaveCount(2);
    await advanced.getByRole('button', { name: 'Copy info hash' }).click();
    await expect(toast(page, 'Info hash copied')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('a big season lists 10 files, then all of them', async ({ page }) => {
    await signIn(page, `/activity/${BLUEY}`);
    const files = page.getByRole('region', { name: /^Files/ });
    await expect(files.getByRole('listitem')).toHaveCount(10);
    await files.getByRole('button', { name: 'Show all 49 files' }).click();
    await expect(files.getByRole('listitem')).toHaveCount(49);
  });

  test('Re-run mapping opens a mapping job for the torrent', async ({ page }) => {
    await signIn(page, `/activity/${SEVERANCE}`);
    await page.getByRole('button', { name: 'Re-run mapping' }).click();
    await expect(page).toHaveURL(/\/mapping\/\d+$/);
  });

  test('pause and resume here; remove goes back to Activity', async ({ page }, testInfo) => {
    await signIn(page, `/activity/${INTERSTELLAR}`);
    await expect(page.getByText('Slow but moving')).toBeVisible();
    await expect(page.getByText('Few peers').first()).toBeVisible();
    await page.getByRole('button', { name: 'Pause', exact: true }).click();
    await expect(toast(page, 'Paused Interstellar')).toBeVisible();
    await expect(page.getByRole('region', { name: 'Paused' })).toContainText('You paused it');
    await page.getByRole('button', { name: 'Resume', exact: true }).click();
    await expect(toast(page, 'Resumed Interstellar')).toBeVisible();
    if (isDesktop(testInfo)) await expect(page.getByRole('button', { name: 'Copy magnet' })).toBeVisible();

    await page.getByRole('button', { name: 'Remove', exact: true }).click();
    await page.getByRole('alertdialog', { name: 'Remove this torrent?' }).getByRole('button', { name: 'Remove torrent' }).click();
    await expect(page).toHaveURL(/\/activity$/);
    await expect(toast(page, 'Removed Interstellar')).toBeVisible();
    await expect(card(page, 'Interstellar')).toHaveCount(0);
  });

  test('an unknown torrent is gone, and says the library entry stays', async ({ page }) => {
    await signIn(page, `/activity/${'0'.repeat(40)}`);
    await expect(page.getByRole('heading', { name: 'That torrent is gone', level: 1 })).toBeVisible();
    await expect(page.getByText('It may have been removed. Your library entry stays put and shows “Needs a torrent”.')).toBeVisible();
    await page.getByRole('link', { name: 'Back to Activity' }).click();
    await expect(page).toHaveURL(/\/activity$/);
  });
});
