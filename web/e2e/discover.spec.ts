import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Discover (PLAN F6, #100) against the MSW fixture catalogue
 * (src/mocks/fixtures/discover.ts): 60 titles, 32 movies and 28 shows, in
 * pages of 20.
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

/** The page's query, in any key order (nuqs keeps a key where it already was). */
async function expectQuery(page: Page, query: string) {
  const sorted = (search: string) => [...new URLSearchParams(search)].map(([key, value]) => `${key}=${value}`).sort().join('&');
  await expect.poll(() => {
    const url = new URL(page.url());
    return `${url.pathname}?${sorted(url.search)}`;
  }).toBe(`/discover?${sorted(query)}`);
}

const count = (page: Page) => page.locator('#disc-count');
const cards = (page: Page) => page.locator('.disc-grid article.pcard');
const titles = (page: Page) => page.locator('.disc-grid article.pcard .meta .title');
const sidebar = (page: Page) => page.getByRole('complementary', { name: 'Filters' });
const quick = (page: Page, name: string) => page.getByRole('group', { name: 'Quick filters' }).getByRole('button', { name, exact: true });
const mood = (page: Page, name: string) => page.getByRole('group', { name: 'Moods' }).getByRole('button', { name, exact: true });
const activeChip = (page: Page, label: string) => page.getByRole('button', { name: `Remove filter: ${label}` });

test.describe('discover', () => {
  test('filters update the URL and the results', async ({ page }, testInfo) => {
    await signIn(page, '/discover');
    await expect(page.getByRole('heading', { name: 'Discover', level: 1 })).toBeVisible();
    await expect(page).toHaveTitle('Discover · momoshtrem');
    await expect(count(page)).toHaveText('60 titles');
    await expect(page.getByText('From TMDB, filtered live', { exact: true })).toBeVisible();
    await expect(cards(page)).toHaveCount(20);
    // Library pills on TMDB cards, and the type as the sub-line
    await expect(cards(page).filter({ hasText: 'Dune: Part Two' })).toContainText('Ready');
    await expect(cards(page).filter({ hasText: 'Dune: Part Two' })).toContainText('2024 · Movie');
    await expect(cards(page).filter({ hasText: 'Pluribus' })).toContainText('2025 · Series');

    if (isDesktop(testInfo)) {
      await sidebar(page).getByRole('button', { name: 'Shows', exact: true }).click();
      await expectQuery(page, 'type=tv');
      await expect(count(page)).toHaveText('28 shows');
      await sidebar(page).getByRole('button', { name: 'Comedy', exact: true }).click();
      await expectQuery(page, 'type=tv&genres=comedy');
      await expect(count(page)).toHaveText('9 shows');
      await sidebar(page).getByRole('group', { name: 'Minimum rating' }).getByRole('button', { name: '8+' }).click();
    } else {
      await quick(page, 'Shows').click();
      await expectQuery(page, 'type=tv');
      await expect(count(page)).toHaveText('28 shows');
      await page.goto('/discover?type=tv&genres=comedy');
      await expect(count(page)).toHaveText('9 shows');
      await expect(quick(page, 'Comedy')).toHaveAttribute('aria-pressed', 'true');
      await quick(page, 'Rated 8+').click();
    }
    await expectQuery(page, 'type=tv&genres=comedy&rating=8');
    await expect(count(page)).toHaveText('8 shows');
    await expect(page.getByText('From TMDB, filtered live · 2 filters')).toBeVisible();
    await expect(activeChip(page, 'Comedy')).toBeVisible();
    await expect(activeChip(page, 'Rated 8+')).toBeVisible();

    await page.getByLabel('Sort by').selectOption('title');
    await expectQuery(page, 'type=tv&genres=comedy&rating=8&sort=title');
    await expect(titles(page).first()).toHaveText('Bluey');

    await activeChip(page, 'Comedy').click();
    await expectQuery(page, 'type=tv&rating=8&sort=title');
    await expectAccessibleAndContained(page);
  });

  test('a deep link restores the filters', async ({ page }, testInfo) => {
    await signIn(page, '/discover?type=movie&genres=sci-fi&to=2019&status=ended&sort=rating');
    await expect(count(page)).toHaveText('8 movies');
    await expect(titles(page).first()).toHaveText('Interstellar');
    await expect(page.getByLabel('Sort by')).toHaveValue('rating');
    await expect(activeChip(page, 'Sci-Fi')).toBeVisible();
    await expect(activeChip(page, 'Until 2019')).toBeVisible();
    // Status is for shows only, so Movies drops it
    await expect(page.getByRole('button', { name: /Remove filter: Ended/ })).toHaveCount(0);
    if (isDesktop(testInfo)) {
      const side = sidebar(page);
      await expect(side.getByRole('button', { name: 'Movies', exact: true })).toHaveAttribute('aria-pressed', 'true');
      await expect(side.getByRole('button', { name: 'Sci-Fi', exact: true })).toHaveAttribute('aria-pressed', 'true');
      await expect(side.getByLabel('To year')).toHaveValue('2019');
      await expect(side.getByText('For shows only.')).toBeVisible();
      await expect(side.getByRole('group', { name: 'Show status' }).getByRole('button', { name: 'Ended' })).toBeDisabled();
    } else {
      await expect(quick(page, 'Movies')).toHaveAttribute('aria-pressed', 'true');
      await expect(quick(page, 'Sci-Fi')).toHaveAttribute('aria-pressed', 'true');
      await expect(page.getByRole('button', { name: /^Filters/ })).toContainText('3');
    }
  });

  test('a mood fills in its preset and stays selected only while it matches', async ({ page }, testInfo) => {
    await signIn(page, '/discover');
    await mood(page, 'Laugh-out-loud').click();
    await expectQuery(page, 'genres=comedy&rating=7&mood=laugh');
    await expect(mood(page, 'Laugh-out-loud')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByText('Laugh-out-loud picks from TMDB · 2 filters')).toBeVisible();

    // Editing a filter keeps it, but it's no longer the mood
    if (isDesktop(testInfo)) await sidebar(page).getByRole('group', { name: 'Minimum rating' }).getByRole('button', { name: '8+' }).click();
    else await quick(page, 'Rated 8+').click();
    await expectQuery(page, 'genres=comedy&rating=8');
    await expect(mood(page, 'Laugh-out-loud')).toHaveAttribute('aria-pressed', 'false');

    // Another mood replaces the filters and keeps the type
    await page.goto('/discover?type=tv&genres=war');
    await mood(page, 'Epic nights').click();
    await expectQuery(page, 'type=tv&genres=adventure&rating=7&runtime=long&mood=epic');
    // Tapping the selected mood clears it
    await mood(page, 'Epic nights').click();
    await expectQuery(page, 'type=tv');
  });

  test('infinite scroll loads the next pages', async ({ page }) => {
    await signIn(page, '/discover');
    await expect(cards(page)).toHaveCount(20);
    const more = page.getByText('Loading more…');
    await more.scrollIntoViewIfNeeded();
    await expect(cards(page)).toHaveCount(40);
    await more.scrollIntoViewIfNeeded();
    await expect(cards(page)).toHaveCount(60);
    await expect(more).toHaveCount(0);
  });

  test('Clear all keeps the type and the sort', async ({ page }) => {
    await signIn(page, '/discover?type=tv&genres=comedy&rating=8&sort=title');
    await expect(count(page)).toHaveText('8 shows');
    await page.getByRole('button', { name: 'Clear all', exact: true }).click();
    await expectQuery(page, 'type=tv&sort=title');
    await expect(count(page)).toHaveText('28 shows');
  });

  test('the empty state gives each filter back, best first', async ({ page }) => {
    await signIn(page, '/discover?type=tv&anime=1&genres=comedy&from=2020&rating=8&status=ended');
    await expect(count(page)).toHaveText('0 shows');
    await expect(page.getByRole('heading', { name: 'No shows match all 5 filters' })).toBeVisible();
    const fixes = page.getByRole('group', { name: 'Remove a filter' }).getByRole('button');
    await expect(fixes).toHaveText([/Ended\s*2 shows/, /Comedy\s*1 show/, /Anime\s*still none/, /2020s\s*still none/, /Rated 8\+\s*still none/]);
    await expect(fixes.first()).toHaveAccessibleName('Remove Ended: 2 shows');
    await expect(fixes.nth(2)).toHaveAccessibleName('Remove Anime: still no shows');
    await expectAccessibleAndContained(page);

    await fixes.first().click();
    await expectQuery(page, 'type=tv&genres=comedy&from=2020&rating=8&anime=1');
    await expect(titles(page)).toHaveText(['Dan Da Dan', 'SPY x FAMILY']);

    await page.goto('/discover?type=tv&genres=romance');
    await page.getByRole('button', { name: 'Clear all filters' }).click();
    await expectQuery(page, 'type=tv');
  });

  test('a failed load says what happened and tries again', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'discover-error'));
    await signIn(page, '/discover');
    const banner = page.getByRole('alert').filter({ hasText: 'Couldn’t load Discover' });
    await expect(banner).toContainText('TMDB didn’t answer.');
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await banner.getByRole('button', { name: 'Try again' }).click();
    await expect(count(page)).toHaveText('60 titles');
  });

  test('the old browse pages redirect to Discover', async ({ page }) => {
    await signIn(page, '/discover');
    await page.goto('/movies');
    await expectQuery(page, 'type=movie');
    await expect(count(page)).toHaveText('32 movies');
    await page.goto('/tv-shows');
    await expectQuery(page, 'type=tv');
  });
});

test.describe('discover on desktop', () => {
  test.beforeEach(({}, testInfo) => test.skip(!isDesktop(testInfo), 'Copy link and the sidebar are desktop affordances'));

  test('Copy link copies this view', async ({ page }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await signIn(page, '/discover?type=tv&genres=comedy');
    await page.getByRole('button', { name: 'Copy link' }).click();
    await expect(page.getByText('Link copied')).toBeVisible();
    await expect(page.getByText('It opens Discover with these filters.')).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(/\/discover\?type=tv&genres=comedy$/);
  });

  test('the sidebar covers years, runtime, language, country and status', async ({ page }) => {
    await signIn(page, '/discover');
    const side = sidebar(page);
    await side.getByRole('button', { name: '2010s' }).click();
    await expectQuery(page, 'from=2010&to=2019');
    await side.getByRole('button', { name: '2010s' }).click();
    await expectQuery(page, '');
    await side.getByLabel('From year').selectOption('2020');
    await side.getByLabel('To year').selectOption('2015');
    // Swapped into order
    await expectQuery(page, 'from=2015&to=2020');
    await side.getByRole('group', { name: 'Runtime' }).getByRole('button', { name: 'Long' }).click();
    await side.getByRole('button', { name: 'Japanese' }).click();
    await side.getByLabel('Country of origin').selectOption('JP');
    await expectQuery(page, 'from=2015&to=2020&runtime=long&lang=ja&country=JP');
    await page.goto('/discover?type=tv');
    await side.getByRole('switch', { name: 'Anime' }).check();
    await side.getByRole('group', { name: 'Show status' }).getByRole('button', { name: 'Airing' }).click();
    await expectQuery(page, 'type=tv&anime=1&status=returning');
    await expect(titles(page)).toHaveText(['One Piece', 'Dan Da Dan', 'Frieren: Beyond Journey’s End', 'SPY x FAMILY']);
  });
});

test.describe('discover on phone', () => {
  test.beforeEach(({}, testInfo) => test.skip(isDesktop(testInfo), 'The filter sheet is the phone layout'));

  test('the filter sheet stages changes until you apply them', async ({ page }) => {
    await signIn(page, '/discover');
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    const sheet = page.getByRole('dialog', { name: 'Filters' });
    await expect(sheet).toBeVisible();
    await expectQuery(page, 'filters=1');
    const apply = sheet.locator('.disc-apply');
    await expect(apply).toHaveText('Show 60 titles');

    await sheet.getByRole('button', { name: 'Shows', exact: true }).click();
    await sheet.getByRole('button', { name: 'Comedy', exact: true }).click();
    await sheet.getByRole('button', { name: 'Title A–Z' }).click();
    await expect(apply).toHaveText('Show 9 shows');
    // Staged: the page hasn't changed yet
    await expectQuery(page, 'filters=1');
    await expectNoA11yViolations(page);

    await apply.click();
    await expect(sheet).toBeHidden();
    await expectQuery(page, 'type=tv&genres=comedy&sort=title');
    await expect(count(page)).toHaveText('9 shows');
    await expect(page.getByRole('button', { name: /^Filters/ })).toContainText('2');

    // Nothing matches: the button says so and waits
    await page.getByRole('button', { name: /^Filters/ }).click();
    await sheet.getByRole('button', { name: 'Romance', exact: true }).click();
    await expect(apply).toHaveText('No shows match');
    await expect(apply).toBeDisabled();
    // Reset keeps the sort; closing keeps what was applied
    await sheet.getByRole('button', { name: 'Reset' }).click();
    await expect(apply).toHaveText('Show 60 titles');
    await expect(sheet.getByRole('button', { name: 'Title A–Z' })).toHaveAttribute('aria-pressed', 'true');
    await sheet.getByRole('button', { name: 'Close filters' }).click();
    await expect(sheet).toBeHidden();
    await expectQuery(page, 'type=tv&genres=comedy&sort=title');
  });

  test('quick chips apply at once and selected genres come off', async ({ page }) => {
    await signIn(page, '/discover?genres=comedy,family');
    await quick(page, 'Anime').click();
    await expectQuery(page, 'genres=comedy,family&anime=1');
    // Commas stay readable, and defaults stay out
    await expect(page).toHaveURL(/\/discover\?genres=comedy,family&anime=1$/);
    await quick(page, '2020s').click();
    await expectQuery(page, 'genres=comedy,family&from=2020&anime=1');
    await quick(page, 'Family').click();
    await expectQuery(page, 'genres=comedy&from=2020&anime=1');
    await expect(page.getByRole('button', { name: /^Filters/ })).toContainText('3');
    await expectAccessibleAndContained(page);
  });
});
