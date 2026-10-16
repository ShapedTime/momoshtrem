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
    // The plan follows the scope, and so does the field until you type
    await expect(dialog.getByRole('searchbox', { name: 'Search releases' })).toHaveValue('Frieren S01');
    await expect(dialog.getByText('Also searched: Sousou no Frieren S01 · Frieren S02 (scene numbering)')).toBeVisible();
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
 * Filters, sort and Inspect (#93): the chip row, the nested Filters, Sort and
 * Inspect sheets, and filtered-empty. Frieren's S01E29 has 27 results: 3 in
 * 2160p, none of them with 100 seeders; its 29–38 batch inspects to 16 files.
 */
test.describe('Torrent search: filters, sort and Inspect', () => {
  const chipRow = (page: Page) => sheet(page).getByRole('group', { name: 'Filter and sort' });
  const filtersSheet = (page: Page) => page.getByRole('dialog', { name: 'Filters' });
  const inspectSheet = (page: Page) => page.getByRole('dialog', { name: /^Inspect · nothing downloads yet/ });
  const status = (page: Page, count: number) => sheet(page).getByText(`${count} results · Nyaa, TorrentLeech, The Pirate Bay`);

  test('a quick chip filters at once, and the counts follow', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    const chips = chipRow(page);
    const uhd = chips.getByRole('button', { name: /^2160p/ });
    await expect(uhd).toHaveAccessibleName('2160p 3');
    await expect(chips.getByRole('button', { name: 'Sort: Best' })).toBeVisible();
    const wide = chips.getByRole('button', { name: /^x265/ });
    // Codec and Dolby Vision join from 640 px; DV finds nothing here, so it's disabled
    if (testInfo.project.name === 'mobile') await expect(wide).toBeHidden();
    else {
      await expect(wide).toHaveAccessibleName('x265 13');
      await expect(chips.getByRole('button', { name: /^Dolby Vision/ })).toBeDisabled();
    }

    await uhd.click();
    await expect(uhd).toHaveAttribute('aria-pressed', 'true');
    await expect(status(page, 3)).toBeVisible();
    await expect(sheet(page).getByRole('article')).toHaveCount(3);
    await expect(chips.getByRole('button', { name: 'Filters, 1 on' })).toBeVisible();
    // Disjunctive: 2160p's own facet keeps its counts, the others narrow
    await expect(chips.getByRole('button', { name: /^1080p/ })).toHaveAccessibleName('1080p 20');
    if (testInfo.project.name === 'desktop') await expect(wide).toHaveAccessibleName('x265 3');
    await expectAccessibleAndContained(page);

    await uhd.click();
    await expect(uhd).toHaveAttribute('aria-pressed', 'false');
    await expect(status(page, 27)).toBeVisible();
  });

  test('the Filters sheet edits a draft: Show N counts it, Cancel drops it, Show applies it', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const opener = chipRow(page).getByRole('button', { name: 'Filters' });
    await opener.click();
    const filters = filtersSheet(page);
    const show = filters.getByRole('button', { name: /^Show / });
    await expect(show).toHaveText('Show 27 results');
    await filters.getByRole('group', { name: 'Pack type' }).getByRole('button', { name: /^Season \d/ }).click();
    await expect(show).toHaveText('Show 7 results');
    const maximum = filters.getByRole('slider', { name: 'Maximum size' });
    await expect(maximum).toHaveAttribute('aria-valuetext', 'No maximum');
    await maximum.fill('50');
    await expect(maximum).toHaveAttribute('aria-valuetext', '1.4 GB');
    // Season packs up to 1.4 GB an episode
    await expect(show).toHaveText('Show 3 results');

    await filters.getByRole('button', { name: 'Cancel' }).click();
    await expect(filters).toBeHidden();
    await expect(opener).toBeFocused();
    await expect(status(page, 27)).toBeVisible();

    // A new draft each time: nothing was applied
    await opener.click();
    await expect(show).toHaveText('Show 27 results');
    await filters.getByRole('group', { name: 'Pack type' }).getByRole('button', { name: /^Season \d/ }).click();
    await expect(show).toHaveText('Show 7 results');
    await show.click();
    await expect(filters).toBeHidden();
    await expect(status(page, 7)).toBeVisible();
    await expect(chipRow(page).getByRole('button', { name: 'Filters, 1 on' })).toBeFocused();

    await chipRow(page).getByRole('button', { name: 'Filters, 1 on' }).click();
    await filters.getByRole('button', { name: 'Reset' }).click();
    await expect(show).toHaveText('Show 27 results');
    // Last: the 320 px check swaps the panel for a drawer, which remounts the openers
    await expectAccessibleAndContained(page);
  });

  test('the Sort sheet: a pick reorders, closes, and the best match steps aside', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const opener = chipRow(page).getByRole('button', { name: 'Sort: Best' });
    await opener.click();
    const sort = page.getByRole('dialog', { name: 'Sort by' });
    await expect(sort.getByRole('button', { name: /^Best match/ })).toHaveAttribute('aria-pressed', 'true');
    await sort.getByRole('button', { name: /^Seeders/ }).click();
    await expect(sort).toBeHidden();
    await expect(chipRow(page).getByRole('button', { name: 'Sort: Seeders' })).toBeFocused();
    await expect(sheet(page).getByRole('heading', { name: 'Results · sorted by seeders' })).toBeVisible();
    await expect(sheet(page).getByText(/Best match/)).toHaveCount(0);
    const cards = sheet(page).getByRole('article');
    await expect(cards.nth(0)).toHaveAccessibleName(BATCH);
    await expect(cards.nth(1)).toHaveAccessibleName('[SubsPlease] Sousou no Frieren - 29 (1080p) [9E3C5D21].mkv');

    await chipRow(page).getByRole('button', { name: 'Sort: Seeders' }).click();
    await sort.getByRole('button', { name: /^Size/ }).click();
    await expect(sheet(page).getByRole('heading', { name: 'Results · sorted by size' })).toBeVisible();
    await expect(cards.nth(0)).toHaveAccessibleName('[MiniFreeza] Frieren - 29 [480p HEVC]');

    await chipRow(page).getByRole('button', { name: 'Sort: Size' }).click();
    await expect(sort.getByRole('button', { name: /^Size/ })).toHaveAttribute('aria-pressed', 'true');
    await expectAccessibleAndContained(page);
  });

  test('filtered to nothing: the filters by name, removable, then Clear all', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    await chipRow(page).getByRole('button', { name: 'Filters' }).click();
    const filters = filtersSheet(page);
    await filters.getByRole('group', { name: 'Resolution' }).getByRole('button', { name: /^2160p/ }).click();
    await filters.getByRole('group', { name: 'Minimum seeders' }).getByRole('button', { name: '100+' }).click();
    await filters.getByRole('slider', { name: 'Maximum size' }).fill('44');
    await filters.getByRole('button', { name: 'Show 0 results' }).click();

    const dialog = sheet(page);
    await expect(dialog.getByRole('heading', { name: 'Your filters hide all 27 results' })).toBeVisible();
    await expect(dialog.getByText('2160p, Up to 1 GB and 100+ seeders together match nothing for this episode.')).toBeVisible();
    const active = dialog.getByRole('group', { name: 'Active filters' });
    await expect(active.getByRole('button')).toHaveText(['2160p', 'Up to 1 GB', '100+ seeders']);
    await expectAccessibleAndContained(page);

    await active.getByRole('button', { name: '100+ seeders' }).click();
    await expect(dialog.getByText('2160p and Up to 1 GB together match nothing for this episode.')).toBeVisible();
    await expect(chipRow(page).getByRole('button', { name: 'Filters, 2 on' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Clear all filters' }).click();
    await expect(status(page, 27)).toBeVisible();
    await expect(chipRow(page).getByRole('button', { name: 'Filters' })).toBeVisible();
  });

  test('Inspect a show’s release: the steps and groups, then Add & review mapping', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('article', { name: BATCH }).getByRole('button', { name: 'Inspect' }).click();
    const inspect = inspectSheet(page);
    await expect(inspect).toHaveAccessibleName(/^Inspect · nothing downloads yet 16 files · \d+\.\d GB$/, { timeout: 15_000 });
    await expect(inspect.getByText('Asked AI about 3 tricky ones')).toBeVisible();
    const willMap = inspect.getByRole('list', { name: 'Will map · 9 episodes' });
    await expect(willMap.getByRole('listitem').first()).toContainText('S01E29 Let’s Go Somewhere We Can’t Reach');
    await expect(willMap.getByText('+ 5 more, all high confidence')).toBeVisible();
    await expect(inspect.getByRole('list', { name: 'You’ll review · 3 files' })).toContainText('AI: "v2" is a re-release of #35');
    await expect(inspect.getByRole('list', { name: 'Specials · 1' })).toContainText('S00E06');
    await expect(inspect.getByRole('list', { name: 'Ignored · 3' })).toContainText('Opening (creditless)');
    await expectAccessibleAndContained(page);

    await inspect.getByRole('button', { name: 'Add & review mapping' }).click();
    await expect(page).toHaveURL(/\/mapping\/\d+$/);
    await expect(page.getByText(BATCH).first()).toBeVisible();
  });

  test('Inspect a movie’s release: the file it would use, then Add & download', async ({ page }) => {
    await signIn(page, BATTLE);
    const best = sheet(page).getByRole('article', { name: BATTLE_BEST });
    await best.getByRole('button', { name: 'Inspect' }).click();
    const inspect = inspectSheet(page);
    await expect(inspect).toHaveAccessibleName(/2 files · 14\.3 GB$/, { timeout: 15_000 });
    await expect(inspect.getByRole('list', { name: 'Will map · 1 file' })).toContainText('One Battle After Another');
    await expect(inspect.getByRole('list', { name: 'Ignored · 1' })).toContainText('Sample');
    await expectAccessibleAndContained(page);

    await inspect.getByRole('button', { name: 'Add & download' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Torrent assigned to movie' })).toBeVisible({ timeout: 20_000 });
    await expect(inspect).toBeHidden();
    await expect(best.getByText('Added')).toBeVisible();
  });

  test('a dry run that fails says why, with Retry', async ({ page }) => {
    await scenario(page, 'job-failed');
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('article', { name: BATCH }).getByRole('button', { name: 'Inspect' }).click();
    const alert = inspectSheet(page).getByRole('alert');
    await expect(alert).toContainText('Couldn’t look inside this torrent');
    await expect(alert).toContainText('No peers sent the torrent’s file list within 60 s.');
    await expect(alert.getByRole('button', { name: 'Retry' })).toBeVisible();
  });

  test('nested sheets: Esc closes only the top one, and focus goes back to its opener', async ({ page }) => {
    await signIn(page, FRIEREN_E29);
    const filtersChip = chipRow(page).getByRole('button', { name: 'Filters' });
    await filtersChip.click();
    await expect(filtersSheet(page)).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(filtersSheet(page)).toBeHidden();
    await expect(sheet(page)).toBeVisible();
    await expect(filtersChip).toBeFocused();

    const inspectButton = sheet(page).getByRole('article', { name: BATCH }).getByRole('button', { name: 'Inspect' });
    await inspectButton.click();
    await expect(inspectSheet(page)).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(inspectSheet(page)).toBeHidden();
    await expect(sheet(page)).toBeVisible();
    await expect(inspectButton).toBeFocused();

    // One more Esc closes the search itself
    await page.keyboard.press('Escape');
    await expect(sheet(page)).toBeHidden();
  });

  test('what the quality profile hid, when nothing else is left', async ({ page }) => {
    await signIn(page, BATTLE);
    const field = sheet(page).getByRole('searchbox', { name: 'Search releases' });
    await field.fill('HDCAM');
    await field.press('Enter');
    await expect(sheet(page).getByRole('heading', { name: 'No releases for “HDCAM”' })).toBeVisible();
    await expect(sheet(page).getByText('1 hidden by your quality profile')).toBeVisible();
    await expect(sheet(page).getByRole('link', { name: 'Quality settings' })).toHaveAttribute('href', '/settings?section=quality');
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

  test('filters', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('button', { name: 'Filters' }).click();
    await expect(page.getByRole('dialog', { name: 'Filters' }).getByRole('button', { name: 'Show 27 results' })).toBeVisible();
    await shoot(page, testInfo, 'filters');
  });

  test('sort', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('button', { name: 'Sort: Best' }).click();
    await expect(page.getByRole('dialog', { name: 'Sort by' })).toBeVisible();
    await shoot(page, testInfo, 'sort');
  });

  test('filtered-empty', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('button', { name: 'Filters' }).click();
    const filters = page.getByRole('dialog', { name: 'Filters' });
    await filters.getByRole('group', { name: 'Resolution' }).getByRole('button', { name: /^2160p/ }).click();
    await filters.getByRole('group', { name: 'Minimum seeders' }).getByRole('button', { name: '100+' }).click();
    await filters.getByRole('button', { name: 'Show 0 results' }).click();
    await expect(sheet(page).getByRole('heading', { name: 'Your filters hide all 27 results' })).toBeVisible();
    await shoot(page, testInfo, 'filtered-empty');
  });

  test('inspect-working', async ({ page }, testInfo) => {
    await scenario(page, 'job-hold:classifying');
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('article', { name: BATCH }).getByRole('button', { name: 'Inspect' }).click();
    await expect(page.getByRole('dialog', { name: /^Inspect/ }).getByText('Fetched file list')).toBeVisible({ timeout: 15_000 });
    await shoot(page, testInfo, 'inspect-working');
  });

  test('inspect', async ({ page }, testInfo) => {
    await signIn(page, FRIEREN_E29);
    await sheet(page).getByRole('article', { name: BATCH }).getByRole('button', { name: 'Inspect' }).click();
    await expect(page.getByRole('dialog', { name: /16 files/ }).getByRole('heading', { name: 'Will map · 9 episodes' })).toBeVisible({ timeout: 15_000 });
    await shoot(page, testInfo, 'inspect');
  });

  test('inspect-movie', async ({ page }, testInfo) => {
    await signIn(page, BATTLE);
    await sheet(page).getByRole('article', { name: BATTLE_BEST }).getByRole('button', { name: 'Inspect' }).click();
    await expect(page.getByRole('dialog', { name: /2 files/ }).getByRole('heading', { name: 'Will map · 1 file' })).toBeVisible({ timeout: 15_000 });
    await shoot(page, testInfo, 'inspect-movie');
  });
});
