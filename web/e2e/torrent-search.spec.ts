import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * The torrent search sheet (#92) against the MSW mocks, through its deep
 * links: until #95 nothing else opens it. Frieren misses S01E29–E38 (the
 * 29–38 batch is the best match), Severance's The Pirate Bay times out until
 * a Retry, One Battle After Another is a movie with a CAM. Scenarios in
 * src/mocks/scenario.ts (search-partial, -none, -slow, -down).
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
const sheet = (page: Page) => page.getByRole('dialog', { name: /^Find a source · / });

/** Client-side, like an entry point will (#95): a reload would forget the session's drafts. */
const openClientSide = (page: Page, href: string) =>
  page.evaluate((url) => (window as unknown as { next: { router: { push: (href: string) => void } } }).next.router.push(url), href);

const FRIEREN_E29 = '/tv/209867?sheet=torrent&show=209867&se=S01E29';
const SEVERANCE_E08 = '/tv/95396?sheet=torrent&show=95396&se=S02E08';
const BATTLE = '/movie/1054867?sheet=torrent&movie=1054867';
const BATCH = '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]';
const BATTLE_BEST = 'One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX';

test.describe('Torrent search', () => {
  test('an episode: the best match and what else was searched; Add goes to the mapping review', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    await expect(dialog).toHaveAccessibleName(/^Find a source · Frieren: Beyond Journey’s End S01E29 · /);
    await expect(dialog.getByRole('tab', { name: 'Torrents' })).toHaveAttribute('aria-selected', 'true');
    await expect(dialog.getByRole('searchbox', { name: 'Search releases' })).toHaveValue('Frieren 29');
    await expect(dialog.getByRole('button', { name: 'Episode', exact: true })).toHaveAttribute('aria-pressed', 'true');

    const best = dialog.getByRole('article', { name: BATCH });
    await expect(best).toContainText('Best match · score 96');
    await expect(best).toContainText('Covers all 10 missing episodes');
    await expect(dialog.getByText('27 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();
    await expect(dialog.getByText('Also searched: Sousou no Frieren 29 · Frieren S02E01 (scene numbering)')).toBeVisible();
    await expect(dialog.getByRole('heading', { name: 'Other results · sorted by best' })).toBeVisible();
    // Long names wrap instead of being cut
    await expect(dialog.getByRole('article', { name: /S01E29\.Lets\.Go\.Somewhere\.We\.Cant\.Reach/ })).toBeVisible();
    await expectAccessibleAndContained(page);

    await best.getByRole('button', { name: 'Add & review mapping' }).click();
    await expect(page).toHaveURL(/\/mapping\/\d+$/);
    await expect(page.getByText(BATCH).first()).toBeVisible();
  });

  test('a movie: Copy magnet, then Add assigns on the spot', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await signIn(page, BATTLE);
    const dialog = sheet(page);
    await expect(dialog).toHaveAccessibleName('Find a source · One Battle After Another 2025 · 2h 42m');
    await expect(dialog.getByRole('searchbox', { name: 'Search releases' })).toHaveValue('One Battle After Another 2025');
    await expect(dialog.getByRole('group', { name: 'Search scope' })).toHaveCount(0);
    await expect(dialog.getByRole('article', { name: /CAMRip/ })).toContainText('CAM recording · filmed in a cinema');
    const best = dialog.getByRole('article', { name: BATTLE_BEST });
    await expectAccessibleAndContained(page);

    await best.getByRole('button', { name: 'Copy magnet link' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Magnet link copied' })).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(/^magnet:\?xt=urn:btih:[0-9a-f]{40}&dn=One\.Battle/);

    await best.getByRole('button', { name: 'Add & assign to movie' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Torrent assigned to movie' })).toBeVisible({ timeout: 20_000 });
    await expect(best.getByText('Added')).toBeVisible();
  });

  test('Load more appends the next page where you are', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    await expect(dialog.getByText('Showing 25 of 27')).toBeVisible();
    const more = dialog.getByRole('button', { name: 'Load more' });
    await more.scrollIntoViewIfNeeded();
    const body = dialog.locator('.sheet-body');
    const before = await body.evaluate((el) => el.scrollTop);
    await more.click();
    await expect(dialog.getByText('Showing 27 of 27')).toBeVisible();
    await expect(more).toHaveCount(0);
    await expect(dialog.getByRole('article')).toHaveCount(27);
    await expect(dialog.getByRole('article').nth(25)).toBeFocused();
    expect(Math.abs((await body.evaluate((el) => el.scrollTop)) - before)).toBeLessThan(4);
  });

  test('the scope switch, and the pack banner’s Season packs', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    const banner = dialog.getByText('10 episodes are missing');
    await expect(banner).toBeVisible();
    await expect(dialog.getByText('A pack gets 29–38 in one go. The best match below already does.')).toBeVisible();
    await dialog.getByRole('button', { name: 'Season packs' }).click();
    await expect(dialog.getByRole('button', { name: 'Season pack', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(dialog.getByText('13 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();
    await expect(banner).toBeHidden();
    await expect(dialog.getByRole('article', { name: BATCH })).toContainText('Best match');

    await dialog.getByRole('button', { name: 'Any', exact: true }).click();
    await expect(dialog.getByText('45 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('a timed-out indexer: a warning with Retry, which asks again', async ({ page }) => {
    await signIn(page, SEVERANCE_E08);
    const dialog = sheet(page);
    const warning = dialog.getByRole('status').filter({ hasText: '2 of 3 indexers answered' });
    await expect(warning).toContainText('The Pirate Bay timed out after 8 s. Results may be incomplete.');
    await expect(dialog.getByText('9 results · TorrentLeech, EZTV')).toBeVisible();
    await expect(dialog.getByRole('article', { name: /SuccessfulCrab/ }).first()).toBeVisible();
    await expectAccessibleAndContained(page);

    await warning.getByRole('button', { name: 'Retry' }).click();
    await expect(warning).toBeHidden();
    await expect(dialog.getByText('13 results · TorrentLeech, EZTV, The Pirate Bay')).toBeVisible();
    await expect(dialog.getByRole('article', { name: /x265-ELiTE/ })).toBeVisible();
  });

  test('nothing found: ways to broaden, and Try HDRezka', async ({ page }) => {
    await scenario(page, 'search-none');
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    await expect(dialog.getByRole('heading', { name: 'No releases for “Frieren 29”' })).toBeVisible();
    await expect(dialog.getByText('3 indexers searched. Release names are often shorter than titles.')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Try “Frieren”' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Use the Japanese title' })).toBeVisible();
    await expectAccessibleAndContained(page);

    await dialog.getByRole('button', { name: 'Search any pack, not just episode 29' }).click();
    await expect(dialog.getByRole('button', { name: 'Any', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await dialog.getByRole('button', { name: 'Try HDRezka' }).click();
    await expect(page).toHaveURL(/[?&]sheet=hdrezka&show=209867/);
    await expect(sheet(page).getByRole('tab', { name: 'HDRezka' })).toHaveAttribute('aria-selected', 'true');
  });

  test('a slow search: each indexer’s progress, then the results', async ({ page }) => {
    await scenario(page, 'search-slow');
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    await expect(dialog.getByText('Searching 3 indexers…')).toBeVisible();
    await expect(dialog.getByText('Frieren 29 · also Sousou no Frieren 29')).toBeVisible();
    await expect(dialog.getByRole('list', { name: 'Indexers' }).getByRole('listitem')).toHaveCount(3);
    await expect(dialog.getByRole('list', { name: 'Indexers' })).toContainText('waiting');
    await expect(dialog.getByRole('article', { name: BATCH })).toBeVisible({ timeout: 15_000 });
    await expect(dialog.getByText('27 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible({ timeout: 15_000 });
  });

  test('Jackett off: the error says so', async ({ page }) => {
    await scenario(page, 'search-down');
    await signIn(page, FRIEREN_E29);
    const alert = sheet(page).getByRole('alert');
    await expect(alert).toContainText('Torrent search is off');
    await expect(alert).toContainText('JACKETT_URL');
    await expectNoA11yViolations(page);
  });

  test('closing keeps the search: reopening picks up where you left off', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const dialog = sheet(page);
    await expect(dialog.getByRole('article', { name: BATCH })).toBeVisible();
    await dialog.getByRole('button', { name: 'Season pack', exact: true }).click();
    const field = dialog.getByRole('searchbox', { name: 'Search releases' });
    await field.fill('Frieren batch');
    await field.press('Enter');
    await expect(dialog.getByText('4 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(page).toHaveURL(/\/tv\/209867$/);

    await openClientSide(page, FRIEREN_E29);
    await expect(dialog.getByText('Picked up where you left off')).toBeVisible();
    await expect(field).toHaveValue('Frieren batch');
    await expect(dialog.getByRole('button', { name: 'Season pack', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await expect(dialog.getByText('4 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();
  });

  test('the Torrents tab still opens the legacy search until #95', async ({ page }) => {
    await signIn(page, '/tv/209867?sheet=hdrezka&show=209867&se=S01E29');
    await sheet(page).getByRole('tab', { name: 'Torrents' }).click();
    await expect(sheet(page)).toBeHidden();
    await expect(page.getByRole('dialog', { name: 'Search: Frieren: Beyond Journey’s End S01E29' })).toBeVisible();
  });
});

/**
 * PR screenshots of the sheet's states at 390 and 1440, written to
 * docs/screenshots/v2/torrent/. Skipped in normal runs:
 *   SCREENSHOTS=1 npx playwright test torrent-search && node scripts/optimize-screenshots.mjs ../docs/screenshots/v2/torrent
 */
test.describe('Torrent search screenshots', () => {
  test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

  async function shoot(page: Page, testInfo: TestInfo, name: string) {
    const width = testInfo.project.name === 'mobile' ? 390 : 1440;
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: path.resolve(__dirname, '../../docs/screenshots/v2/torrent', `${name}-${width}.png`), animations: 'disabled' });
  }

  test('results', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await expect(sheet(page).getByRole('article', { name: BATCH })).toBeVisible();
    await shoot(page, testInfo, 'results');
  });

  test('season', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('button', { name: 'Season packs' }).click();
    await expect(sheet(page).getByText('13 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeVisible();
    await shoot(page, testInfo, 'season');
  });

  test('searching', async ({ page }, testInfo) => {
    await scenario(page, 'search-slow');
    await signIn(page, FRIEREN_E29);
    await expect(sheet(page).getByText('Searching 3 indexers…')).toBeVisible();
    await shoot(page, testInfo, 'searching');
  });

  test('partial', async ({ page }, testInfo) => {
    await signIn(page, SEVERANCE_E08);
    await expect(sheet(page).getByText('2 of 3 indexers answered')).toBeVisible();
    await shoot(page, testInfo, 'partial');
  });

  test('none', async ({ page }, testInfo) => {
    await scenario(page, 'search-none');
    await signIn(page, FRIEREN_E29);
    await expect(sheet(page).getByRole('heading', { name: 'No releases for “Frieren 29”' })).toBeVisible();
    await shoot(page, testInfo, 'none');
  });

  test('movie', async ({ page }, testInfo) => {
    await signIn(page, BATTLE);
    await expect(sheet(page).getByRole('article', { name: BATTLE_BEST })).toBeVisible();
    await shoot(page, testInfo, 'movie');
  });
});
