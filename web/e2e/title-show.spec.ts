import { expect, test, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * The show page (PLAN F2) against the MSW mocks: Frieren needs episodes
 * 29–38 and has files to review, Doctor Who has named specials, The Good
 * Place isn't in the library and One Piece uses absolute numbers.
 */

const FRIEREN = 'Frieren: Beyond Journey’s End';

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page) {
  await expectNoA11yViolations(page);
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

const seasons = (page: Page) => page.getByRole('tablist', { name: 'Seasons' });
const toast = (page: Page, text: string) => page.getByRole('status').filter({ hasText: text });

test.describe('show page', () => {
  test('Frieren: seasons with Specials last, the review banner and the next missing episode', async ({ page }) => {
    await signIn(page, '/tv/209867');
    await expect(page.getByRole('heading', { name: FRIEREN, level: 1 })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Episodes 29–38 have no torrent' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Find episodes 29–38' })).toBeVisible();
    await expect(page.getByText(/^\d+ of 38 ready$/)).toBeVisible();

    await expect(seasons(page).getByRole('tab')).toHaveText([/^Season 1\s*38$/, /^Specials\s*6$/]);
    await expect(seasons(page).getByRole('tab', { name: /Season 1/ })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByText('2 files need a quick review')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/101');

    // Episodes 1–25 are all watched: one row until you ask for them
    const list = page.getByRole('list', { name: 'Season 1 episodes' });
    await expect(list.getByRole('listitem')).toHaveCount(13);
    await expect(page.getByRole('group', { name: 'Episode ranges' }).getByRole('button', { name: /29–38/ })).toHaveAttribute('aria-pressed', 'true');
    const next = list.locator('.eprow.is-next');
    await expect(next).toContainText('Up next');
    await expect(next).toContainText('Let’s Go Somewhere We Can’t Reach');
    // Absolute numbers that match the in-season ones aren't repeated
    await expect(next).toContainText('S01E29');
    await expect(next).not.toContainText('#29');
    await expect(list.getByRole('link', { name: 'Review the mapping for S01E26' })).toHaveAttribute('href', '/mapping/101');
    await expectAccessibleAndContained(page);

    await seasons(page).getByRole('tab', { name: /Specials/ }).click();
    await expect(page).toHaveURL(/[?&]season=0/);
    await expect(page.getByRole('list', { name: 'Specials episodes' }).getByRole('listitem')).toHaveCount(6);
  });

  test('Find on a missing row opens the search for that episode', async ({ page }) => {
    await signIn(page, '/tv/209867');
    await page.getByRole('button', { name: 'Find a torrent for S01E30' }).click();
    const search = page.getByRole('dialog', { name: `Search: ${FRIEREN} S01E30` });
    await expect(search).toBeVisible();
    await expect(search.getByRole('searchbox', { name: 'Search torrents' })).toHaveValue(`${FRIEREN} S01E30`);
    await search.getByRole('button', { name: 'Close modal' }).click();
    await expect(search).toBeHidden();
  });

  test('show settings save, and turning specials off hides their tab', async ({ page }) => {
    await signIn(page, '/tv/209867');
    await page.getByRole('button', { name: 'Show settings' }).click();
    const sheet = page.getByRole('dialog', { name: `Show settings · ${FRIEREN}` });
    await expect(sheet.getByRole('radio', { name: /Absolute \(anime\)/ })).toBeChecked();
    await expect(sheet.getByRole('combobox', { name: 'Episode group' })).toBeDisabled();
    await expectNoA11yViolations(page);

    await sheet.getByRole('switch', { name: 'Include specials' }).click();
    await sheet.getByRole('button', { name: 'Save' }).click();
    await expect(toast(page, 'Show settings saved')).toBeVisible();
    await expect(sheet).toBeHidden();
    await expect(seasons(page).getByRole('tab')).toHaveText([/^Season 1/]);
  });

  test('the episode sheet: how to play, subtitles, and removing the file', async ({ page }) => {
    await signIn(page, '/tv/209867');
    await page.getByRole('button', { name: /^S01E28 options/ }).click();
    const sheet = page.getByRole('dialog', { name: 'It Would Be Embarrassing When We Met Again' });
    await expect(sheet).toContainText(`${FRIEREN} · S01E28`);
    await expect(page).toHaveURL(/[?&]sheet=episode&.*se=S01E28/);
    await expect(sheet.getByLabel('WebDAV path of this episode')).toContainText('S01E28');

    const subtitles = sheet.getByRole('region', { name: 'Subtitles' });
    const yours = subtitles.getByRole('list', { name: 'Your subtitles' });
    await expect(yours).toContainText('English');
    await expect(yours.getByRole('button')).toHaveCount(0);
    await subtitles.getByRole('button', { name: 'Search' }).click();
    await subtitles.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' }).getByRole('button', { name: /^Get/ }).first().click();
    await expect(toast(page, 'Azerbaijani subtitles added')).toBeVisible();
    await expect(yours).toContainText('Azerbaijani');
    await expectNoA11yViolations(page);

    await sheet.getByRole('button', { name: 'Remove file from episode' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Remove the file from S01E28?' });
    await confirm.getByRole('button', { name: 'Remove file' }).click();
    await expect(toast(page, 'Removed the file from S01E28')).toBeVisible();
    await expect(sheet).toBeHidden();
    await expect(page.getByRole('region', { name: 'Episodes 28–38 have no torrent' })).toBeVisible();
  });

  test('the episode sheet closes with Escape and with Back, and hands Replace to the search', async ({ page }) => {
    await signIn(page, '/tv/209867');
    const options = page.getByRole('button', { name: /^S01E28 options/ });
    const sheet = page.getByRole('dialog', { name: 'It Would Be Embarrassing When We Met Again' });

    await options.click();
    await expect(sheet).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/tv\/209867$/);

    await options.click();
    await expect(sheet).toBeVisible();
    await page.goBack();
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/tv\/209867$/);

    await options.click();
    await sheet.getByRole('button', { name: 'Replace torrent' }).click();
    await expect(sheet).toBeHidden();
    const search = page.getByRole('dialog', { name: `Search: ${FRIEREN} S01E28` });
    await expect(search).toBeVisible();
    await expect(search.getByRole('searchbox', { name: 'Search torrents' })).toHaveValue(`${FRIEREN} S01E28`);
  });

  test('Doctor Who opens on its specials with ?season=0', async ({ page }) => {
    await signIn(page, '/tv/57243?season=0');
    await expect(seasons(page).getByRole('tab', { name: /Specials/ })).toHaveAttribute('aria-selected', 'true');
    await expect(seasons(page).getByRole('tab').last()).toHaveText(/^Specials/);
    const list = page.getByRole('list', { name: 'Specials episodes' });
    await expect(list.getByRole('listitem')).toHaveCount(8);
    await expect(list).toContainText('The Waters of Mars');
    await expect(list.getByRole('button', { name: 'Find a torrent for S00E07' })).toBeVisible();
    await expect(page.getByText('1 file needs a quick review')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('not in the library: Add, then it needs episodes', async ({ page }) => {
    await signIn(page, '/tv/66573');
    await expect(page.getByRole('region', { name: 'Not in your library' })).toContainText('53 episodes across 4 seasons.');
    await expect(page.getByRole('heading', { name: 'Add it to see what’s ready' })).toBeVisible();
    await expect(seasons(page).getByRole('tab')).toHaveCount(4);
    await expect(page.getByRole('button', { name: 'Show settings' })).toBeHidden();
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Add to library' }).click();
    await expect(toast(page, 'Added to library')).toBeVisible();
    await expect(page.getByRole('region', { name: 'In your library · no episodes yet' })).toBeVisible();
    await expect(page.getByRole('list', { name: 'Season 1 episodes' }).getByRole('listitem')).toHaveCount(13);
    await expect(page.getByRole('button', { name: 'Find season pack' })).toBeVisible();
  });

  test('One Piece labels episodes with absolute numbers', async ({ page }) => {
    await signIn(page, '/tv/37854');
    await expect(seasons(page).getByRole('tab')).toHaveText([/^Wano Country/, /^Egghead/, /^Specials/]);
    await expect(seasons(page).getByRole('tab', { name: /Egghead/ })).toHaveAttribute('aria-selected', 'true');
    const list = page.getByRole('list', { name: 'Egghead episodes' });
    await expect(list.locator('#ep-26')).toContainText('#1111 · S22E26');
    await expect(list.locator('#ep-37')).toContainText(/Airs/);
    await expect(page.getByText('Absolute numbering · 192 episodes')).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('long seasons: Wano Country (156 episodes) is virtualised', async ({ page }) => {
    await signIn(page, '/tv/37854?season=21');
    const list = page.getByRole('list', { name: 'Wano Country episodes' });
    await expect(list.getByRole('listitem').first()).toHaveAttribute('aria-setsize', '156');
    const rendered = await list.getByRole('listitem').count();
    expect(rendered).toBeGreaterThan(5);
    expect(rendered).toBeLessThan(156);
    await expect(page.getByRole('group', { name: 'Episode ranges' }).getByRole('button')).toHaveText([/^Wano: Act 1 · 892–916/, /^Wano: Act 2/, /^Wano: Act 3/]);
    await expectAccessibleAndContained(page);
  });

  test('jump to episode: an absolute number lands on its row, in another season too', async ({ page }) => {
    await signIn(page, '/tv/37854');
    const jump = page.getByRole('textbox', { name: 'Jump to episode number' });
    await jump.fill('1100');
    await jump.press('Enter');
    const row = page.locator('#ep-15');
    await expect(row).toContainText('#1100 · S22E15');
    await expect(row).toHaveClass(/flash/);
    await expect(row).toBeInViewport();

    await jump.fill('5000');
    await jump.press('Enter');
    await expect(page.getByText('Egghead has 38 episodes.')).toBeVisible();

    await jump.fill('1000');
    await jump.press('Enter');
    await expect(page).toHaveURL(/[?&]season=21/);
    await expect(seasons(page).getByRole('tab', { name: /Wano Country/ })).toHaveAttribute('aria-selected', 'true');
    const far = page.locator('#ep-109');
    await expect(far).toContainText('#1000 · S21E109');
    await expect(far).toBeInViewport();
    await expect(page).not.toHaveURL(/[?&]ep=/);
  });

  test('range chips jump, and the watched run expands in place', async ({ page }) => {
    await signIn(page, '/tv/209867');
    const list = page.getByRole('list', { name: 'Season 1 episodes' });
    const chips = page.getByRole('group', { name: 'Episode ranges' });
    await expect(chips.getByRole('button')).toHaveText(['1–10', '11–20', '21–28', /^29–38\s*10 missing$/]);

    await chips.getByRole('button', { name: '11–20' }).click();
    await expect(chips.getByRole('button', { name: '11–20' })).toHaveAttribute('aria-pressed', 'true');
    await expect(list.locator('#ep-11')).toBeInViewport();
    await expect(list.getByRole('listitem')).toHaveCount(38);

    await page.goto('/tv/209867');
    await page.getByRole('button', { name: 'Show episodes 1–25 (all watched)' }).click();
    await expect(page.getByRole('button', { name: /Show episodes/ })).toHaveCount(0);
    await expect(list.getByRole('listitem')).toHaveCount(38);
    await expect(list.locator('#ep-1')).toContainText('The Journey’s End');
  });

  test('density: rows with stills via ?view=rich', async ({ page }) => {
    await signIn(page, '/tv/95396');
    const density = page.getByRole('group', { name: 'Episode list density' });
    await density.getByRole('button', { name: 'With stills' }).click();
    await expect(page).toHaveURL(/[?&]view=rich/);
    await expect(page.locator('.eprow--rich').first()).toBeVisible();
    await expect(density.getByRole('button', { name: 'With stills' })).toHaveAttribute('aria-pressed', 'true');
    await expectAccessibleAndContained(page);
    await density.getByRole('button', { name: 'Compact' }).click();
    await expect(page).not.toHaveURL(/view=rich/);
    await expect(page.locator('.eprow--rich')).toHaveCount(0);
  });
});
