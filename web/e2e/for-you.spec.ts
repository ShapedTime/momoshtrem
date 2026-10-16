import { expect, test, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * For You (`/`, PLAN F5, #135) against the MSW mocks' Trakt account (#130).
 * Up next: The Studio downloading, Frieren ready, One Piece needing review,
 * The Good Place not in the library, Doctor Who needing a torrent, Bluey
 * ready (a watchlist start), Severance airing next week. Scenarios
 * (src/mocks/scenario.ts): trakt-off, first-time, trakt-syncing,
 * trakt-sync-error, trakt-reauth, up-next-empty, me-error, trakt-loading.
 */

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page, exclude: string[] = []) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
  await expectNoA11yViolations(page, { exclude });
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
const region = (page: Page, name: string) => page.getByRole('region', { name, exact: true });
const hero = (page: Page) => page.locator('section.home-hero');
const card = (page: Page, rail: string, show: string) => region(page, rail).locator('article.lcard').filter({ hasText: show });

test.describe('For You', () => {
  test('the personal page: hero, Up next with every availability, new episodes, the watchlist and trending', async ({ page }) => {
    await signIn(page, '/');
    await expect(page).toHaveTitle('For You · momoshtrem');
    await expect(page.getByRole('heading', { name: 'For You', level: 1 })).toBeAttached();

    // The hero: a ready Up next show, played from the episode's file
    await expect(hero(page).getByText('Ready in Infuse')).toBeVisible();
    await expect(hero(page).getByText(/^Up next: S01E\d\d/)).toBeVisible();
    await expect(hero(page).getByRole('button', { name: 'Play in Infuse' })).toBeVisible();
    await expect(hero(page).getByRole('link', { name: 'Details' })).toHaveAttribute('href', /^\/tv\/\d+$/);

    const upNext = region(page, 'Up next');
    await expect(upNext.getByText('From your Trakt progress')).toBeVisible();
    await expect(upNext.locator('article.lcard')).toHaveCount(7);
    await expect(card(page, 'Up next', 'The Studio')).toContainText('Downloading 37%');
    await expect(card(page, 'Up next', 'The Studio').getByRole('link', { name: 'Download details for The Studio S01E10' })).toHaveAttribute('href', '/activity/seed-5');
    await expect(card(page, 'Up next', 'Frieren')).toContainText('Ready to watch');
    await expect(card(page, 'Up next', 'One Piece')).toContainText('S21E01 · #892');
    await expect(card(page, 'Up next', 'One Piece').getByRole('link', { name: /^Review the files for One Piece/ })).toHaveAttribute('href', '/mapping/102');
    await expect(card(page, 'Up next', 'The Good Place')).toContainText('Not in library');
    await expect(card(page, 'Up next', 'Doctor Who')).toContainText('Needs a torrent');
    await expect(card(page, 'Up next', 'Severance')).toContainText(/Airs \w+/);
    await expect(card(page, 'Up next', 'Severance').getByRole('button')).toHaveCount(0);

    await expect(region(page, 'New episodes').getByText('For shows you watch')).toBeVisible();
    await expect(region(page, 'New episodes').locator('article.lcard').first()).toBeVisible();
    await expect(region(page, 'Your Trakt watchlist').getByText('4 titles · 2 in your library')).toBeVisible();
    await expect(region(page, 'Your Trakt watchlist').locator('article.pcard')).toHaveCount(4);
    await expect(region(page, 'Trending this week').locator('article.pcard').first()).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('the hero’s pager changes the pick, and Play in Infuse opens the setup sheet with the episode', async ({ page }) => {
    await signIn(page, '/');
    const title = hero(page).getByRole('heading', { level: 2 });
    await expect(title).not.toHaveText(/Loading/);
    const first = await title.textContent();
    const pager = hero(page).getByRole('group', { name: 'Picks' });
    await expect(pager.getByRole('button', { name: 'Pick 1 of 3' })).toHaveAttribute('aria-pressed', 'true');
    await pager.getByRole('button', { name: 'Pick 3 of 3' }).click();
    await expect(pager.getByRole('button', { name: 'Pick 3 of 3' })).toHaveAttribute('aria-pressed', 'true');
    await expect(hero(page).getByText('Trending this week')).toBeVisible();
    await expect(title).not.toHaveText(first ?? '');

    await pager.getByRole('button', { name: 'Pick 1 of 3' }).click();
    await hero(page).getByRole('button', { name: 'Play in Infuse' }).click();
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet).toBeVisible();
    await expect(page).toHaveURL(/sheet=infuse-setup&path=/);
    await expect(sheet.getByText('Find it under Library')).toBeVisible();
    await expect(sheet.getByLabel('WebDAV path of this title')).toBeVisible();
  });

  test('phone flow: Up next → Find → the torrent sheet on that episode', async ({ page }) => {
    await signIn(page, '/');
    await card(page, 'Up next', 'Doctor Who').getByRole('button', { name: 'Find a torrent for Doctor Who S03E04' }).click();
    await expect(page).toHaveURL(/\/\?sheet=torrent&show=57243&se=S03E04$/);
    const sheet = page.getByRole('dialog', { name: /^Find a source · Doctor Who/ });
    await expect(sheet).toBeVisible();
    await expect(sheet.getByText(/S03E04/).first()).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/$/);
  });

  test('Play in Infuse and Add on episode cards', async ({ page }) => {
    await signIn(page, '/');
    await card(page, 'Up next', 'Frieren').getByRole('button', { name: /^Play Frieren.* S01E27 in Infuse$/ }).click();
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet.getByText(/S01E27 - An Ordinary Mage/).first()).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();

    // Add the show; the card flips to Find
    const goodPlace = card(page, 'Up next', 'The Good Place');
    await goodPlace.getByRole('button', { name: 'Add The Good Place to your library' }).click();
    await expect(page.getByText('Added to library')).toBeVisible();
    await expect(goodPlace.getByRole('button', { name: 'Find a torrent for The Good Place S01E06' })).toBeVisible();
    await expect(goodPlace).toContainText('Needs a torrent');
  });

  test('Trakt off: a trending hero, Make this page yours, new episodes from the library and trending rails', async ({ page }) => {
    await scenario(page, 'trakt-off');
    await signIn(page, '/');
    await expect(hero(page).getByText('Trending this week')).toBeVisible();
    await expect(hero(page).getByRole('link', { name: 'Details' })).toBeVisible();
    const connect = page.getByRole('region', { name: 'Make this page yours' });
    await expect(connect.getByRole('link', { name: 'Connect Trakt' })).toHaveAttribute('href', '/settings?section=trakt');
    await expect(connect.getByRole('link', { name: 'Browse instead' })).toHaveAttribute('href', '/discover');
    await expect(region(page, 'Up next')).toHaveCount(0);
    await expect(region(page, 'New episodes').getByText('From your library')).toBeVisible();
    for (const rail of ['Trending this week', 'Popular anime', 'Popular movies']) {
      await expect(region(page, rail).locator('article.pcard').first()).toBeVisible();
    }
    // Library pills on TMDB cards
    await expect(region(page, 'Popular movies').locator('article.pcard').filter({ hasText: 'Dune: Part Two' })).toContainText('Ready');
    // On a 390 × 844 phone, Connect Trakt's amber sits right under the glass tab bar, which
    // dims its labels there; the tab bar's contrast depends on what scrolls under it, not on this page
    await expectAccessibleAndContained(page, ['.tabbar']);
  });

  test('first time: Noir’s three steps, and trending while it gets to know you', async ({ page }) => {
    await scenario(page, 'first-time');
    await signIn(page, '/');
    await expect(page.getByRole('heading', { name: 'Evening. I’m Noir.', level: 1 })).toBeVisible();
    await expect(hero(page)).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Connect Trakt' })).toHaveAttribute('href', '/settings?section=trakt');
    const trending = region(page, 'Trending this week');
    await expect(trending.getByText('From TMDB, while I get to know you')).toBeVisible();
    await expect(trending.locator('article.pcard').first()).toBeVisible();
    await expect(trending.locator('.pill')).toHaveCount(0);
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Show me how to set up Infuse' }).click();
    await expect(page.getByRole('dialog', { name: 'Play it in Infuse' })).toBeVisible();
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: 'Search for a title' }).click();
    await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeVisible();
  });

  test('first sync: the steps, and Up next as it fills', async ({ page }) => {
    await scenario(page, 'trakt-syncing');
    await signIn(page, '/');
    const sync = page.getByRole('region', { name: 'First sync with Trakt' });
    await expect(sync.getByText(/^Imported \d[\d,]* plays, \d+ ratings and 4 watchlist items so far/)).toBeVisible();
    const steps = sync.getByRole('listitem');
    await expect(steps).toHaveCount(5);
    await expect(steps.nth(0)).toContainText(/Imported your history\s*\d[\d,]* plays/);
    await expect(steps.nth(3)).toContainText('Found Up next for 6 shows');
    await expect(steps.nth(4)).toHaveAttribute('aria-current', 'step');
    await expect(region(page, 'Up next').locator('article.lcard').first()).toBeVisible();
    await expect(hero(page)).toHaveCount(0);
    await expectAccessibleAndContained(page);
  });

  test('first sync stopped: Try again picks it up, and For You fills in when it’s done', async ({ page }) => {
    await scenario(page, 'trakt-sync-error');
    await signIn(page, '/');
    await expect(page.getByText('The first sync stopped at your watchlist')).toBeVisible();
    await expect(page.getByText(/Trakt didn’t answer while importing your watchlist \(502\)/)).toBeVisible();
    await expect(page.getByRole('listitem').filter({ hasText: 'Importing your watchlist' })).toContainText('Stopped here');
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByText(/^Imported .* so far/)).toBeVisible();
    // The mock imports a step per poll (every 3 s); then the personal page takes over
    await expect(region(page, 'Your Trakt watchlist')).toBeVisible({ timeout: 30_000 });
    await expect(region(page, 'Up next').locator('article.lcard').first()).toBeVisible();
  });

  test('reauth: the imported rows stay, with a way to reconnect', async ({ page }) => {
    await scenario(page, 'trakt-reauth');
    await signIn(page, '/');
    await expect(page.getByText('Trakt needs you to sign in again')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Reconnect' })).toHaveAttribute('href', '/settings?section=trakt');
    await expect(region(page, 'Up next').locator('article.lcard').first()).toBeVisible();
  });

  test('nothing up next says so, and the other rows stay', async ({ page }) => {
    await scenario(page, 'up-next-empty');
    await signIn(page, '/');
    await expect(region(page, 'Up next').getByText(/You’re caught up/)).toBeVisible();
    await expect(region(page, 'Your Trakt watchlist').locator('article.pcard')).toHaveCount(4);
    await expectAccessibleAndContained(page);
  });

  test('when the Trakt rows fail, one banner offers Try again and trending still works', async ({ page }) => {
    await scenario(page, 'me-error');
    await signIn(page, '/');
    await expect(page.getByText('Couldn’t load your Trakt rows')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
    await expect(region(page, 'Trending this week').locator('article.pcard').first()).toBeVisible();
    await expect(hero(page).getByText('Trending this week')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('loading: skeletons the size of the page', async ({ page }) => {
    await scenario(page, 'trakt-loading');
    await signIn(page, '/');
    await expect(page.getByRole('status').filter({ hasText: 'Loading For You' })).toBeAttached();
    await expect(page.locator('.home-body .skel').first()).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });
});
