import { expect, test, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * HDRezka as a second source (#67), against the MSW mocks: One Battle After
 * Another needs a file, Dune: Part Two plays from a torrent. Scenarios in
 * src/mocks/scenario.ts (hdrezka-blocked, -premium, -off).
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
const BEST = 'Битва за битвой (2025)';

/** The panel's button on desktop, the dock's icon on phone: both "Stream from HDRezka". */
async function openFromPanel(page: Page) {
  await page.getByRole('button', { name: 'Stream from HDRezka' }).click();
  await expect(page).toHaveURL(/[?&]sheet=hdrezka&movie=1054867/);
  await expect(sheet(page)).toBeVisible();
}

test.describe('HDRezka', () => {
  test('stream a movie: best match, voice-over and quality, then it’s ready with the stream', async ({ page }) => {
    await signIn(page, '/movie/1054867');
    await expect(page.getByRole('region', { name: 'In your library · no torrent yet' })).toContainText('Add · Source · Download · Ready');
    await openFromPanel(page);
    const dialog = sheet(page);
    await expect(dialog).toHaveAccessibleName('Find a source · One Battle After Another 2025 · 2h 42m');
    await expect(dialog.getByRole('tab', { name: 'HDRezka' })).toHaveAttribute('aria-selected', 'true');
    await expect(dialog.getByRole('searchbox', { name: 'Search HDRezka, or paste a link' })).toHaveValue('One Battle After Another');

    const best = dialog.getByRole('article', { name: BEST });
    await expect(best).toContainText('Best match');
    await expect(best).toContainText('Exact title');
    await expect(dialog.getByRole('article', { name: 'Битва полов (2017)' })).toContainText('Weak match');
    await expectAccessibleAndContained(page);

    await best.getByRole('button', { name: `Use ${BEST}` }).click();
    await expect(dialog.getByRole('radio', { name: 'HDrezka Studio' })).toBeChecked();
    await expect(dialog.getByRole('radio', { name: 'Дубляж, Premium' })).toBeDisabled();
    await dialog.getByRole('radio', { name: 'Оригинал (+субтитры)' }).check();
    await expect(dialog.getByText('Subtitles: English, Русский')).toBeVisible();
    await expect(dialog.getByRole('radio', { name: '1080p, 4.3 GB' })).toBeChecked();
    await expect(dialog.getByRole('radio', { name: '1080p Ultra, Premium' })).toBeDisabled();
    await expectAccessibleAndContained(page);

    await dialog.getByRole('button', { name: 'Use HDRezka' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Streaming from HDRezka' })).toBeVisible();
    await expect(dialog).toBeHidden();
    await expect(page).toHaveURL(/\/movie\/1054867$/);
    await expect(page.getByRole('region', { name: 'Ready in Infuse' })).toContainText('HDRezka · Оригинал (+субтитры) · 1080p · 4.3 GB');
    const current = page.getByRole('region', { name: 'Current stream' });
    await expect(current).toContainText('Битва за битвой');
    await expect(current.getByRole('link', { name: 'Open on HDRezka' })).toHaveAttribute('href', 'https://rezka.ag/films/action/78655-bitva-za-bitvoy-2025.html');
    await expectAccessibleAndContained(page);

    // Change voice-over or quality reopens it on the title, at what plays now
    await current.getByRole('button', { name: 'Change voice-over or quality' }).click();
    await expect(sheet(page).getByRole('radio', { name: 'Оригинал (+субтитры)' })).toBeChecked();
    await expect(sheet(page).getByText('This is what Infuse plays now')).toBeVisible();
    await expect(sheet(page).getByRole('button', { name: 'Use HDRezka' })).toBeDisabled();
  });

  test('HDRezka’s bot check: an error with Retry and the backend’s message', async ({ page }) => {
    await scenario(page, 'hdrezka-blocked');
    await signIn(page, '/movie/1054867');
    await openFromPanel(page);
    const alert = sheet(page).getByRole('alert');
    await expect(alert).toContainText('HDRezka is showing its bot check');
    await expect(alert).toContainText('HDRezka answered with its anti-bot page instead of the title.');
    await expectAccessibleAndContained(page);

    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await alert.getByRole('button', { name: 'Retry' }).click();
    await expect(sheet(page).getByRole('article', { name: BEST })).toBeVisible();
  });

  test('every voice-over needs Premium: explained, and nothing to use', async ({ page }) => {
    await scenario(page, 'hdrezka-premium');
    await signIn(page, '/movie/1054867');
    await openFromPanel(page);
    await sheet(page).getByRole('button', { name: `Use ${BEST}` }).click();
    await expect(sheet(page).getByText('Every voice-over here needs HDRezka Premium')).toBeVisible();
    for (const radio of await sheet(page).getByRole('radio').all()) await expect(radio).toBeDisabled();
    await expect(sheet(page).getByRole('button', { name: 'Use HDRezka' })).toBeDisabled();
    await expectAccessibleAndContained(page);
  });

  test('the Torrents tab moves to torrent search, and closing keeps the picks', async ({ page }) => {
    await signIn(page, '/movie/1054867');
    await openFromPanel(page);
    await sheet(page).getByRole('button', { name: `Use ${BEST}` }).click();
    await sheet(page).getByRole('radio', { name: 'Jaskier, Director’s cut' }).check();
    await sheet(page).getByRole('button', { name: 'Close (your choices are kept)' }).click();
    await expect(sheet(page)).toBeHidden();
    await expect(page).toHaveURL(/\/movie\/1054867$/);

    await openFromPanel(page);
    await expect(sheet(page).getByText('Picked up where you left off')).toBeVisible();
    await expect(sheet(page).getByRole('radio', { name: 'Jaskier, Director’s cut' })).toBeChecked();

    await sheet(page).getByRole('tab', { name: 'Torrents' }).click();
    await expect(sheet(page)).toBeHidden();
    const search = page.getByRole('dialog', { name: 'Search Torrents: One Battle After Another (2025)' });
    await expect(search).toBeVisible();
    await expect(search.getByRole('searchbox', { name: 'Search torrents' })).toHaveValue('One Battle After Another 2025');
  });

  test('a stream replaces a torrent after asking, from Replace in the current torrent', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    const torrent = page.getByRole('region', { name: 'Current torrent' });
    await torrent.getByRole('button', { name: 'Replace' }).click();
    // A menu on desktop, a sheet on phone
    if (testInfo.project.name === 'mobile') await page.getByRole('dialog', { name: 'Replace with' }).getByRole('button', { name: 'Stream from HDRezka' }).click();
    else await page.getByRole('menuitem', { name: 'Stream from HDRezka' }).click();
    await expect(page).toHaveURL(/[?&]sheet=hdrezka&movie=693134/);

    const dialog = sheet(page);
    await dialog.getByRole('button', { name: 'Use Дюна: Часть вторая (2024)' }).click();
    await expect(dialog.getByRole('radio', { name: '1080p, 4.5 GB' })).toBeChecked();
    await dialog.getByRole('button', { name: 'Use HDRezka' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Stream Dune: Part Two from HDRezka?' });
    await expect(confirm).toContainText('It may show up as a new file (it ends in .mp4), so Infuse may forget where you stopped.');
    await expectNoA11yViolations(page);
    await confirm.getByRole('button', { name: 'Use HDRezka' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Streaming from HDRezka' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Ready in Infuse' })).toContainText('HDRezka · Дубляж · 1080p · 4.5 GB');
    await expect(page.getByRole('region', { name: 'Ready in Infuse' }).getByLabel('WebDAV path')).toHaveText('/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mp4');
    await expect(page.getByRole('heading', { name: 'Current torrent' })).toBeHidden();
    await expect(page.getByRole('heading', { name: 'Current stream' })).toBeVisible();
  });

  test('Back closes the sheet; the ⋯ menu opens it too', async ({ page }) => {
    await signIn(page, '/movie/693134');
    await page.getByRole('button', { name: 'More actions for Dune: Part Two' }).click();
    await page.getByText('Stream from HDRezka…').click();
    await expect(sheet(page)).toBeVisible();
    await page.goBack();
    await expect(sheet(page)).toBeHidden();
    await expect(page).toHaveURL(/\/movie\/693134$/);
  });

  test('with HDRezka off, nothing offers it', async ({ page }) => {
    await scenario(page, 'hdrezka-off');
    await signIn(page, '/movie/1054867');
    await expect(page.getByRole('button', { name: 'Find a torrent' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Stream from HDRezka' })).toHaveCount(0);
    await page.getByRole('button', { name: 'More actions for One Battle After Another' }).click();
    await expect(page.getByText('Refresh from TMDB')).toBeVisible();
    await expect(page.getByText('Stream from HDRezka…')).toHaveCount(0);
  });
});
