import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * The mapping review (PLAN F4; #5) against the MSW job mock: job 101
 * (Frieren, two files to review, one unmatched), 102 (One Piece, a large
 * picker catalog) and 103 (Doctor Who, two files want the same special).
 * PATCH, apply and discard stick until a full reload.
 */

/** Entry animations (rise, toast) done, so axe reads final colours; endless ones (shimmer, spinners) don't count. */
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

const isPhone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';
const applyButton = (page: Page) => page.getByRole('button', { name: /^Apply \d+ mappings?$/ });
const row = (page: Page, file: string) => page.locator('.mrow').filter({ hasText: file });
const toast = (page: Page, text: string) => page.getByRole('status').filter({ hasText: text });
/** The pane on desktop, the sheet on phone. */
const picker = (page: Page, testInfo: TestInfo) =>
  isPhone(testInfo) ? page.getByRole('dialog', { name: 'Choose an episode' }) : page.getByRole('complementary', { name: 'Episode picker' });

/** A new job for Frieren (in the browser's mock), opened without a reload so the mock keeps it. */
async function openNewJob(page: Page): Promise<number> {
  const jobId = await page.evaluate(async () => {
    const response = await fetch('/api/shows/1/assign-torrent', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ magnet_uri: 'magnet:?xt=urn:btih:aaaabbbbccccddddeeeeffff0000111122223333', indexer: 'Nyaa' }),
    });
    return ((await response.json()) as { job_id: number }).job_id;
  });
  await page.evaluate((id) => (window as unknown as { next: { router: { push: (href: string) => void } } }).next.router.push(`/mapping/${id}`), jobId);
  await expect(page).toHaveURL(new RegExp(`/mapping/${jobId}$`));
  return jobId;
}

test.describe('mapping review', () => {
  test('Frieren: from the show’s banner to Infuse, and the banner is gone', async ({ page }, testInfo) => {
    await signIn(page, '/tv/209867');
    await page.getByRole('link', { name: 'Review mapping' }).click();
    await expect(page).toHaveURL(/\/mapping\/101$/);
    await expect(page.getByRole('heading', { name: 'Check the episodes', level: 1 })).toBeVisible();
    await expect(page.getByText('[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]')).toBeVisible();
    await expect(page.locator('.map-summary')).toHaveText(/9 matched\s*2 need review\s*1 special\s*3 ignored\s*1 unmatched/);
    // A task page: no phone bar or tab bar, the foot sits at the bottom
    await expect(page.locator('.tabbar')).toBeHidden();
    await expect(page.locator('.phone-bar')).toBeHidden();
    await expect(applyButton(page)).toHaveAttribute('aria-disabled', 'true');
    await expect(page.locator('.map-foot-msg')).toHaveText('Decide on 2 files to apply');
    // Below 1024 px the long, settled groups start collapsed, with a peek
    const matched = page.getByRole('button', { name: /^Matched/ });
    await expect(matched).toHaveAttribute('aria-expanded', isPhone(testInfo) ? 'false' : 'true');
    if (isPhone(testInfo)) await expect(matched).toContainText('S01 E29–E34, E36–E38');
    await expectAccessibleAndContained(page);

    // Accept one
    await page.getByRole('button', { name: 'Accept #35v2' }).click();
    await expect(page.getByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('.map-foot-msg')).toHaveText('Decide on 1 file to apply');

    // Change one through the picker
    await page.getByRole('button', { name: 'Change episode for #28.5' }).click();
    const episodes = picker(page, testInfo);
    await expect(episodes.getByRole('option', { name: /S00E03/ })).toHaveAttribute('aria-selected', 'true');
    await expect(episodes.getByRole('option', { name: /S01E31/ })).toContainText('Taken · #31');
    await episodes.getByRole('searchbox', { name: 'Search episodes' }).fill('sp5');
    await expect(episodes.getByRole('option')).toHaveCount(1);
    await episodes.getByRole('option', { name: /S00E05/ }).click();
    if (isPhone(testInfo)) await expect(episodes).toBeHidden();
    const recap = row(page, 'Frieren - 28.5');
    await expect(recap).toContainText('S00E05');
    await expect(recap).toContainText('You picked this episode');

    // Ignore the unmatched file: unmatched files never block, but now everything has a place
    await expect(page.locator('.map-foot-msg')).toHaveText('1 file stays unmapped. You can map it later.');
    // By keyboard: the Ignore button goes away, so focus moves to the row's Undo
    await page.getByRole('button', { name: 'Ignore Special Extra' }).focus();
    await page.keyboard.press('Enter');
    await expect(row(page, 'Special Extra')).toContainText(/Ignored\s*· not in Infuse/);
    await expect(row(page, 'Special Extra').getByRole('button', { name: 'Undo: map Special Extra again' })).toBeFocused();
    await expect(page.locator('.map-foot-msg')).toHaveText('Everything has a place');
    await expect(applyButton(page)).toHaveAttribute('aria-disabled', 'false');

    await applyButton(page).click();
    await expect(page.getByRole('heading', { name: 'Ready in Infuse.', level: 1 })).toBeVisible();
    const done = page.getByRole('list', { name: 'Mapped to Infuse' });
    await expect(done.getByRole('listitem')).toHaveText([/Season 1\s*E29–E38\s*10 ready/, /Specials\s*E05, E06\s*2 ready/, /Left out\s*4 ignored/]);
    const ready = toast(page, 'Frieren S01E29–E38 are ready');
    await expect(ready).toBeVisible();
    await expectAccessibleAndContained(page);
    // The sticky toast never covers Applied's buttons (on phone it sits at the top)
    const back = page.getByRole('link', { name: 'Back to Frieren' });
    const [toastBox, backBox] = [await ready.boundingBox(), await back.boundingBox()];
    if (!toastBox || !backBox) throw new Error('No toast or Back link');
    expect(toastBox.y + toastBox.height <= backBox.y || backBox.y + backBox.height <= toastBox.y || toastBox.x >= backBox.x + backBox.width).toBe(true);

    await back.click();
    await expect(page).toHaveURL(/\/tv\/209867$/);
    await expect(page.getByRole('heading', { name: 'Frieren: Beyond Journey’s End', level: 1 })).toBeVisible();
    await expect(page.getByText(/need a quick review|needs a quick review/)).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Review mapping' })).toHaveCount(0);
  });

  test('Doctor Who: a conflict blocks Apply, and clicking it goes to the conflict', async ({ page }) => {
    await signIn(page, '/mapping/103');
    await expect(page.locator('.map-foot-msg')).toHaveText('Two files point at the same episode. Keep one.');
    const waters = row(page, 'S00E02.The.Waters.of.Mars');
    await expect(waters).toContainText('Same episode as the Waters of Mars Extended Edition file. Keep one of them.');
    await expect(row(page, 'Easter.Special')).toContainText('Replaces the current S00E01 file');
    await expect(applyButton(page)).toHaveAttribute('aria-disabled', 'true');

    // Disabled, but clickable: it applies nothing and focuses the first blocker
    await applyButton(page).click({ force: true });
    await expect(waters.getByRole('button', { name: 'Change episode for S00E02' })).toBeFocused();
    await expect(waters).toBeInViewport();
    await expect(page.getByRole('heading', { name: 'Check the episodes' })).toBeVisible();

    // Keep one: the conflict goes, the pending files still block
    await page.getByRole('button', { name: 'Ignore Waters of Mars Extended Edition' }).click();
    await expect(page.locator('.map-foot-msg')).toHaveText('Decide on 2 files to apply');
    await expect(waters).not.toContainText('Same episode as');
    await applyButton(page).click({ force: true });
    await expect(row(page, 'Easter.Special').getByRole('button', { name: 'Accept S00E01' })).toBeFocused();
    await expectAccessibleAndContained(page);
  });

  test('progress: named steps while the files are matched, then the plan', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'job-hold:classifying'));
    await signIn(page, '/tv/209867');
    await openNewJob(page);
    await expect(page.getByText('Mapping files to episodes')).toBeVisible();
    const steps = page.locator('.map-steps li');
    await expect(steps).toHaveCount(3);
    await expect(steps.nth(0)).toHaveClass(/is-done/);
    await expect(steps.nth(0)).toContainText('Frieren · 2 seasons, 44 episodes');
    await expect(steps.nth(1)).toHaveAttribute('aria-current', 'step');
    await expect(steps.nth(1)).toContainText('16 files · 13 videos');
    await expect(steps.nth(2)).not.toHaveClass(/is-done|is-active/);
    await expect(page.locator('.tabbar')).toBeHidden();
    await expectAccessibleAndContained(page);

    // Lift the hold: the job finishes, the AI step reports, then the plan
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await expect(page.getByText('Done. Here’s the plan.')).toBeVisible();
    await expect(steps.nth(2)).toContainText('3 files');
    await expect(page.getByRole('button', { name: /^Needs review/ })).toBeVisible();
    await expect(page.locator('.map-summary')).toContainText('2 need review');
  });

  test('One Piece: a long catalog is virtualised, searches absolute numbers and works by keyboard', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/102');
    await page.getByRole('button', { name: 'Change episode for #892' }).click();
    const episodes = picker(page, testInfo);
    const list = episodes.getByRole('listbox', { name: 'Episodes' });
    await expect(list.getByRole('option', { name: /S21E01/ })).toHaveAttribute('aria-selected', 'true');
    // 236 episodes in the catalog, only the ones near the viewport rendered
    expect(await list.getByRole('option').count()).toBeLessThan(80);
    await expect(episodes.getByRole('group', { name: 'Jump to season' }).getByRole('button')).toHaveText(['Specials', 'S21 Wano', 'S22 Egghead']);
    await expect(list.getByRole('option', { name: /S21E02/ })).toContainText('Taken · #893');
    await expectNoA11yViolations(page);

    const search = episodes.getByRole('searchbox', { name: 'Search episodes' });
    await search.fill('zzz');
    await expect(episodes.getByText('No episode matches “zzz”.')).toBeVisible();
    await search.fill('#1000');
    await expect(list.getByRole('option')).toHaveText([/S21E109\s*Episode 1000\s*Absolute #1000/]);
    // ↓ moves into the list, Enter picks
    await search.press('ArrowDown');
    await expect(list).toBeFocused();
    await page.keyboard.press('Enter');
    const first = row(page, 'One.Piece.892.1080p');
    await expect(first).toContainText('S21E109');
    await expect(first).toContainText('You picked this episode');
    if (isPhone(testInfo)) await expect(episodes).toBeHidden();
    await expect(page.locator('.map-summary')).toContainText('0 left to review');
  });

  test('discard: confirmed, then back to the show with nothing added', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    await page.getByRole('button', { name: 'More mapping actions' }).click();
    if (isPhone(testInfo)) await page.getByRole('dialog', { name: 'More' }).getByRole('button', { name: 'Discard this mapping…' }).click();
    else await page.getByRole('menuitem', { name: 'Discard this mapping…' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Discard this mapping?' });
    await expect(confirm).toContainText('Nothing is added to Infuse.');
    // The page leaves without flashing the job's new, discarded state
    await page.evaluate(() => {
      const seen = { discarded: false };
      Object.assign(window, { seen });
      new MutationObserver(() => {
        if (document.body.textContent?.includes('This mapping was discarded')) seen.discarded = true;
      }).observe(document.body, { childList: true, subtree: true, characterData: true });
    });
    await confirm.getByRole('button', { name: 'Discard mapping' }).click();
    await expect(page).toHaveURL(/\/tv\/209867$/);
    await expect(toast(page, 'Mapping discarded')).toBeVisible();
    await expect(page.getByText(/need a quick review/)).toHaveCount(0);
    expect(await page.evaluate(() => (window as unknown as { seen: { discarded: boolean } }).seen.discarded)).toBe(false);
  });

  test('keyboard: Accept all hands focus to the first file it accepted', async ({ page }) => {
    await signIn(page, '/mapping/101');
    await page.getByRole('button', { name: 'Accept all 2' }).focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toBeFocused();
    await expect(page.getByRole('button', { name: /^Accept all/ })).toHaveCount(0);
    expect(await page.evaluate(() => document.activeElement === document.body)).toBe(false);
  });

  test('a cold load never shows the phone bar or tab bar, not even before hydration', async ({ page }) => {
    await signIn(page, '/tv/209867');
    // The server's HTML already hides them (CSS on .task-page), so nothing jumps when the page hydrates
    await page.goto('/mapping/101', { waitUntil: 'domcontentloaded' });
    const shown = await page.evaluate(() =>
      ['.tabbar', '.phone-bar'].map((selector) => {
        const element = document.querySelector(selector);
        return element ? getComputedStyle(element).display !== 'none' : false;
      })
    );
    expect(shown).toEqual([false, false]);
    await expect(page.getByRole('heading', { name: 'Check the episodes' })).toBeVisible();
  });

  test('a missing job is a 404; a failed one says why', async ({ page }) => {
    await signIn(page, '/mapping/999');
    await expect(page.getByRole('heading', { name: 'Nothing here but the cat' })).toBeVisible();

    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'job-failed'));
    await page.goto('/mapping/101');
    await expect(page.getByRole('alert').filter({ hasText: 'Mapping failed' })).toContainText('No peers sent the torrent’s file list');
    await expect(page.getByRole('alert').getByRole('link', { name: 'Back to Frieren' })).toHaveAttribute('href', '/tv/209867');
    await expectAccessibleAndContained(page);
  });
});
