import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Search and the ⌘K palette (PLAN F6, #101) against the MSW fixtures:
 * TMDB from src/mocks/fixtures/title-search.ts ("dune" finds six titles,
 * one of them, Dune: Part Two, in the library), the library, mapping jobs
 * and torrents from the other mocks.
 */

const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page, include?: string) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
  await expectNoA11yViolations(page, include ? { include } : {});
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

const field = (page: Page) => page.getByRole('searchbox', { name: 'Search shows, movies and people' });
const tab = (page: Page, name: RegExp) => page.getByRole('tablist', { name: 'Result type' }).getByRole('tab', { name });
const librarySection = (page: Page) => page.getByRole('region', { name: 'In your library' });
const libraryRows = (page: Page) => librarySection(page).locator('article.srch-row');
const tmdbCards = (page: Page) => page.locator('.srch-hit article.pcard');
const cardTitles = (page: Page) => page.locator('.srch-hit article.pcard .meta .title');

test.describe('search page', () => {
  test('tabs with counts, your library first, then TMDB', async ({ page }) => {
    await signIn(page, '/search?q=dune');
    await expect(page.getByRole('heading', { name: 'Search', level: 1 })).toBeVisible();
    await expect(field(page)).toHaveValue('dune');
    await expect(page).toHaveTitle('“dune” · momoshtrem');

    await expect(tab(page, /^All\s*6$/)).toHaveAttribute('aria-selected', 'true');
    await expect(tab(page, /^Shows\s*2$/)).toBeVisible();
    await expect(tab(page, /^Movies\s*4$/)).toBeVisible();

    await expect(libraryRows(page)).toHaveCount(1);
    const row = libraryRows(page).first();
    await expect(row.getByRole('link', { name: 'Dune: Part Two' })).toHaveAttribute('href', '/movie/693134');
    await expect(row).toContainText('2024 · Movie · 2160p · 18.4 GB');
    await expect(row).toContainText('Ready in Infuse');
    // The match is marked
    await expect(row.locator('mark')).toHaveText('Dune');

    await expect(page.getByRole('heading', { name: 'Shows and movies' })).toBeVisible();
    await expect(page.getByText('5 on TMDB')).toBeVisible();
    await expect(cardTitles(page)).toHaveText(['Dune', 'Dune', 'Dune: Prophecy', 'Jodorowsky’s Dune', 'Frank Herbert’s Dune']);
    await expect(tmdbCards(page).first()).toContainText('2021 · Movie');

    await tab(page, /^Shows/).click();
    await expect(page).toHaveURL(/[?&]tab=tv/);
    await expect(librarySection(page)).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Shows', exact: true })).toBeVisible();
    await expect(cardTitles(page)).toHaveText(['Dune: Prophecy', 'Frank Herbert’s Dune']);

    await tab(page, /^Movies/).click();
    await expect(page).toHaveURL(/[?&]tab=movie/);
    await expect(libraryRows(page)).toHaveCount(1);
    await expect(cardTitles(page)).toHaveText(['Dune', 'Dune', 'Jodorowsky’s Dune']);
  });

  test('a library row’s action fits its state', async ({ page }) => {
    await signIn(page, '/search?q=dune');
    await libraryRows(page).first().getByRole('button', { name: /^Play Dune: Part Two/ }).click();
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet).toBeVisible();
    await expect(page).toHaveURL(/sheet=infuse-setup/);
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();

    // Frieren has files to review (job 101): Review goes to the mapping
    await field(page).fill('frieren');
    const frieren = libraryRows(page).filter({ hasText: 'Frieren' });
    await expect(frieren).toContainText('28 of 38 ready · 2 files to check');
    await frieren.getByRole('link', { name: 'Review Frieren: Beyond Journey’s End mapping' }).click();
    await expect(page).toHaveURL(/\/mapping\/101$/);
  });

  test('typing searches as you go; people and original titles say why; an empty tab offers everything', async ({ page }) => {
    await signIn(page, '/search');
    await field(page).fill('vince gilligan');
    await expect(page).toHaveURL(/\/search\?q=vince\+gilligan$/);
    await expect(cardTitles(page)).toHaveText(['Breaking Bad', 'Better Call Saul', 'El Camino: A Breaking Bad Movie']);
    await expect(tmdbCards(page).first()).toContainText('Created by Vince Gilligan');
    await expect(tmdbCards(page).last()).toContainText('Written by Vince Gilligan');

    // An alternative title finds a library title, which says why
    await field(page).fill('sousou no frieren');
    await expect(libraryRows(page)).toHaveCount(1);
    await expect(libraryRows(page).first()).toContainText('2023 · Show · Original title match');

    await tab(page, /^Movies\s*0$/).click();
    await expect(page.getByText('No movies for “sousou no frieren”.')).toBeVisible();
    await page.getByRole('button', { name: 'See the 1 result' }).click();
    await expect(tab(page, /^All/)).toHaveAttribute('aria-selected', 'true');
    await expect(libraryRows(page)).toHaveCount(1);

    // Clearing goes back to the idle page
    await page.getByRole('button', { name: 'Clear search' }).click();
    await expect(field(page)).toHaveValue('');
    await expect(field(page)).toBeFocused();
    await expect(page.getByRole('heading', { name: 'Recent searches' })).toBeVisible();
  });

  test('nothing found: tips and what’s popular', async ({ page }) => {
    await signIn(page, '/search?q=zzxq');
    await expect(page.getByRole('heading', { name: 'Nothing for “zzxq”' })).toBeVisible();
    const tips = page.getByRole('list', { name: 'Search tips' });
    await expect(tips).toContainText('“sousou no frieren” finds Frieren');
    await expect(tips.getByRole('link', { name: 'Browse Discover' })).toHaveAttribute('href', '/discover');
    await expect(page.getByRole('heading', { name: 'Popular right now' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Popular right now' }).locator('article.pcard')).toHaveCount(6);
    await expectAccessibleAndContained(page);
  });

  test('idle: recent searches are kept, removable and clearable; moods open Discover', async ({ page }) => {
    await signIn(page, '/search');
    const recent = page.getByRole('region', { name: 'Recent searches' });
    await expect(recent.getByText('No recent searches.')).toBeVisible();

    for (const query of ['dune', 'frieren']) {
      await field(page).fill(query);
      await field(page).press('Enter');
      await expect(page).toHaveURL(new RegExp(`q=${query}`));
    }
    await page.getByRole('button', { name: 'Clear search' }).click();
    await expect(page).toHaveURL(/\/search$/);
    await expect(recent.getByRole('link')).toHaveText(['frieren', 'dune']);

    // Kept across a reload
    await page.reload();
    await expect(recent.getByRole('link')).toHaveText(['frieren', 'dune']);

    await recent.getByRole('button', { name: 'Remove frieren from recent searches' }).click();
    await expect(recent.getByRole('link')).toHaveText(['dune']);
    await recent.getByRole('link', { name: 'dune' }).click();
    await expect(field(page)).toHaveValue('dune');
    await expect(libraryRows(page)).toHaveCount(1);

    await page.getByRole('button', { name: 'Clear search' }).click();
    await recent.getByRole('button', { name: 'Clear' }).click();
    await expect(recent.getByText('No recent searches.')).toBeVisible();

    const moods = page.getByRole('region', { name: 'Not sure what you want?' });
    await expect(moods.getByRole('link')).toHaveText(['Cosy', 'Mind-bending', 'Slow cinema', 'Laugh-out-loud', 'Short & sharp', 'Epic nights']);
    await moods.getByRole('link', { name: 'Cosy' }).click();
    await expect(page).toHaveURL(/\/discover\?.*mood=cosy/);
    await expect(page.getByRole('group', { name: 'Moods' }).getByRole('button', { name: 'Cosy' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('a TMDB failure keeps your library’s matches and offers a retry', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'title-search-error'));
    await signIn(page, '/search?q=dune');
    const alert = page.getByRole('alert').filter({ hasText: 'Couldn’t search TMDB' });
    await expect(alert).toContainText('Your library and Infuse aren’t affected.');
    await expect(alert.getByRole('button', { name: 'Try again' })).toBeVisible();
    await expect(libraryRows(page)).toHaveCount(1);
  });

  test('accessible and contained: idle and results', async ({ page }) => {
    await signIn(page, '/search');
    await expect(page.getByRole('region', { name: 'Trending this week' }).locator('article.pcard').first()).toBeVisible();
    await expectAccessibleAndContained(page);
    await field(page).fill('dune');
    await expect(cardTitles(page)).toHaveCount(5);
    await expectAccessibleAndContained(page);
  });
});

test.describe('command palette', () => {
  async function openPalette(page: Page, testInfo: TestInfo) {
    if (isDesktop(testInfo)) {
      await page.keyboard.press('ControlOrMeta+k');
    } else {
      await page.locator('header.phone-bar').getByRole('button', { name: /Account menu/ }).click();
      await page.getByRole('dialog', { name: 'Account' }).getByRole('button', { name: 'Command palette' }).click();
    }
    const palette = page.getByRole('dialog', { name: 'Search or jump to' });
    await expect(palette).toBeVisible();
    return palette;
  }
  const option = (page: Page, name: string | RegExp) => page.getByRole('option', { name });

  test('type, pick a library title, Enter goes to its page', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('sev');
    await expect(page.getByRole('group', { name: 'Your library' }).getByRole('option')).toHaveCount(1);
    await expect(option(page, /^Severance/)).toHaveAttribute('aria-selected', 'true');
    // TMDB titles you don't have, then "Search all results"
    await expect(page.getByRole('group', { name: 'Titles' }).getByRole('option')).toHaveText([/Seven Samurai/]);
    await expect(option(page, /Search all results for “sev”/)).toContainText('2 titles');
    await page.keyboard.press('Enter');
    await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeHidden();
    await expect(page).toHaveURL(/\/tv\/95396$/);
  });

  test('an action: review a mapping', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('review frieren');
    await expect(option(page, /^Review Frieren mapping/)).toHaveAttribute('aria-selected', 'true');
    await expect(option(page, /^Review Frieren mapping/)).toContainText('2 files need a look');
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/mapping\/101$/);
  });

  test('"Search all results" hands off to the search page', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('dune');
    await expect(option(page, /Search all results for “dune”/)).toContainText('6 titles');
    await page.keyboard.press('End');
    await expect(option(page, /Search all results for “dune”/)).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/search\?q=dune$/);
    await expect(tab(page, /^All\s*6$/)).toBeVisible();
  });

  test('before typing: recent searches, your latest titles and every action; arrows wrap', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('momoshtrem:recent-searches', JSON.stringify(['frieren', 'past lives', 'dune'])));
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await expect(page.getByRole('group', { name: 'Recent' }).getByRole('option')).toHaveText([/^frieren/, /^past lives/]);
    await expect(page.getByRole('group', { name: 'Your library' }).getByRole('option')).toHaveCount(4);
    const actions = page.getByRole('group', { name: 'Actions' });
    await expect(actions.getByRole('option', { name: /^Review Frieren mapping/ })).toBeVisible();
    await expect(actions.getByRole('option', { name: /^Find Frieren episodes 29–38/ })).toContainText('10 episodes missing');
    await expect(actions.getByRole('option', { name: /^Go to Activity/ })).toContainText('1 downloading · 1 streaming');
    await expect(actions.getByRole('option', { name: /^Go to Library/ })).toContainText(/\d+ titles/);
    await expect(actions.getByRole('option', { name: /^Infuse setup/ })).toContainText('Copy the WebDAV address');
    await expect(actions.getByRole('option', { name: /^Log out/ })).toBeVisible();

    await expect(option(page, /^frieren/)).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('ArrowUp');
    await expect(option(page, /^Log out/)).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('ArrowDown');
    await expect(option(page, /^frieren/)).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/search\?q=frieren$/);
  });

  test('⌘↵ runs the secondary action: find what a library title lacks', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('one battle');
    await expect(option(page, /^One Battle After Another/)).toHaveAttribute('aria-selected', 'true');
    if (isDesktop(testInfo)) await expect(page.locator('.p-foot')).toContainText('Find a torrent');
    await page.keyboard.press('ControlOrMeta+Enter');
    await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeHidden();
    await expect(page).toHaveURL(/\/library\?sheet=torrent&movie=1054867$/);
    await expect(page.getByRole('dialog', { name: /^Find a source · / })).toBeVisible();
  });

  test('⌘↵ on a TMDB title opens its page with torrent search', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('past lives');
    await expect(page.getByRole('group', { name: 'Titles' }).getByRole('option', { name: /^Past Lives/ })).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('ControlOrMeta+Enter');
    await expect(page).toHaveURL(/\/movie\/666277\?sheet=torrent&movie=666277$/);
    await expect(page.getByRole('dialog', { name: /^Find a source · Past Lives/ })).toBeVisible();
  });

  test('an action: find missing episodes in torrent search', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('find frieren');
    await expect(option(page, /^Find Frieren episodes 29–38/)).toHaveAttribute('aria-selected', 'true');
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/library\?sheet=torrent&show=209867&season=1&se=S01E29$/);
  });

  test('Esc closes it, and it opens empty again', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const palette = await openPalette(page, testInfo);
    await page.keyboard.type('dune');
    await expect(option(page, /Search all results for “dune”/)).toBeVisible();
    if (isDesktop(testInfo)) {
      await page.keyboard.press('Escape');
    } else {
      // Phone: pinned to the top with Cancel; no footer, no pills
      await expect(palette.locator('.p-foot')).toBeHidden();
      await expect(palette.locator('.p-pill').first()).toBeHidden();
      await palette.getByRole('button', { name: 'Cancel' }).click();
    }
    await expect(palette).toBeHidden();
    await openPalette(page, testInfo);
    await expect(page.getByRole('combobox')).toHaveValue('');
  });

  test('accessible and contained', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await openPalette(page, testInfo);
    await page.keyboard.type('sev');
    await expect(option(page, /^Severance/)).toBeVisible();
    await expectAccessibleAndContained(page, '.palette');
  });
});
