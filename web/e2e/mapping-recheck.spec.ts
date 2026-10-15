import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * "Re-check with AI" on the mapping review (PLAN F4; #5) against the MSW job
 * mock. Job 101 (Frieren) has three unresolved files; `job-hold:resolving`
 * keeps a re-check working until the test lifts it.
 */

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

const isPhone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';
const panel = (page: Page) => page.getByRole('region', { name: 'Re-check with AI' });
/** Its name stays "Re-check with AI"; below 640 px only "Re-check" shows. */
const toggle = (page: Page) => panel(page).getByRole('button', { name: 'Re-check with AI' });
const runButton = (page: Page) => panel(page).locator('.map-ai-run').getByRole('button', { name: 'Re-check' });
const row = (page: Page, file: string) => page.locator('.mrow').filter({ hasText: file });

test.describe('re-check with AI', () => {
  test('a hint chip, Re-check, Noir works while the groups stay usable, then Updated rows', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'job-hold:resolving'));
    await signIn(page, '/mapping/101');
    await expect(panel(page)).toContainText('Not quite right?');
    await expect(panel(page)).toContainText('Noir can take another look. A hint helps with odd numbering.');
    await expect(toggle(page)).toHaveAttribute('aria-expanded', 'false');

    await toggle(page).click();
    await expect(toggle(page)).toHaveAttribute('aria-expanded', 'true');
    const hint = panel(page).getByLabel('Hint for Noir (optional)');
    await expect(hint).toHaveAttribute('placeholder', 'e.g. these are absolute numbers');
    // Desktop puts the cursor in the hint; a phone keeps its keyboard down
    if (isPhone(testInfo)) await expect(hint).not.toBeFocused();
    else await expect(hint).toBeFocused();
    const chips = panel(page).getByRole('group', { name: 'Example hints' }).getByRole('button');
    await expect(chips).toHaveText(['these are absolute numbers', '28.5 is a recap', 'v2 replaces v1']);
    const scopes = panel(page).getByRole('group', { name: 'Which files to re-check' }).getByRole('button');
    // As the backend picks them: undecided episode files, pending or in a conflict for unresolved
    await expect(scopes).toHaveText(['Unresolved · 3', 'All files · 13']);
    await expect(scopes.first()).toHaveAttribute('aria-pressed', 'true');
    await expectAccessibleAndContained(page);

    await chips.nth(1).click();
    await expect(hint).toHaveValue('28.5 is a recap');
    await expect(chips.nth(1)).toHaveAttribute('aria-pressed', 'true');
    await runButton(page).click();

    // Screen readers hear it through the panel's one live region; the eyes and steps show it
    const live = panel(page).getByRole('status');
    await expect(live).toHaveText('Re-checking 3 files with your hint… “28.5 is a recap”');
    await expect(panel(page).getByText('Re-checking 3 files with your hint…', { exact: true })).toBeVisible();
    await expect(panel(page).getByRole('listitem')).toHaveText([/Read file names/, /Comparing with TMDB air dates/, /Updating suggestions/]);
    await expectAccessibleAndContained(page);

    // The groups stay usable: a file you decide now is yours, and the re-check leaves it alone
    await page.getByRole('button', { name: 'Accept #35v2' }).click();
    await expect(page.getByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toHaveAttribute('aria-pressed', 'true');

    // Lift the hold: the run ends on the next poll
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await expect(live).toHaveText('Updated 2 suggestions, marked Updated below. Accept the ones that look right.');
    await expect(panel(page).locator('.map-ai-done')).toBeVisible();
    await expect(toggle(page)).toHaveAttribute('aria-expanded', 'false');
    const recap = row(page, 'Frieren - 28.5');
    await expect(recap.getByText('Updated', { exact: true })).toBeVisible();
    await expect(recap).toContainText('AI, with your hint: 28.5 is the recap “The Journey So Far” that aired Mar 15 → S00E03');
    await expect(recap).toContainText('High');
    await expect(row(page, 'Special Extra')).toContainText('Updated');
    await expect(row(page, 'Frieren - 35v2')).not.toContainText('Updated');
    await expect(page.getByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('ai-off: no panel', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'ai-off'));
    await signIn(page, '/mapping/101');
    await expect(page.getByRole('button', { name: /^Needs review/ })).toBeVisible();
    await expect(panel(page)).toHaveCount(0);
    await expectAccessibleAndContained(page);
  });

  test('ai-error: the run fails, nothing changes, and the panel says so', async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'ai-error'));
    await signIn(page, '/mapping/101');
    await toggle(page).click();
    await runButton(page).click();
    const live = panel(page).getByRole('status');
    await expect(live).toHaveText('Re-checking 3 files… Comparing names, air dates and runtimes');
    await expect(live).toHaveText("Noir couldn’t finish. The AI didn't answer in time, so nothing changed.");
    await expect(panel(page).locator('.map-ai-note')).toHaveText("Noir couldn’t finish. The AI didn't answer in time, so nothing changed.");
    await expect(page.getByText('Updated', { exact: true })).toHaveCount(0);
    await expect(row(page, 'Frieren - 28.5')).toContainText('Low');
    await expectAccessibleAndContained(page);
  });
});
