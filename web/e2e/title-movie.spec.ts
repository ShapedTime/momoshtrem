import { expect, test, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * The movie page (PLAN F2) against the MSW mocks, one stage per title:
 * Past Lives isn't in the library, One Battle After Another needs a torrent,
 * Interstellar is downloading and Dune: Part Two is ready.
 */

const DUNE_PATH = '/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mkv';
const DUNE_URL = 'https://momoshtrem.local:36911/Movies/Dune-%20Part%20Two%20(2024)/Dune-%20Part%20Two%20(2024).mkv';

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page) {
  // Entry animations (fades, the dock) done first, so axe reads final colours under load too
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

const panel = (page: Page, name: string | RegExp) => page.getByRole('region', { name });

test.describe('movie page', () => {
  test('not in the library: Add, then it needs a torrent', async ({ page }) => {
    await signIn(page, '/movie/666277');
    await expect(page.getByRole('heading', { name: 'Past Lives', level: 1 })).toBeVisible();
    await expect(panel(page, 'Not in your library')).toContainText('Step 1 of 4');
    await expect(page.getByText('Quiet, warm and under two hours.')).toBeVisible();
    await expect(page).toHaveTitle('Past Lives · momoshtrem');
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Add to library' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Added to library' })).toBeVisible();
    await expect(panel(page, 'In your library · no torrent yet')).toContainText('Step 2 of 4');
    await expect(page.getByRole('button', { name: 'Find a torrent' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('Find a torrent opens the search sheet, and adding one starts the download', async ({ page }) => {
    await signIn(page, '/movie/1054867');
    await expect(panel(page, 'In your library · no torrent yet')).toBeVisible();
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Find a torrent' }).click();
    await expect(page).toHaveURL(/[?&]sheet=torrent&movie=1054867$/);
    const search = page.getByRole('dialog', { name: 'Find a source · One Battle After Another 2025 · 2h 42m' });
    await expect(search.getByRole('searchbox', { name: 'Search releases' })).toHaveValue('One Battle After Another 2025');
    const best = search.getByRole('article', { name: 'One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX' });
    await expect(best).toContainText('Best match');
    await best.getByRole('button', { name: 'Add & assign to movie' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Torrent assigned to movie' })).toBeVisible({ timeout: 20_000 });
    await page.keyboard.press('Escape');
    await expect(search).toBeHidden();
    await expect(page).toHaveURL(/\/movie\/1054867$/);
    await expect(panel(page, 'Downloading')).toContainText('Step 3 of 4');
  });

  test('downloading: progress, and it streams now', async ({ page }) => {
    await signIn(page, '/movie/157336');
    const downloading = panel(page, 'Downloading');
    await expect(downloading.getByRole('progressbar', { name: 'Interstellar download' })).toHaveAttribute('aria-valuenow', '42');
    await expect(downloading).toContainText('9 seeders. You can start watching in Infuse now; it streams while it downloads.');
    await expect(downloading).not.toContainText(/left|ETA/);
    await expect(page.getByRole('link', { name: 'View download' })).toHaveAttribute('href', '/torrents');
    // The torrent and subtitles only move up once it's ready
    await expect(page.getByRole('heading', { name: 'Current torrent' })).toBeHidden();
    await expectAccessibleAndContained(page);
  });

  test('ready: copy the path and open How to play', async ({ page, context }, testInfo) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await signIn(page, '/movie/693134');
    const ready = panel(page, 'Ready in Infuse');
    await expect(ready).toContainText('2160p · 18.4 GB · Dolby Vision · Atmos');
    await expect(ready.getByLabel('WebDAV path')).toHaveText(DUNE_PATH);
    await expect(page.getByRole('heading', { name: 'Current torrent' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'On Trakt' })).toBeVisible();
    await expectAccessibleAndContained(page);

    // Phone: the dock's copy icon; desktop: the path's Copy button. Both copy the full URL.
    const copy =
      testInfo.project.name === 'mobile' ? page.getByRole('button', { name: 'Copy WebDAV path' }) : ready.getByRole('button', { name: 'Copy' });
    await copy.click();
    await expect(page.getByRole('status').filter({ hasText: 'Copied' })).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(DUNE_URL);

    await page.getByRole('button', { name: testInfo.project.name === 'mobile' ? 'Play in Infuse' : 'How to play in Infuse' }).click();
    await expect(page).toHaveURL(/[?&]sheet=infuse-setup&path=.+&url=https%3A/);
    await expect(page.getByRole('dialog', { name: 'Play it in Infuse' })).toBeVisible();
  });

  test('How to play opens from the URL with this title’s path, and copies the server’s URL', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await signIn(page, '/movie/693134');
    await page.goto(`/movie/693134?sheet=infuse-setup&path=${encodeURIComponent(DUNE_PATH)}&url=${encodeURIComponent(DUNE_URL)}`);
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet.getByLabel('WebDAV address')).toBeVisible();
    await expect(sheet).toContainText('Movies › Dune- Part Two (2024)');
    await expect(sheet.getByLabel('WebDAV path of this title')).toHaveText(DUNE_PATH);
    await expectNoA11yViolations(page);
    await sheet.getByRole('button', { name: 'Copy' }).last().click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(DUNE_URL);
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/movie\/693134$/);
  });

  test('ready: search OpenSubtitles and get a subtitle', async ({ page }) => {
    await signIn(page, '/movie/693134');
    const subtitles = page.getByRole('region', { name: 'Subtitles' });
    const yours = subtitles.getByRole('list', { name: 'Your subtitles' });
    await expect(yours.getByRole('listitem')).toHaveCount(2);
    // The torrent's English subtitles stay with the file
    await expect(yours.getByRole('button', { name: 'Remove English subtitles' })).toHaveCount(0);
    await expect(yours.getByRole('button', { name: 'Remove Russian subtitles' })).toBeVisible();

    await expect(subtitles.getByRole('combobox', { name: 'Subtitle language' })).toHaveValue('az');
    await subtitles.getByRole('button', { name: 'Search' }).click();
    const results = subtitles.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' });
    await expect(results.getByRole('listitem')).toHaveCount(3);
    await expect(results.getByRole('listitem').first()).toContainText('Exact release');
    await expectAccessibleAndContained(page);

    await results.getByRole('listitem').first().getByRole('button', { name: /^Get/ }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Azerbaijani subtitles added' })).toBeVisible();
    await expect(yours.getByRole('listitem')).toHaveCount(3);
    await expect(yours).toContainText('Azerbaijani');
    await expect(results.getByRole('listitem').first()).toContainText('Added');
  });

  test('Remove asks first, and Cancel keeps everything', async ({ page }) => {
    await signIn(page, '/movie/693134');
    const more = page.getByRole('button', { name: 'More actions for Dune: Part Two' });
    await more.click();
    await page.getByText('Remove from library…').click();
    const confirm = page.getByRole('alertdialog', { name: 'Remove Dune: Part Two from your library?' });
    await expect(confirm).toContainText('Infuse loses it right away and its torrent is unassigned.');
    await expectNoA11yViolations(page);
    await confirm.getByRole('button', { name: 'Cancel' }).click();
    await expect(confirm).toBeHidden();
    await expect(panel(page, 'Ready in Infuse')).toBeVisible();

    // The page is still usable after the menu and dialog closed
    await more.click();
    await page.getByText('Remove from library…').click();
    await confirm.getByRole('button', { name: 'Remove movie' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Removed Dune: Part Two' })).toBeVisible();
    await expect(panel(page, 'Not in your library')).toBeVisible();
  });

  test('an unknown id is a 404 inside the shell', async ({ page }) => {
    await signIn(page, '/movie/999999');
    await expect(page.getByRole('heading', { name: 'Nothing here but the cat' })).toBeVisible();
    await page.goto('/movie/not-a-number');
    await expect(page.getByRole('heading', { name: 'Nothing here but the cat' })).toBeVisible();
  });
});
