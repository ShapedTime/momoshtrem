import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';

/**
 * Title pages against a backend without downloads (before #77's P4 and P6, the
 * `downloads-absent` scenario): titles, seasons and episodes carry no `keep`, so
 * no Keep is offered anywhere: not the movie panel's second action, the ⋯ menu,
 * the season row, an episode row's pill or the episode sheet. With downloads,
 * keep.spec.ts finds each of them on these titles: Dune: Part Two is ready and
 * nothing keeps it, Interstellar is kept, Bluey season 3 is kept, and Frieren
 * S01E27 is a smart pick.
 */

const DUNE = 'Dune: Part Two';
const KEEP_ITEM = /keep|Change duration/i;

const panel = (page: Page, name: string | RegExp) => page.getByRole('region', { name });
const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'downloads-absent'));
});

/** Opens the ⋯ menu (a menu on desktop, the "More" sheet on phone) and checks it has no Keep item. */
async function expectNoKeepInMore(page: Page, testInfo: TestInfo, title: string) {
  await page.getByRole('button', { name: `More actions for ${title}` }).click();
  if (phone(testInfo)) {
    const more = page.getByRole('dialog', { name: 'More' });
    await expect(more.getByRole('button', { name: 'Refresh from TMDB' })).toBeVisible();
    await expect(more.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
  } else {
    await expect(page.getByRole('menuitem', { name: 'Refresh from TMDB' })).toBeVisible();
    await expect(page.getByRole('menuitem', { name: KEEP_ITEM })).toHaveCount(0);
  }
  await page.keyboard.press('Escape');
}

test.describe('Download & keep without a backend for it', () => {
  test('a movie: no second action, nothing in the ⋯ menu, and a Keep link says so', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    await expect(panel(page, 'Ready in Infuse')).toBeVisible();
    await expect(page.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
    await expectNoKeepInMore(page, testInfo, DUNE);

    // Kept with downloads: no keep line, and no way to change or stop it
    await page.goto('/movie/157336');
    await expect(page.locator('.primary-panel')).toBeVisible();
    await expect(page.locator('.primary-panel')).not.toContainText(/Kept/);
    await expect(page.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
    await expectNoKeepInMore(page, testInfo, 'Interstellar');

    // A link to the Keep sheet explains instead of offering one
    await page.goto('/movie/693134?sheet=keep&movie=693134');
    const sheet = page.getByRole('dialog');
    await expect(sheet).toContainText('This momoshtrem can’t keep things on disk yet. Everything still streams to Infuse on demand.');
    await expect(sheet.getByRole('button', { name: 'Download & keep' })).toHaveCount(0);
  });

  test('a show: no season keep, no keep pills, nothing in the ⋯ menu or the episode sheet', async ({ page }, testInfo) => {
    await signIn(page, '/tv/209867');
    const row = (n: number) => page.locator('.eprow').filter({ hasText: `S01E${n}` });
    await expect(row(27)).toBeVisible();
    await expect(row(27).getByText('Smart', { exact: true })).toHaveCount(0);
    // No "Download & keep season", and the rows' options don't name a keep
    await expect(row(27).getByRole('button', { name: 'S01E27 options: how to play, subtitles, replace, remove' })).toBeVisible();
    await expect(page.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
    await expectNoKeepInMore(page, testInfo, 'Frieren: Beyond Journey’s End');

    // The smart pick's episode sheet has no keep section
    await page.getByRole('button', { name: /^S01E27 options/ }).click();
    const smart = page.getByRole('dialog', { name: 'An Ordinary Mage' });
    await expect(smart.getByRole('heading', { name: 'Play in Infuse' })).toBeVisible();
    await expect(smart.locator('section[aria-labelledby="episode-keep-h"]')).toHaveCount(0);
    await expect(smart.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);

    // Bluey's kept season 3: no season button or Kept pills, and the episode sheet says nothing of a keep
    await page.goto('/tv/82728?season=3');
    await expect(page.locator('.eprow').first()).toBeVisible();
    await expect(page.locator('.eprow').getByText('Kept', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
    await page.goto('/tv/82728?season=3&sheet=episode&tmdb=82728&se=S03E01');
    const episode = page.getByRole('dialog', { name: 'Episode 1' });
    await expect(episode.getByRole('heading', { name: 'Play in Infuse' })).toBeVisible();
    await expect(episode.getByRole('region', { name: 'Kept', exact: true })).toHaveCount(0);
    await expect(episode.locator('section[aria-labelledby="episode-keep-h"]')).toHaveCount(0);
    await expect(episode.getByRole('button', { name: KEEP_ITEM })).toHaveCount(0);
  });
});
