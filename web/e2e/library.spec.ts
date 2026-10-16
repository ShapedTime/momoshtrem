import { expect, test, type Page, type Request, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * The Library (PLAN F7, #97) against the MSW mocks' nine titles: Interstellar
 * downloading, One Battle After Another without a torrent, Dune ready, The
 * Studio playing, Frieren, Doctor Who and One Piece with files to review,
 * Bluey and Severance ready.
 */

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

const chip = (page: Page, name: RegExp) => page.getByRole('group', { name: 'Filter' }).getByRole('button', { name });
const card = (page: Page, title: string) => page.locator('article.libcard').filter({ has: page.getByRole('link', { name: title, exact: true }) });
const titles = (page: Page) => page.locator('article.libcard .meta .title');
const moreActions = (page: Page, title: string) => page.getByRole('button', { name: `${title}: more actions` });

/** The DELETEs this page sends. */
function deletes(page: Page): Request[] {
  const sent: Request[] = [];
  page.on('request', (request) => {
    if (request.method() === 'DELETE' && /\/api\/(movies|shows)\/\d+/.test(request.url())) sent.push(request);
  });
  return sent;
}

async function openRemove(page: Page, title: string) {
  await moreActions(page, title).click();
  await page.getByRole(isDesktopPage(page) ? 'menuitem' : 'button', { name: 'Remove from library…' }).click();
  return page.getByRole('alertdialog', { name: `Remove ${title} from your library?` });
}

const isDesktopPage = (page: Page) => (page.viewportSize()?.width ?? 0) >= 1024;

test.describe('library', () => {
  test('shows every title with its status, totals and filter counts', async ({ page }) => {
    await signIn(page, '/library');
    await expect(page.getByRole('heading', { name: 'Library', level: 1 })).toBeVisible();
    await expect(page).toHaveTitle('Library · momoshtrem');
    await expect(page.getByText('9 titles · 699 GB · 5 ready in Infuse')).toBeVisible();
    await expect(titles(page)).toHaveCount(9);
    for (const [name, count] of [['All', 9], ['Movies', 3], ['Shows', 6], ['Watching', 4], ['Ready', 5], ['Incomplete', 4], ['Needs review', 3]] as const) {
      await expect(chip(page, new RegExp(`^${name} ${count}$`))).toBeVisible();
    }
    await expect(chip(page, /^All/)).toHaveAttribute('aria-pressed', 'true');

    // Status without hover: the pills that want attention and the status lines
    await expect(card(page, 'Interstellar')).toContainText('42%');
    await expect(card(page, 'Interstellar')).toContainText('Downloading 42%');
    await expect(card(page, 'One Battle After Another')).toContainText('No torrent');
    await expect(card(page, 'The Studio')).toContainText('Playing');
    await expect(card(page, 'Frieren: Beyond Journey’s End')).toContainText('Review');
    await expect(card(page, 'Frieren: Beyond Journey’s End')).toContainText('28/38 ready');
    // What Trakt says was watched
    await expect(card(page, 'Frieren: Beyond Journey’s End')).toContainText('26 of 38 watched');
    await expect(card(page, 'Severance')).toContainText('Watched all');
    await expect(card(page, 'Bluey')).toContainText('Not started');
    await expect(card(page, 'Dune: Part Two')).toContainText('Watched');
    await expect(card(page, 'Dune: Part Two')).toContainText('Ready');
    await expect(moreActions(page, 'Dune: Part Two')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('filters, sorts and switches to the list, all in the URL', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await chip(page, /^Movies/).click();
    await expect(page).toHaveURL(/\/library\?filter=movies$/);
    await expect(titles(page)).toHaveText(['Interstellar', 'One Battle After Another', 'Dune: Part Two']);

    await chip(page, /^Needs review/).click();
    await expect(page).toHaveURL(/filter=review/);
    await expect(titles(page)).toHaveCount(3);

    await page.getByLabel('Sort by').selectOption('title');
    await expect(page).toHaveURL(/filter=review&sort=title/);
    await expect(titles(page)).toHaveText(['Doctor Who', 'Frieren: Beyond Journey’s End', 'One Piece']);

    // Shows partway through (Trakt): not Severance (caught up) or Bluey (not started)
    await chip(page, /^Watching/).click();
    await expect(page).toHaveURL(/filter=watching&sort=title/);
    await expect(titles(page)).toHaveText(['Doctor Who', 'Frieren: Beyond Journey’s End', 'One Piece', 'The Studio']);
    await expect(card(page, 'The Studio')).toContainText('9 of 10 watched');
    await chip(page, /^All/).click();
    await expect(page).toHaveURL(/\/library\?sort=title$/);

    await page.getByRole('button', { name: 'List' }).click();
    await expect(page).toHaveURL(/sort=title&view=list/);
    const rows = page.getByRole('list', { name: 'Your library' }).getByRole('listitem');
    await expect(rows).toHaveCount(9);
    await expect(rows.first()).toContainText('Bluey');
    await expect(rows.filter({ hasText: 'Frieren' })).toContainText('2 to review');
    await expect(rows.filter({ hasText: 'The Studio' })).toContainText('Streaming');
    await expect(rows.filter({ hasText: 'One Battle After Another' })).toContainText('Never');
    if (isDesktop(testInfo)) await expect(page.locator('.lrows-head')).toHaveText(/Title\s*Status\s*Watched\s*Size\s*Last watched/);
    await expectAccessibleAndContained(page);

    // Reload: the URL brings the same view back
    await page.reload();
    await expect(page.getByRole('button', { name: 'List' })).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByLabel('Sort by')).toHaveValue('title');
  });

  test('Last watched sorts by Trakt’s last play, never-watched titles last', async ({ page }) => {
    await signIn(page, '/library?sort=last_watched');
    await expect(page.getByLabel('Sort by')).toHaveValue('last_watched');
    // Dune and Frieren were last played the same day: then the newest added first
    await expect(titles(page).first()).toHaveText('The Studio');
    await expect(titles(page).nth(1)).toHaveText('Dune: Part Two');
    await expect(titles(page).nth(2)).toHaveText('Frieren: Beyond Journey’s End');
    await expect(titles(page).last()).toHaveText(/Bluey|One Battle After Another/);
  });

  test('Trakt off: no watch lines, and Watching says how to fill it', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'trakt-off'));
    await signIn(page, '/library');
    await expect(chip(page, /^Watching 0$/)).toBeVisible();
    // A show would read "Not started" whatever you'd watched; a movie keeps its runtime
    await expect(card(page, 'Frieren: Beyond Journey’s End')).toContainText('28/38 ready');
    await expect(card(page, 'Frieren: Beyond Journey’s End')).not.toContainText('watched');
    await expect(card(page, 'Bluey')).not.toContainText('Not started');
    await expect(card(page, 'Dune: Part Two')).toContainText('2h 47m');
    await expect(page.locator('article.libcard .progress--watched')).toHaveCount(0);

    await chip(page, /^Watching/).click();
    await expect(page.getByRole('heading', { name: 'Connect Trakt to see what you’re watching' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Connect Trakt' })).toHaveAttribute('href', '/settings?section=trakt');
    await expectAccessibleAndContained(page);
    await page.getByRole('button', { name: 'Show all titles' }).click();
    await expect(page).toHaveURL(/\/library$/);

    await page.getByRole('button', { name: 'List' }).click();
    const rows = page.getByRole('list', { name: 'Your library' }).getByRole('listitem');
    await expect(rows.filter({ hasText: 'Bluey' })).not.toContainText('Not started');
    await expect(rows.filter({ hasText: 'One Battle After Another' })).not.toContainText('Never');
    await expect(rows.filter({ hasText: 'The Studio' })).toContainText('Now');
    if (isDesktop(testInfo)) await expectAccessibleAndContained(page);
  });

  test('searches the library, and offers to search everywhere when nothing matches', async ({ page }) => {
    await signIn(page, '/library');
    const search = page.getByRole('searchbox', { name: 'Search your library' });
    await search.fill('one');
    await expect(page).toHaveURL(/q=one/);
    await expect(titles(page)).toHaveText(['One Battle After Another', 'One Piece']);
    await expect(chip(page, /^All 2$/)).toBeVisible();

    await search.fill('zzz');
    await expect(page.getByRole('heading', { name: 'No titles match “zzz”' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Search everywhere' })).toHaveAttribute('href', '/search?q=zzz');
    await expectAccessibleAndContained(page);
    await page.getByRole('button', { name: 'Clear search' }).first().click();
    await expect(search).toHaveValue('');
    await expect(page).toHaveURL(/\/library$/);
    await expect(titles(page)).toHaveCount(9);
  });

  test('the “…” menu: a popover on desktop, a sheet on phone', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await moreActions(page, 'Frieren: Beyond Journey’s End').click();
    if (isDesktop(testInfo)) {
      const menu = page.getByRole('menu', { name: 'More actions' });
      await expect(menu.getByRole('menuitem')).toHaveText([/Open details/, /Review mapping\s*2 files/, /Find missing episodes\s*10/, /Copy WebDAV path/, /Remove from library…/]);
      await expect(menu.getByRole('menuitem', { name: /Review mapping/ })).toHaveAttribute('href', '/mapping/101');
      // The modal menu hides the page from assistive tech meanwhile, so check the menu itself
      await expectNoA11yViolations(page, { include: '[role="menu"]' });
      await page.keyboard.press('Escape');
      await expect(menu).toBeHidden();
    } else {
      const sheet = page.getByRole('dialog', { name: 'Frieren: Beyond Journey’s End' });
      await expect(sheet).toContainText('28/38 ready · 39.2 GB');
      await expect(sheet.getByRole('link', { name: /Review mapping/ })).toHaveAttribute('href', '/mapping/101');
      await expect(sheet.getByRole('button', { name: /Find missing episodes/ })).toBeVisible();
      await expectNoA11yViolations(page);
      await sheet.getByRole('button', { name: 'Close' }).click();
      await expect(sheet).toBeHidden();
    }
    await expect(moreActions(page, 'Frieren: Beyond Journey’s End')).toBeFocused();

    // Copy WebDAV path says where to paste it
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await moreActions(page, 'Dune: Part Two').click();
    await page.getByRole(isDesktop(testInfo) ? 'menuitem' : 'button', { name: 'Copy WebDAV path' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Copied' })).toContainText('Paste it into Infuse › Add files › WebDAV');
  });

  test('Find missing episodes opens torrent search on the first missing one', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await moreActions(page, 'Frieren: Beyond Journey’s End').click();
    await page.getByRole(isDesktop(testInfo) ? 'menuitem' : 'button', { name: /Find missing episodes/ }).click();
    await expect(page).toHaveURL(/\/library\?sheet=torrent&show=209867&season=1&se=S01E29$/);
    const search = page.getByRole('dialog', { name: /^Find a source · Frieren: Beyond Journey’s End S01E29/ });
    await expect(search.getByRole('article', { name: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]' })).toContainText('Best match');
  });

  test('remove, then Undo: the title comes back and nothing is sent', async ({ page }) => {
    const sent = deletes(page);
    await signIn(page, '/library');
    const dialog = await openRemove(page, 'Interstellar');
    await expect(dialog).toContainText('This removes the movie and its mapping. Infuse stops showing it. Torrents other titles use are kept.');
    await expect(dialog.getByRole('checkbox', { name: 'Also delete downloaded data (up to 63.1 GB)' })).toBeChecked();
    await expectNoA11yViolations(page);
    await dialog.getByRole('button', { name: 'Remove', exact: true }).click();

    const toast = page.getByRole('status').filter({ hasText: 'Removed Interstellar' });
    await expect(toast).toContainText('Undo within 10 seconds');
    await expect(card(page, 'Interstellar')).toHaveCount(0);
    await expect(page.getByText('8 titles · 636 GB · 5 ready in Infuse')).toBeVisible();
    await expect(chip(page, /^Movies 2$/)).toBeVisible();

    await toast.getByRole('button', { name: 'Undo' }).click();
    await expect(card(page, 'Interstellar')).toHaveCount(1);
    await expect(chip(page, /^Movies 3$/)).toBeVisible();
    await page.waitForTimeout(11_000);
    expect(sent).toHaveLength(0);
    await expect(card(page, 'Interstellar')).toHaveCount(1);
  });

  test('remove, then wait: the DELETE goes out with delete_data when the toast closes', async ({ page }) => {
    const sent = deletes(page);
    await signIn(page, '/library');
    await (await openRemove(page, 'Interstellar')).getByRole('button', { name: 'Remove', exact: true }).click();
    await expect(card(page, 'Interstellar')).toHaveCount(0);
    const request = await page.waitForRequest((r) => r.method() === 'DELETE' && r.url().includes('/api/movies/2'), { timeout: 15_000 });
    expect(new URL(request.url()).searchParams.get('delete_data')).toBe('true');
    await expect(page.getByRole('status').filter({ hasText: 'Removed Interstellar' })).toBeHidden();
    await expect(page.getByText('8 titles · 636 GB · 5 ready in Infuse')).toBeVisible();
    await expect(card(page, 'Interstellar')).toHaveCount(0);

    // Unticked, and dismissing the toast sends it at once
    const dialog = await openRemove(page, 'Bluey');
    await dialog.getByRole('checkbox').uncheck();
    await dialog.getByRole('button', { name: 'Remove', exact: true }).click();
    await page.getByRole('status').filter({ hasText: 'Removed Bluey' }).getByRole('button', { name: 'Dismiss' }).click();
    await expect.poll(() => sent.map((r) => new URL(r.url()).pathname + new URL(r.url()).search)).toEqual([
      '/api/movies/2?delete_data=true',
      '/api/shows/5?delete_data=false',
    ]);
    await expect(card(page, 'Bluey')).toHaveCount(0);
  });

  test('loading, first-time empty, nothing to review and a failed load', async ({ page }) => {
    await signIn(page, '/library');
    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'library-loading'));
    await page.reload();
    await expect(page.getByRole('status').filter({ hasText: 'Loading your library' })).toBeAttached();
    await expect(page.getByText('Loading…', { exact: true })).toBeVisible();

    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'library-empty'));
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Your library is empty' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Discover' }).last()).toBeVisible();
    await expect(page.getByRole('link', { name: 'Search titles' })).toHaveAttribute('href', '/search');
    await expectAccessibleAndContained(page);

    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'library-reviewed'));
    await page.goto('/library?filter=review');
    await expect(page.getByRole('heading', { name: 'Nothing needs review' })).toBeVisible();
    await expect(chip(page, /^Needs review 0$/)).toBeVisible();
    await expectAccessibleAndContained(page);

    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'library-error'));
    await page.goto('/library');
    const banner = page.getByRole('alert').filter({ hasText: 'Couldn’t load your library' });
    await expect(banner).toBeVisible({ timeout: 20_000 });
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await banner.getByRole('button', { name: 'Try again' }).click();
    await expect(titles(page)).toHaveCount(9);
  });
});
