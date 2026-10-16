import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Settings › AI & picks (#159), against the MSW mocks: a fresh install's
 * settings (picks on, Opus 5.5 nightly, $15 budget) and the design's month
 * ($3.12 spent; last night's run took 41 s and cost $0.36). Scenarios in
 * src/mocks/scenario.ts (ai-off, ai-budget-invalid, recs-off, recs-refreshing, ask-budget, trakt-off).
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
const saveBar = (page: Page) => page.getByRole('region', { name: 'Unsaved changes' });
const segment = (page: Page, option: string) => page.getByRole('group', { name: 'Recommendation schedule' }).getByRole('button', { name: option, exact: true });
const picks = (page: Page) => page.getByRole('region', { name: 'Personal picks' });
const budgetInput = (page: Page) => page.getByRole('textbox', { name: 'Budget' });
const scheduleHelp = (page: Page) => page.locator('#ai-schedule-help');
const runNow = (page: Page) => page.getByRole('button', { name: 'Run now' });

/** The section list: beside the section on desktop; on phone, Back to it (client-side, so the mocks keep what was saved). */
async function sectionNav(page: Page) {
  const back = page.getByRole('link', { name: 'Settings', exact: true });
  if (await back.isVisible()) await back.click();
  return page.getByRole('navigation', { name: 'Settings sections' });
}

async function openSection(page: Page) {
  await signIn(page, '/settings?section=ai');
  await expect(page.getByRole('heading', { name: 'AI & picks', level: 2 })).toBeVisible();
  await expect(page.getByRole('radio', { name: 'Opus 5.5' })).toBeVisible();
}

test.describe('Settings › AI & picks', () => {
  test('change the schedule and the model; Save sends only those, and the nav follows', async ({ page }) => {
    await openSection(page);
    await expect(page.getByText('When Noir writes your picks, which model it uses, and what it may spend.')).toBeVisible();
    await expect(scheduleHelp(page)).toHaveText('Next run tonight at 03:00; the last one took 41 s and cost $0.36.');
    await expect(segment(page, 'Nightly')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('radio', { name: 'Opus 5.5' })).toBeChecked();
    await expect(page.getByRole('radio', { name: 'Opus 5.5' })).toHaveAccessibleDescription('Sharper reasons, reads more of your history. ≈ $0.36 a run');
    await expect(page.getByRole('radio', { name: 'Sonnet 5.5' })).toHaveAccessibleDescription('Good enough most nights, a bit more generic. ≈ $0.07 a run');
    await expect(page.getByRole('switch', { name: 'Write personal picks' })).toBeChecked();
    await expect(page.getByRole('img', { name: /^Spent \$3\.12 of \$15\.00: / })).toHaveAttribute(
      'aria-label',
      'Spent $3.12 of $15.00: Recommendations $2.18, Ask momoshtrem $0.53, Mapping classifier $0.41'
    );
    await expect(page.getByText(/^Resets [A-Z][a-z]{2} 1$/)).toBeVisible();
    await expect(page.getByText('Haiku 5.5, under $0.001 a torrent.', { exact: false })).toBeVisible();
    await expect(budgetInput(page)).toHaveValue('15.00');
    await expect(saveBar(page)).toBeHidden();
    await expectAccessibleAndContained(page);

    await segment(page, 'Weekly').click();
    await page.getByRole('radio', { name: 'Sonnet 5.5' }).check();
    await expect(saveBar(page)).toBeVisible();
    // The next run is known once the schedule is saved
    await expect(scheduleHelp(page)).toHaveText('Nightly runs at 03:00 server time, weekly on Sunday. The last run took 41 s and cost $0.36.');
    await expectAccessibleAndContained(page);

    const put = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/api/settings/ai'));
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    expect((await put).postDataJSON()).toEqual({ rec_schedule: 'weekly', rec_model: 'claude-sonnet-5-5' });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(saveBar(page)).toBeHidden();
    await expect(scheduleHelp(page)).toContainText(/Next run (tonight|on Sunday) at 03:00; the last one took 41 s/);

    await expect((await sectionNav(page)).getByRole('link', { name: /AI & picks/ })).toContainText('Weekly · $3.12 of $15.00');
  });

  test('Discard puts back what was loaded', async ({ page }) => {
    await openSection(page);
    await page.getByRole('switch', { name: 'Write personal picks' }).uncheck();
    await expect(scheduleHelp(page)).toContainText('Picks are off, so nothing runs');
    await page.getByRole('switch', { name: 'Ask AI about tricky files' }).uncheck();
    await expect(saveBar(page)).toBeVisible();

    await saveBar(page).getByRole('button', { name: 'Discard' }).click();
    await expect(saveBar(page)).toBeHidden();
    await expect(page.getByRole('switch', { name: 'Write personal picks' })).toBeChecked();
    await expect(page.getByRole('switch', { name: 'Ask AI about tricky files' })).toBeChecked();
    await expect((await sectionNav(page)).getByRole('link', { name: /AI & picks/ })).toContainText('Nightly · $3.12 of $15.00');
  });

  test('Run now: the toast, the run’s steps, then the last run and its cost', async ({ page }) => {
    await openSection(page);
    await runNow(page).click();
    const toast = page.getByRole('status').filter({ hasText: 'Writing your picks' }).filter({ hasText: 'About a minute' });
    await expect(toast).toContainText('About a minute. Opus 5.5 · ≈ $0.36');
    await expect(toast.getByRole('button', { name: 'Watch' })).toBeVisible();

    const progress = picks(page).getByRole('status');
    await expect(progress).toContainText('Writing your picks');
    await expect(progress.getByRole('listitem').first()).toBeVisible();
    await expect(runNow(page)).toBeDisabled();
    await expectAccessibleAndContained(page);

    // The mock walks the run one step per read of it (every 2 s)
    await expect(progress).toBeHidden({ timeout: 30_000 });
    await expect(scheduleHelp(page)).toContainText(/the last one took \d+ s and cost \$0\.36\./);
    await expect(page.getByRole('img', { name: /^Spent \$3\.48 of \$15\.00/ })).toBeVisible();
    await expect(runNow(page)).toBeEnabled();
  });

  test('For You’s “Change schedule” opens this section', async ({ page }) => {
    await signIn(page, '/');
    await page.getByRole('link', { name: 'Change schedule' }).click();
    await page.waitForURL((url) => url.pathname === '/settings' && url.searchParams.get('section') === 'ai');
    await expect(page.getByRole('heading', { name: 'AI & picks', level: 2 })).toBeVisible();
    await expect(segment(page, 'Nightly')).toBeVisible();
  });

  test('Run now’s Watch goes to For You', async ({ page }) => {
    await openSection(page);
    await runNow(page).click();
    await page.getByRole('button', { name: 'Watch' }).click();
    await page.waitForURL((url) => url.pathname === '/');
  });

  test('Run now with picks off says so; switching them on and saving lets it run', async ({ page }) => {
    await scenario(page, 'recs-off');
    await openSection(page);
    const master = page.getByRole('switch', { name: 'Write personal picks' });
    await expect(master).not.toBeChecked();
    await expect(page.getByText('Off: no shelves on For You and no runs, scheduled or by hand. Ask momoshtrem still works.')).toBeVisible();
    await expect(scheduleHelp(page)).toContainText('Picks are off, so nothing runs');
    await runNow(page).click();
    await expect(picks(page).getByRole('alert')).toContainText('Personal picks are off');

    await master.check();
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(picks(page).getByRole('alert')).toBeHidden();
    await expect(scheduleHelp(page)).toContainText('Next run tonight');
    await runNow(page).click();
    await expect(page.getByRole('status').filter({ hasText: 'About a minute' })).toContainText('Writing your picks');
  });

  test('the nav reads Picks off while they are', async ({ page }) => {
    await scenario(page, 'recs-off');
    await openSection(page);
    await expect((await sectionNav(page)).getByRole('link', { name: /AI & picks/ })).toContainText('Picks off · $3.12 of $15.00');
  });

  test('Run now without enough Trakt history points to Trakt', async ({ page }) => {
    await scenario(page, 'trakt-off');
    await openSection(page);
    await runNow(page).click();
    const alert = picks(page).getByRole('alert');
    await expect(alert).toContainText('Not enough history yet');
    await expect(alert.getByRole('link', { name: 'Trakt settings' })).toHaveAttribute('href', '/settings?section=trakt');
  });

  test('a spent budget: Run now’s 429, the spend says so, and the nav flags it', async ({ page }) => {
    await scenario(page, 'ask-budget');
    await openSection(page);
    await expect(page.getByText(/^This month’s budget is used up: AI is paused until [A-Z][a-z]{2} 1 and mapping runs on rules\.$/)).toBeVisible();
    await runNow(page).click();
    const alert = picks(page).getByRole('alert');
    await expect(alert).toContainText('This month’s budget is used up');
    await expect(alert).toContainText(/AI is paused until [A-Z][a-z]{2} 1\. Raise the budget below to run now\./);
    const link = (await sectionNav(page)).getByRole('link', { name: /AI & picks/ });
    await expect(link).toContainText('Nightly · $5.00 of $5.00');
    await expect(link.getByRole('img', { name: 'Needs attention' })).toBeVisible();
  });

  test('the budget: what it can’t send stays here, on the field, and empty is no cap', async ({ page }) => {
    await openSection(page);
    let puts = 0;
    page.on('request', (request) => {
      if (request.method() === 'PUT' && request.url().endsWith('/api/settings/ai')) puts += 1;
    });

    await budgetInput(page).fill('ten');
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Budget” above.');
    await expect(budgetInput(page)).toHaveAccessibleDescription(/^Enter an amount in dollars, like 15 or 7\.50, or leave it empty for no cap\./);
    await expect(budgetInput(page)).toBeFocused();

    await budgetInput(page).fill('-5');
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(budgetInput(page)).toHaveAttribute('aria-invalid', 'true');
    await expect(budgetInput(page)).toHaveAccessibleDescription(/^The budget can’t be negative\. Enter 0 or more, or leave it empty for no cap\./);
    await expect(budgetInput(page)).toBeFocused();
    await expect(budgetInput(page)).toBeInViewport();
    await expectAccessibleAndContained(page);
    expect(puts).toBe(0);

    await budgetInput(page).fill('');
    await expect(budgetInput(page)).not.toHaveAttribute('aria-invalid');
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(page.getByRole('img', { name: /^Spent \$3\.12 this month, no cap: / })).toBeVisible();
    await expect(page.getByText('There’s no cap, so AI never pauses.', { exact: false })).toBeVisible();
    await expect((await sectionNav(page)).getByRole('link', { name: /AI & picks/ })).toContainText('Nightly · $3.12 this month');
  });

  test('a 422 on the budget shows on the field in plain words, and focus moves there', async ({ page }) => {
    await scenario(page, 'ai-budget-invalid');
    await openSection(page);
    await budgetInput(page).fill('20');
    const save = saveBar(page).getByRole('button', { name: 'Save' });
    await save.click();
    await expect(saveBar(page).getByRole('alert')).toHaveText('Check “Budget” above.');
    await expect(budgetInput(page)).toHaveAttribute('aria-invalid', 'true');
    await expect(budgetInput(page)).toHaveAccessibleDescription(/^momoshtrem didn’t accept that budget\. Enter 0 or more, or leave it empty for no cap\./);
    await expect(page.getByText('monthly_budget_usd', { exact: false })).toHaveCount(0);
    await expect(budgetInput(page)).toBeFocused();
    await expect(budgetInput(page)).toBeInViewport();
    await expectAccessibleAndContained(page);
  });

  test('without an Anthropic key (ai-off): picks are off, the classifier still saves', async ({ page }) => {
    await scenario(page, 'ai-off');
    await openSection(page);
    await expect(page.getByText('No Anthropic key on this server')).toBeVisible();
    await expect(runNow(page)).toBeDisabled();
    await expect(page.getByRole('switch', { name: 'Write personal picks' })).toBeDisabled();
    await expect(segment(page, 'Weekly')).toBeDisabled();
    await expect(page.getByRole('radio', { name: 'Sonnet 5.5' })).toBeDisabled();
    await expect(budgetInput(page)).toBeEnabled();
    await expectAccessibleAndContained(page);

    await page.getByRole('switch', { name: 'Auto-apply high-confidence mappings' }).uncheck();
    const put = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/api/settings/ai'));
    await saveBar(page).getByRole('button', { name: 'Save' }).click();
    expect((await put).postDataJSON()).toEqual({ auto_apply_high_confidence: false });
    await expect(page.getByText('Settings saved')).toBeVisible();
    await expect(page.getByRole('switch', { name: 'Auto-apply high-confidence mappings' })).not.toBeChecked();
    await expect((await sectionNav(page)).getByRole('link', { name: /AI & picks/ })).toContainText('No Anthropic key');
  });
});

/** PR screenshots into docs/screenshots/v2/settings, as e2e/screenshots.spec.ts takes them. Set SCREENSHOTS=1. */
test.describe('screenshots', () => {
  test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

  async function shoot(page: Page, testInfo: TestInfo, name: string, { fullPage = false } = {}) {
    const width = testInfo.project.name === 'mobile' ? 390 : 1440;
    await page.evaluate(() => document.fonts.ready);
    if (fullPage) {
      // A viewport as tall as the page, so fixed parts (save bar, tab bar) sit at the bottom as in the design exports
      const height = await page.evaluate(() => document.documentElement.scrollHeight);
      await page.setViewportSize({ width, height });
      await page.waitForTimeout(300);
    }
    await page.screenshot({ path: path.resolve(__dirname, '../../docs/screenshots/v2/settings', `${name}-${width}.png`), animations: 'disabled' });
  }

  test('ai', async ({ page }, testInfo) => {
    await openSection(page);
    await expect(page.getByRole('img', { name: /^Spent / })).toBeVisible();
    await shoot(page, testInfo, 'ai', { fullPage: true });
  });

  test('ai, unsaved changes', async ({ page }, testInfo) => {
    await openSection(page);
    await segment(page, 'Weekly').click();
    await page.getByRole('radio', { name: 'Sonnet 5.5' }).check();
    await expect(saveBar(page)).toBeVisible();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'ai-dirty');
  });

  test('ai, a run writing', async ({ page }, testInfo) => {
    await scenario(page, 'recs-refreshing');
    await openSection(page);
    await expect(picks(page).getByRole('status')).toContainText('Writing your shelves');
    await shoot(page, testInfo, 'ai-running');
  });

  test('ai, no key', async ({ page }, testInfo) => {
    await scenario(page, 'ai-off');
    await openSection(page);
    await shoot(page, testInfo, 'ai-off', { fullPage: true });
  });
});
