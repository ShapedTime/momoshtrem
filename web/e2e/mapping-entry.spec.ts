import { expect, test, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Ways into the mapping review (#5) against the MSW mocks: adding a torrent
 * from the legacy search starts a mapping job (a show's goes to its review;
 * a movie's goes to the review to pick a cut, or applies on the spot with one
 * plausible file, as title-movie.spec.ts checks), and Activity lists the open jobs.
 */

const FRIEREN = 'Frieren: Beyond Journey’s End';
const OBAA = 'One Battle After Another';

/** Entry animations done, so axe reads final colours; endless ones (spinners, the eyes) don't count. */
async function settle(page: Page) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
}

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page) {
  await settle(page);
  await expectNoA11yViolations(page);
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

const panel = (page: Page, name: string | RegExp) => page.getByRole('region', { name });
const toast = (page: Page, text: string) => page.getByRole('status').filter({ hasText: text });

test.describe('mapping entry points', () => {
  test('a show: adding a torrent from an episode’s search goes to its mapping, with the episode as a hint', async ({ page }) => {
    // Held at classifying, so the progress shows however long the route takes to compile
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'job-hold:classifying'));
    await signIn(page, '/tv/209867');
    await page.getByRole('button', { name: 'Find a torrent for S01E30' }).click();
    const search = page.getByRole('dialog', { name: `Search: ${FRIEREN} S01E30` });
    const release = 'Frieren.Beyond.Journeys.End.S01E30.1080p.BluRay.x264-SPARKS';
    const assign = page.waitForRequest((request) => request.method() === 'POST' && /\/api\/shows\/1\/assign-torrent$/.test(request.url()));
    await search.getByRole('button', { name: `Add ${release}` }).click();
    expect((await assign).postDataJSON()).toMatchObject({ hint: { season: 1, episode: 30 }, indexer: 'IPTorrents' });

    await expect(page).toHaveURL(/\/mapping\/\d+$/);
    await expect(search).toBeHidden();
    // The job's progress, then the review
    await expect(page.getByText('Mapping files to episodes')).toBeVisible();
    await expect(page.locator('.map-steps li').nth(1)).toHaveAttribute('aria-current', 'step');
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await expect(page.getByText('Done. Here’s the plan.')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Check the episodes', level: 1 })).toBeVisible();
    await expect(page.getByText(release)).toBeVisible();
    await expect(page.getByRole('button', { name: /^Needs review/ })).toBeVisible();
    await expect(page.locator('.map-summary')).toContainText('2 need review');
    await expectAccessibleAndContained(page);
  });

  test('a movie: two cuts go to the review, where you use one, and it’s ready in Infuse', async ({ page }) => {
    await signIn(page, '/movie/1054867');
    await page.getByRole('button', { name: 'Find a torrent' }).click();
    const search = page.getByRole('dialog', { name: `Search Torrents: ${OBAA} (2025)` });
    const release = 'One.Battle.After.Another.2025.1080p.BluRay.x264-SPARKS';
    await search.getByRole('button', { name: `Add ${release}` }).click();

    await expect(page).toHaveURL(/\/mapping\/\d+$/, { timeout: 15_000 });
    await expect(page.getByRole('heading', { name: 'Check the files', level: 1 })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Re-check with AI' })).toHaveCount(0);
    await expect(page.locator('.map-foot-msg')).toHaveText('Pick the file to use');
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: `Use this file: ${release}.mkv` }).click();
    await expect(page.getByRole('button', { name: `Using this file: ${release}.mkv. Tap to undo` })).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('.mrow').filter({ hasText: `${release}.Extended.Cut.mkv` })).toContainText(/Not used\s*· not in Infuse/);
    // Undo brings both back
    await page.getByRole('button', { name: `Using this file: ${release}.mkv. Tap to undo` }).click();
    await expect(page.getByRole('button', { name: /^Use this file:/ })).toHaveCount(2);
    await page.getByRole('button', { name: `Use this file: ${release}.mkv` }).click();
    await expect(page.locator('.map-foot-msg')).toHaveText('Everything has a place');

    await page.getByRole('button', { name: 'Apply 1 mapping' }).click();
    await expect(page.getByRole('heading', { name: 'Ready in Infuse.', level: 1 })).toBeVisible();
    const done = page.getByRole('list', { name: 'Mapped to Infuse' });
    await expect(done.getByRole('listitem').first()).toContainText(`${release}.mkv`);
    await expect(done).not.toContainText('Season');
    await expect(toast(page, `${OBAA} is ready`)).toBeVisible();
    await expectAccessibleAndContained(page);

    await page.getByRole('link', { name: `Back to ${OBAA}` }).click();
    await expect(panel(page, 'Downloading')).toContainText('Step 3 of 4');
  });

  test('Activity lists the open mappings, each linking to its review', async ({ page }) => {
    await signIn(page, '/torrents');
    const jobs = page.getByRole('region', { name: 'Episode mappings to check' });
    await expect(jobs.getByRole('listitem')).toHaveCount(3);
    await expect(jobs.getByRole('listitem').first()).toContainText('Doctor Who: 3 files to check');
    await expect(jobs).toContainText('Frieren: 2 files to check');
    await expect(jobs).toContainText('One Piece: 1 file to check');
    // The legacy page around it is rebuilt in #7 (its torrent cards already scroll sideways at 320 px): check the banner
    await settle(page);
    await expectNoA11yViolations(page, { include: '.banner' });
    for (const width of [page.viewportSize()?.width ?? 390, 320]) {
      await page.setViewportSize({ width, height: 800 });
      const box = await jobs.boundingBox();
      expect(box && box.x >= 0 && box.x + box.width <= width).toBe(true);
    }
    await jobs.getByRole('link', { name: 'Review mapping for Frieren' }).click();
    await expect(page).toHaveURL(/\/mapping\/101$/);
    await expect(page.getByRole('heading', { name: 'Check the episodes', level: 1 })).toBeVisible();
  });
});
