import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Download & keep on title pages (#206) against the MSW mocks
 * (fixtures/downloads.ts): Interstellar is kept and downloading, One Battle
 * After Another is kept with no file yet, Bluey season 3 is kept until
 * removed, and Frieren S01E27–E28 are smart picks. Dune: Part Two is ready
 * and nothing keeps it.
 */

const DUNE = 'Dune: Part Two';
const FRIEREN = 'Frieren: Beyond Journey’s End';

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
const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';

/** The ⋯ menu's item: a menu item on desktop, a row of the "More" sheet on phone. */
async function moreAction(page: Page, testInfo: TestInfo, title: string, label: string) {
  await page.getByRole('button', { name: `More actions for ${title}` }).click();
  if (phone(testInfo)) await page.getByRole('dialog', { name: 'More' }).getByRole('button', { name: label }).click();
  else await page.getByRole('menuitem', { name: label }).click();
}

test.describe('Download & keep', () => {
  test('a ready movie: keep it for 7 days, change how long, then stop keeping it', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    await expect(panel(page, 'Ready in Infuse')).toBeVisible();
    // The panel's second action on desktop; the ⋯ menu on phone, where the dock has one button
    if (phone(testInfo)) await moreAction(page, testInfo, DUNE, 'Download & keep…');
    else await panel(page, 'Ready in Infuse').getByRole('button', { name: 'Download & keep' }).click();

    await expect(page).toHaveURL(/[?&]sheet=keep&movie=693134/);
    const sheet = page.getByRole('dialog', { name: 'Download & keep' });
    await expect(sheet).toContainText(DUNE);
    await expect(sheet).toContainText(/Needs 18\.4 GB · [\d.]+ GB left for kept downloads/);
    await expect(sheet.getByRole('radio', { name: /Until I watch it/ })).toBeChecked();
    await expect(sheet.getByRole('radio', { name: /Until I watch it/ })).toHaveAccessibleName(/30 days at most/);
    await expectAccessibleAndContained(page);

    await sheet.getByText('7 days').click();
    await sheet.getByRole('button', { name: 'Download & keep' }).click();
    await expect(toast(page, `Keeping ${DUNE} on disk`)).toBeVisible();
    await expect(sheet).toBeHidden();
    const kept = page.locator('.primary-panel');
    await expect(kept).toContainText(/Kept until \w{3} \d{1,2}/);

    await moreAction(page, testInfo, DUNE, 'Change duration…');
    const change = page.getByRole('dialog', { name: 'Kept on disk' });
    await expect(change.getByRole('button', { name: 'Change duration' })).toBeDisabled();
    await change.getByText('Until I remove it').click();
    await change.getByRole('button', { name: 'Change duration' }).click();
    await expect(toast(page, `Changed how long ${DUNE} is kept`)).toBeVisible();
    await expect(change).toBeHidden();
    await expect(kept.getByText('Kept', { exact: true })).toBeVisible();

    await moreAction(page, testInfo, DUNE, 'Stop keeping…');
    const confirm = page.getByRole('alertdialog', { name: `Stop keeping ${DUNE}?` });
    await expect(confirm).toContainText('What’s downloaded stays as cache and plays until it’s cleared to make room.');
    await confirm.getByRole('button', { name: 'Stop keeping' }).click();
    await expect(toast(page, `Stopped keeping ${DUNE}`)).toBeVisible();
    await expect(kept.getByText(/^Kept/)).toHaveCount(0);
    await expect(panel(page, 'Ready in Infuse')).toBeVisible();
  });

  test('a kept movie downloads the file it gets', async ({ page }) => {
    await signIn(page, '/movie/1054867');
    const needs = panel(page, 'In your library · no torrent yet');
    await expect(needs).toContainText(/Kept until watched · by \w{3} \d{1,2}/);
    await expect(needs).toContainText('No file yet');

    await page.getByRole('button', { name: 'Find a torrent' }).click();
    const search = page.getByRole('dialog', { name: /^Find a source · One Battle After Another/ });
    await search
      .getByRole('article', { name: 'One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX' })
      .getByRole('button', { name: 'Add & assign to movie' })
      .click();
    await expect(toast(page, 'Torrent assigned to movie')).toBeVisible({ timeout: 20_000 });
    await page.keyboard.press('Escape');
    await expect(search).toBeHidden();
    // The Keep wants the new file, so it downloads (a file nothing keeps would be ready: title-movie.spec.ts)
    const downloading = panel(page, 'Downloading');
    await expect(downloading).toContainText('Step 3 of 4');
    await expect(downloading).toContainText(/Kept until watched · by/);
  });

  test('a Keep that doesn’t fit says so before it’s sent, and links to Settings › Storage', async ({ page }) => {
    await scenario(page, 'downloads-full');
    await signIn(page, '/movie/693134?sheet=keep&movie=693134');
    const sheet = page.getByRole('dialog', { name: 'Download & keep' });
    const alert = sheet.getByRole('alert');
    await expect(alert).toContainText('Not enough space: needs 18.4 GB, 1.2 GB left.');
    await expect(alert).toContainText('Free some kept downloads or raise the limit in Settings › Storage.');
    await expect(alert.getByRole('link', { name: 'Settings › Storage' })).toHaveAttribute('href', '/settings?section=storage');
    await expect(sheet.getByRole('button', { name: 'Download & keep' })).toBeDisabled();
    await expectAccessibleAndContained(page);
  });

  test('a season: keep it from the season row; its episodes show Kept', async ({ page }) => {
    await signIn(page, '/tv/209867');
    // Frieren S01E27 is a smart pick (Up next)
    const row = (n: number) => page.locator('.eprow').filter({ hasText: `S01E${n}` });
    await expect(row(27).getByText('Smart', { exact: true })).toBeAttached();

    await page.getByRole('button', { name: 'Download & keep season' }).click();
    await expect(page).toHaveURL(/[?&]sheet=keep&show=209867&scope=season&season=1/);
    const sheet = page.getByRole('dialog', { name: 'Download & keep' });
    await expect(sheet).toContainText(`${FRIEREN} · Season 1`);
    await expect(sheet).toContainText(/Needs [\d.]+ [GM]B · [\d.]+ GB left for kept downloads/);
    await sheet.getByRole('button', { name: 'Download & keep' }).click();
    await expect(toast(page, `Keeping ${FRIEREN} · Season 1 on disk`)).toBeVisible();
    await expect(page.getByRole('button', { name: /^Season kept until watched · by \w{3} \d{1,2} – change or stop keeping this season$/ })).toBeVisible();
    // Kept wins over smart
    await expect(row(27).getByText('Kept', { exact: true })).toBeAttached();
    await expectAccessibleAndContained(page);
  });

  test('an episode its season keeps opens the season’s keep, and Back comes back', async ({ page }) => {
    await signIn(page, '/tv/82728?season=3&sheet=episode&tmdb=82728&se=S03E01');
    const episode = page.getByRole('dialog', { name: 'Episode 1' });
    const keep = episode.getByRole('region', { name: 'Kept on disk' });
    await expect(keep).toContainText('Season 3 is kept, so this episode is too.');
    await expect(keep).toContainText('On disk');
    await expectAccessibleAndContained(page);

    await keep.getByRole('button', { name: 'Change season 3’s keep' }).click();
    const season = page.getByRole('dialog', { name: 'Kept on disk' });
    await expect(season).toContainText('Bluey · Season 3');
    await expect(season.getByRole('button', { name: 'Stop keeping' })).toBeVisible();
    await page.goBack();
    await expect(season).toBeHidden();
    await expect(episode).toBeVisible();
  });

  test('a smart pick: keep it from its episode', async ({ page }) => {
    await signIn(page, '/tv/209867');
    await page.getByRole('button', { name: /^S01E27 options/ }).click();
    const episode = page.getByRole('dialog', { name: 'An Ordinary Mage' });
    const keep = episode.getByRole('region', { name: 'Kept on disk' });
    await expect(keep).toContainText(/Smart pick · until watched · by/);
    await keep.getByRole('button', { name: 'Keep it' }).click();

    const sheet = page.getByRole('dialog', { name: 'Keep this smart pick' });
    await expect(sheet).toContainText('Downloaded ahead because you’re watching the show.');
    await expect(sheet.getByRole('button', { name: 'Dismiss smart pick' })).toBeVisible();
    await sheet.getByRole('button', { name: 'Keep it' }).click();
    await expect(toast(page, `Keeping ${FRIEREN} · S01E27 on disk`)).toBeVisible();
    // Back on the episode: kept now, so its duration can change
    await expect(keep.getByRole('button', { name: 'Change duration' })).toBeVisible();
    await expect(keep).toContainText(/Kept until watched · by/);
  });
});
