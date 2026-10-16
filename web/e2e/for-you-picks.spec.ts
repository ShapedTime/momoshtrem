import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Noir's picks on For You (#155, PLAN F5) against the recommendations mock
 * (#152): last night's run (the design's shelves and Tonight's pick, with
 * Pluribus liked), feedback with Undo, and the `recs-*` and `ai-off`
 * scenarios (src/mocks/scenario.ts). The mock's state lives in the page, so
 * a reload starts over.
 */

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
const region = (page: Page, name: string) => page.getByRole('region', { name, exact: true });
const hero = (page: Page) => page.locator('section.home-hero');
const body = (page: Page) => page.locator('.home-body');
const card = (page: Page, shelf: string, title: string) => region(page, shelf).locator('article.pcard').filter({ hasText: title });
const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';
const SEVERANCE = 'Because you loved Severance';

/** The page's sections in order, by their headings (h2s under the hero). */
const sectionOrder = (page: Page) => body(page).locator('section h2').filter({ visible: true }).allTextContents();

test.describe('For You: Noir’s picks', () => {
  test('Tonight’s pick, the shelves in the design’s order with reasons and feedback, the Ask entries and the footer', async ({ page }, testInfo) => {
    await signIn(page, '/');
    await expect(hero(page).getByText('Tonight’s pick')).toBeVisible();
    await expect(hero(page).getByRole('heading', { name: 'Past Lives', level: 2 })).toBeVisible();
    await expect(hero(page).getByText(/^You gave Perfect Days a 9/)).toBeVisible();
    await expect(hero(page).getByRole('button', { name: 'Feedback on Past Lives' })).toBeVisible();

    const severance = region(page, SEVERANCE);
    await expect(severance.getByText('Workplace dread, slow-burn mysteries, impeccable design')).toBeVisible();
    await expect(severance.locator('article.pcard')).toHaveCount(7);
    // A reason on every card, the library's state on the poster, and feedback without hover
    await expect(card(page, SEVERANCE, 'Silo').locator('.reason')).toHaveText('Same sealed-world mystery, unspools one clue an episode');
    await expect(card(page, SEVERANCE, 'The Studio')).toContainText('Downloading');
    const silo = card(page, SEVERANCE, 'Silo').getByRole('group', { name: 'Feedback on Silo' });
    for (const name of ['More like this', 'Less like this', 'More feedback']) await expect(silo.getByRole('button', { name })).toBeVisible();
    // Pluribus was liked earlier
    await expect(card(page, SEVERANCE, 'Pluribus').getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');

    // Coming soon: last, with its dates on the posters
    await expect(card(page, 'Coming soon', 'The Bear')).toContainText('Summer');
    await expect(card(page, 'Coming soon', 'Frieren: Beyond')).toContainText('Jan 16');

    const order = await sectionOrder(page);
    const expected = ['Up next', 'New episodes', SEVERANCE, 'New this month for you', 'Anime you’ll like', 'Hidden gems', 'Your Trakt watchlist', 'Coming soon'];
    expect(order.filter((name) => expected.includes(name))).toEqual(expected);
    await expect(region(page, 'Trending this week')).toHaveCount(0);

    // Ask: an entry under the hero on phone, the band after the first shelf on desktop
    if (phone(testInfo)) {
      await expect(body(page).getByRole('link', { name: /^Ask momoshtrem…/ })).toHaveAttribute('href', '/ask');
      await expect(region(page, 'Ask momoshtrem')).toHaveCount(0);
    } else {
      const band = region(page, 'Ask momoshtrem');
      await expect(band.getByRole('link', { name: /^Ask momoshtrem…/ })).toHaveAttribute('href', '/ask');
      const chips = band.getByRole('list', { name: 'Try asking' }).getByRole('link');
      await expect(chips).toHaveCount(3);
      await expect(chips.nth(1)).toHaveAttribute('href', '/ask?q=a%2090-minute%20thriller%20for%20tonight');
      // After the first shelf, before the second
      const top = (locator: ReturnType<Page['locator']>) => locator.evaluate((el) => el.getBoundingClientRect().top);
      expect(await top(band)).toBeGreaterThan(await top(severance));
      expect(await top(band)).toBeLessThan(await top(region(page, 'New this month for you')));
    }

    const foot = body(page).locator('.picks-foot');
    await expect(foot).toContainText(/Picks written by Noir at \d\d:\d\d from your Trakt history\./);
    await expect(foot.getByRole('link', { name: 'Change schedule' })).toHaveAttribute('href', '/settings?section=ai');
    await expectAccessibleAndContained(page);
  });

  test('dislike: the card dims at once with “Got it, fewer like this”, and Undo brings it back', async ({ page }) => {
    await signIn(page, '/');
    const andor = card(page, SEVERANCE, 'Andor');
    await expect(andor).toBeVisible();
    await expect(andor).not.toHaveClass(/is-dismissed/);
    await andor.getByRole('button', { name: 'Less like this' }).click();
    await expect(andor).toHaveClass(/is-dismissed/);
    await expect(andor.getByRole('button', { name: 'Less like this' })).toHaveAttribute('aria-pressed', 'true');
    const toast = page.getByRole('status').filter({ hasText: 'Got it, fewer like this' });
    await expect(toast).toContainText('Andor won’t be picked again.');
    await toast.getByRole('button', { name: 'Undo' }).click();
    await expect(andor).not.toHaveClass(/is-dismissed/);
    await expect(andor.getByRole('button', { name: 'Less like this' })).toHaveAttribute('aria-pressed', 'false');
    await expect(toast).toBeHidden();
  });

  test('like: thumbs up toggles with aria-pressed and never dims; the title agrees on every shelf', async ({ page }) => {
    await signIn(page, '/');
    const like = card(page, SEVERANCE, 'Silo').getByRole('button', { name: 'More like this' });
    await expect(like).toHaveAttribute('aria-pressed', 'false');
    await like.click();
    await expect(like).toHaveAttribute('aria-pressed', 'true');
    await expect(card(page, SEVERANCE, 'Silo')).not.toHaveClass(/is-dismissed/);
    await expect(page.getByText('Got it, fewer like this')).toHaveCount(0);

    // Mushishi is on two shelves: liking it on one marks both
    await card(page, 'Anime you’ll like', 'Mushishi').getByRole('button', { name: 'More like this' }).click();
    await expect(card(page, 'Hidden gems', 'Mushishi').getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');

    await like.click();
    await expect(like).toHaveAttribute('aria-pressed', 'false');
  });

  test('the “…” sheet: Not interested dims the card; on the hero, Already seen dims Tonight’s pick until Undo', async ({ page }) => {
    await signIn(page, '/');
    const mythic = card(page, SEVERANCE, 'Mythic Quest');
    await mythic.getByRole('button', { name: 'More feedback' }).click();
    const sheet = page.getByRole('dialog', { name: 'Mythic Quest' });
    await expect(sheet.getByText('Tune your picks')).toBeVisible();
    await expect(sheet.getByRole('button', { name: /Not interested/ })).toContainText('Hides it everywhere');
    await expect(sheet.getByRole('button', { name: /Already seen/ })).toContainText('Counts as watched');
    await expect(sheet.getByRole('button', { name: 'Add to library' })).toBeVisible();
    await sheet.getByRole('button', { name: /Not interested/ }).click();
    await expect(sheet).toBeHidden();
    await expect(mythic).toHaveClass(/is-dismissed/);
    await expect(page.getByRole('status').filter({ hasText: 'Mythic Quest is hidden from your picks and Ask.' })).toBeVisible();

    await hero(page).getByRole('button', { name: 'Feedback on Past Lives' }).click();
    const heroSheet = page.getByRole('dialog', { name: 'Past Lives' });
    await heroSheet.getByRole('button', { name: /Already seen/ }).click();
    await expect(hero(page)).toHaveClass(/is-dismissed/);
    const toast = page.getByRole('status').filter({ hasText: 'Past Lives counts as watched.' });
    await toast.getByRole('button', { name: 'Undo' }).click();
    await expect(hero(page)).not.toHaveClass(/is-dismissed/);
  });

  test('refreshing: the glass banner with the step and shelves done, one shelf as skeletons, the old picks stay', async ({ page }, testInfo) => {
    await scenario(page, 'recs-refreshing');
    await signIn(page, '/');
    const banner = body(page).locator('.refresh-banner');
    await expect(banner).toContainText('Refreshing your picks');
    await expect(banner).toContainText('Writing your shelves… · 2 of 5 shelves done');
    if (phone(testInfo)) await expect(banner.getByText('You can keep browsing')).toBeHidden();
    else await expect(banner.getByText('You can keep browsing')).toBeVisible();
    // The first shelf after the Ask band is being rewritten; the rest stay
    const rewriting = region(page, 'New this month for you');
    await expect(rewriting.getByText('Refreshing…')).toBeVisible();
    await expect(rewriting.locator('article.pcard')).toHaveCount(0);
    await expect(rewriting.locator('.skel').first()).toBeVisible();
    await expect(region(page, SEVERANCE).locator('article.pcard').first()).toBeVisible();
    await expect(hero(page).getByText('Tonight’s pick')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('a failed refresh keeps the shelves, with a quiet note', async ({ page }) => {
    await scenario(page, 'recs-failed');
    await signIn(page, '/');
    await expect(body(page).getByText('Couldn’t refresh your picks.')).toBeVisible();
    await expect(body(page).locator('.picks-note')).toContainText('Your last picks stay');
    await expect(region(page, SEVERANCE).locator('article.pcard').first()).toBeVisible();
    await expect(body(page).locator('.refresh-banner')).toHaveCount(0);
  });

  test('no picks yet (recs-none): For You’s own hero and trending, and Ask still there', async ({ page }, testInfo) => {
    await scenario(page, 'recs-none');
    await signIn(page, '/');
    await expect(hero(page).getByText(/Ready in Infuse|From your watchlist|Trending this week/).first()).toBeVisible();
    await expect(hero(page).getByRole('button', { name: /^Feedback on/ })).toHaveCount(0);
    await expect(region(page, 'Trending this week').locator('article.pcard').first()).toBeVisible();
    await expect(region(page, SEVERANCE)).toHaveCount(0);
    await expect(body(page).locator('.picks-foot')).toHaveCount(0);
    const ask = phone(testInfo) ? body(page).getByRole('link', { name: /^Ask momoshtrem…/ }) : region(page, 'Ask momoshtrem');
    await expect(ask).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('the first run after Trakt’s import (recs-first-run): the first sync adds “Writing your shelves”', async ({ page }) => {
    await scenario(page, 'recs-first-run');
    await signIn(page, '/');
    const sync = page.getByRole('region', { name: 'First sync with Trakt' });
    await expect(sync.getByText(/Building your picks…$/)).toBeVisible();
    const steps = sync.getByRole('listitem');
    await expect(steps).toHaveCount(6);
    await expect(steps.nth(4)).toContainText('Fetched artwork and episode names');
    await expect(steps.nth(5)).toContainText(/Writing your shelves\s*2 of 5/);
    await expect(steps.nth(5)).toHaveAttribute('aria-current', 'step');
    await expect(region(page, 'Your picks').getByText('Writing…')).toBeVisible();
    await expect(region(page, 'Up next').locator('article.lcard').first()).toBeVisible();
    await expect(hero(page)).toHaveCount(0);
    await expectAccessibleAndContained(page);
  });

  test('picks switched off (recs-off): trending instead of shelves, and the Ask entries stay', async ({ page }, testInfo) => {
    await scenario(page, 'recs-off');
    await signIn(page, '/');
    await expect(region(page, 'Trending this week').locator('article.pcard').first()).toBeVisible();
    await expect(region(page, SEVERANCE)).toHaveCount(0);
    await expect(hero(page).getByText('Tonight’s pick')).toHaveCount(0);
    const ask = phone(testInfo) ? body(page).getByRole('link', { name: /^Ask momoshtrem…/ }) : region(page, 'Ask momoshtrem');
    await expect(ask).toBeVisible();
  });

  test('no key (ai-off): no picks and no Ask entries', async ({ page }) => {
    await scenario(page, 'ai-off');
    await signIn(page, '/');
    await expect(region(page, 'Trending this week').locator('article.pcard').first()).toBeVisible();
    await expect(region(page, SEVERANCE)).toHaveCount(0);
    await expect(body(page).getByRole('link', { name: /^Ask momoshtrem…/ })).toHaveCount(0);
    await expect(region(page, 'Ask momoshtrem')).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
  });
});
