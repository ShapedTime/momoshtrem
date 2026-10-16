import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Ask momoshtrem (`/ask`, `/ask/{thread}`; PLAN F5, #156) against the
 * stateful recommendations mock (src/mocks/fixtures/recommendations.ts):
 * the design's four past conversations (12: Severance, refined to Lighter;
 * 11: a thriller; 10: Frieren; 9: parents), a new one gets id 13, and asks
 * stream the design's sets. Scenarios: ask-budget, ask-limit, ask-error,
 * ask-hold (never finishes, for Stop), ai-off, recs-off.
 */

const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';
const SEVERANCE = 'something like Severance but lighter';

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
const answers = (page: Page) => page.getByRole('article', { name: 'Noir’s answer' });
const cards = (answer: ReturnType<typeof answers>) => answer.locator('article.ask-card');
const card = (page: Page, title: string) => page.locator('article.ask-card').filter({ has: page.getByRole('heading', { name: title, exact: true }) });
const field = (page: Page) => page.getByRole('textbox', { name: 'Ask momoshtrem' });
const announcer = (page: Page) => page.locator('.ask-main > [role="status"]');
const rail = (page: Page) => page.getByRole('complementary', { name: 'Conversations' });

test.describe('Ask momoshtrem', () => {
  test('ask → named steps → cards → refine', async ({ page }, testInfo) => {
    await signIn(page, '/ask');
    await expect(page).toHaveTitle('Ask momoshtrem · momoshtrem');
    await expect(page.getByRole('heading', { name: 'What are we in the mood for?', level: 1 })).toBeVisible();
    await expect(page.getByRole('list', { name: 'Example asks' }).getByRole('link')).toHaveCount(6);
    // No tab bar on phone: the page has its own header
    await expect(page.locator('nav.tabbar')).toBeHidden();
    await expectAccessibleAndContained(page);

    await page.getByRole('link', { name: SEVERANCE, exact: true }).click();
    // The new thread's `start` moves the URL without leaving the page
    await expect(page).toHaveURL(/\/ask\/13$/);
    const first = answers(page).first();
    await expect(first.locator('.ask-steps .step.is-done').first()).toContainText('Read your history');
    await expect(first.locator('.ask-steps .step.is-done').first()).toContainText('2,184 plays');
    await expect(announcer(page)).toHaveText(/…$/);

    await expect(announcer(page)).toHaveText('Noir found 5 picks.', { timeout: 20_000 });
    await expect(cards(first)).toHaveCount(5);
    await expect(first.locator('.ask-trace summary')).toHaveText('History, web and TMDB checked · 9 s');
    await expect(first.getByText('Five picks with the same “what is this place, really?” itch as Severance')).toBeVisible();
    const goodPlace = card(page, 'The Good Place');
    await expect(goodPlace).toContainText('Mystery + workplace · lighter');
    await expect(goodPlace.locator('.ask-meta')).toHaveText(/2016–2020\s*·?\s*4 seasons\s*·?\s*22 min\s*·?\s*8\.1/);
    await expect(goodPlace.getByRole('button', { name: 'Add to library' })).toBeVisible();
    await expect(goodPlace.getByRole('link', { name: 'Details' })).toHaveAttribute('href', '/tv/66573');
    if (isDesktop(testInfo)) {
      await expect(rail(page).getByRole('link', { name: /^Something like Severance but lighter/ }).first()).toHaveAttribute('aria-current', 'page');
    }
    // The trace opens to the steps and their tallies
    await first.locator('.ask-trace summary').click();
    await expect(first.locator('.ask-trace .step')).toHaveText([/Read your history/, /Searched the web for new releases/, /Checked TMDB/, /Wrote reasons\s*5 picks/]);
    await expectAccessibleAndContained(page);

    // Refine: the chip is the next message, and it re-ranks
    await page.getByRole('group', { name: 'Refine these picks' }).getByRole('button', { name: 'Lighter' }).click();
    await expect(page.locator('.msg--me')).toHaveText([`You asked: ${SEVERANCE}`, 'You asked: Lighter']);
    const second = answers(page).nth(1);
    await expect(second.locator('.ask-steps .step')).toHaveText([/Re-rank/, /Writ/]);
    await expect(announcer(page)).toHaveText('Noir found 3 picks.', { timeout: 20_000 });
    await expect(second.locator('.ask-trace summary')).toHaveText('Re-ranked your picks · 4 s');
    await expect(cards(second).getByRole('heading')).toHaveText(['Mythic Quest', 'The Good Place', 'Bluey']);
    // A title in your library: its pill and Open
    const bluey = cards(second).filter({ hasText: 'Bluey' });
    await expect(bluey.locator('.pill')).toHaveText('Ready');
    await expect(bluey.getByRole('link', { name: 'Open' })).toHaveAttribute('href', '/tv/82728');
    // Only the newest answer offers chips
    await expect(page.getByRole('group', { name: 'Refine these picks' })).toHaveCount(1);
  });

  test('Add to library, then Find a torrent opens season packs (flow 2)', async ({ page }) => {
    await signIn(page, `/ask?q=${encodeURIComponent(SEVERANCE)}`);
    await expect(page).toHaveURL(/\/ask\/13$/);
    const goodPlace = card(page, 'The Good Place');
    await goodPlace.getByRole('button', { name: 'Add to library' }).click();
    // For You's add (useAddRecTitle): the same toast
    await expect(page.getByRole('status').filter({ hasText: 'Added to library' })).toContainText('The Good Place · next, find a torrent');
    await expect(goodPlace.locator('.pill')).toHaveText('In library');
    await goodPlace.getByRole('button', { name: 'Find a torrent' }).click();
    await expect(page).toHaveURL(/\/ask\/13\?sheet=torrent&show=66573&season=1$/);
    const search = page.getByRole('dialog', { name: /^Find a source · The Good Place Season 1/ });
    await expect(search.getByRole('button', { name: 'Season pack', exact: true })).toHaveAttribute('aria-pressed', 'true');
    // Closing the sheet keeps the conversation
    await page.keyboard.press('Escape');
    await expect(search).toBeHidden();
    await expect(goodPlace.getByRole('button', { name: 'Find a torrent' })).toBeVisible();
  });

  test('feedback on a pick: 👍 toggles, 👎 dims it with Undo, and the "…" sheet adds it to the library', async ({ page }) => {
    await signIn(page, '/ask/12');
    const goodPlace = cards(answers(page).first()).filter({ has: page.getByRole('heading', { name: 'The Good Place', exact: true }) });
    const row = goodPlace.getByRole('group', { name: 'Feedback on The Good Place' });
    await row.getByRole('button', { name: 'More like this' }).click();
    await expect(row.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');
    await expect(goodPlace).not.toHaveClass(/is-dimmed/);

    // Every card of the same title agrees (the Lighter answer has The Good Place too)
    const again = cards(answers(page).nth(1)).filter({ has: page.getByRole('heading', { name: 'The Good Place', exact: true }) });
    await expect(again.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');

    const mythic = cards(answers(page).first()).filter({ has: page.getByRole('heading', { name: 'Mythic Quest', exact: true }) });
    await mythic.getByRole('button', { name: 'Less like this' }).click();
    await expect(mythic).toHaveClass(/is-dimmed/);
    const toast = page.getByRole('status').filter({ hasText: 'Got it, fewer like this' });
    await expect(toast).toContainText('Mythic Quest won’t be picked again.');
    await toast.getByRole('button', { name: 'Undo' }).click();
    await expect(mythic).not.toHaveClass(/is-dimmed/);

    // The "…" sheet's Add to library turns the card into In library + Find a torrent
    await goodPlace.getByRole('button', { name: 'More feedback' }).click();
    const sheet = page.getByRole('dialog', { name: 'The Good Place' });
    await sheet.getByRole('button', { name: 'Add to library' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Added to library' })).toBeVisible();
    await expect(goodPlace.locator('.pill')).toHaveText('In library');
    await expect(goodPlace.getByRole('button', { name: 'Find a torrent' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('Stop cancels the answer, and the next ask goes through', async ({ page }, testInfo) => {
    await scenario(page, 'ask-hold');
    await signIn(page, `/ask?q=${encodeURIComponent(SEVERANCE)}`);
    const first = answers(page).first();
    await expect(first.locator('.ask-steps .step.is-active')).toHaveText(/Searching the web for new releases…/);
    await expect(announcer(page)).toHaveText('Searching the web for new releases…');
    if (isDesktop(testInfo)) await page.keyboard.press('Escape');
    else await page.getByRole('button', { name: 'Stop' }).click();
    await expect(first.getByText('You stopped this answer.')).toBeVisible();
    await expect(announcer(page)).toHaveText('Stopped.');
    // An ended turn never spins: only the finished step stays, in the trace
    await expect(first.locator('.ask-trace summary')).toHaveText('History checked');
    await expect(page.getByRole('button', { name: 'Send' })).toBeVisible();

    // Shift+Enter adds a line, Enter sends
    await field(page).fill('a 90-minute thriller');
    await field(page).press('Shift+Enter');
    await field(page).pressSequentially('for tonight');
    await expect(field(page)).toHaveValue('a 90-minute thriller\nfor tonight');
    await field(page).press('Enter');
    await expect(field(page)).toHaveValue('');
    await expect(page.locator('.msg--me').nth(1)).toHaveText('You asked: a 90-minute thriller\nfor tonight');
    const second = answers(page).nth(1);
    await expect(second.locator('.ask-steps .step.is-active')).toHaveText(/Searching the web/);
    await expect(second.locator('.ask-error')).toHaveCount(0);
  });

  test('budget reached: the design’s copy, the reset date, and the composer off', async ({ page }, testInfo) => {
    await scenario(page, 'ask-budget');
    await signIn(page, `/ask?q=${encodeURIComponent('a 90-minute thriller for tonight')}`);
    const error = answers(page).first().locator('.ask-error');
    await expect(error.getByRole('heading', { name: 'This month’s AI budget is used up' })).toBeVisible();
    await expect(error).toContainText(/You set \$5\.00 for \w+ and it’s all spent, so asks and new picks pause until \w{3} 1\. Everything else keeps working/);
    await expect(error.getByRole('link', { name: 'Settings › AI' })).toHaveAttribute('href', '/settings?section=ai');
    await expect(error.getByRole('link', { name: 'Browse Discover instead' })).toHaveAttribute('href', '/discover');
    await expect(field(page)).toBeDisabled();
    await expect(field(page)).toHaveAttribute('placeholder', /^Asks are paused until \w{3} 1$/);
    // The URL stays /ask: a refused ask starts no thread
    await expect(page).toHaveURL(/\/ask\?q=/);
    if (isDesktop(testInfo)) await expect(rail(page).locator('.ask-budget')).toContainText('$5.00 of $5.00');
    else await expect(page.locator('.ask-bar')).toContainText(/Paused until \w{3} 1/);
    await expectAccessibleAndContained(page);
  });

  test('daily limit, and a mid-stream error with Try again', async ({ page }) => {
    await scenario(page, 'ask-limit');
    await signIn(page, `/ask?q=${encodeURIComponent('a 90-minute thriller for tonight')}`);
    const limit = answers(page).first().locator('.ask-error');
    await expect(limit.getByRole('heading', { name: 'That’s all the asks for today' })).toBeVisible();
    await expect(limit).toContainText('so asks pause until midnight');
    await expect(field(page)).toHaveAttribute('placeholder', 'Asks are paused until midnight');

    await scenario(page, 'ask-error');
    await page.goto(`/ask?q=${encodeURIComponent('a 90-minute thriller for tonight')}`);
    const failed = answers(page).first();
    await expect(failed.locator('.ask-error').getByRole('heading', { name: 'Claude didn’t answer' })).toBeVisible({ timeout: 20_000 });
    await expect(failed.locator('.ask-error')).toContainText('The Anthropic API didn’t answer. Try again in a minute.');
    await expect(failed.locator('.ask-trace summary')).toHaveText('History checked');
    await expect(announcer(page)).toHaveText('Claude didn’t answer');
    await expect(field(page)).toBeEnabled();
    // Try again asks once more; the failed answer stays in the thread, without its button
    await failed.getByRole('button', { name: 'Try again' }).click();
    await expect(page.locator('.msg--me')).toHaveCount(2);
    await expect(failed.getByRole('button', { name: 'Try again' })).toHaveCount(0);
    await expect(answers(page).nth(1).locator('.ask-error').getByRole('heading', { name: 'Claude didn’t answer' })).toBeVisible({ timeout: 20_000 });
  });

  test('reopening a conversation reads its saved turns', async ({ page }, testInfo) => {
    await signIn(page, '/ask/12');
    await expect(page).toHaveTitle('Something like Severance but lighter · momoshtrem');
    await expect(page.locator('.msg--me')).toHaveText([`You asked: ${SEVERANCE}`, 'You asked: Lighter']);
    await expect(answers(page).locator('.ask-trace summary')).toHaveText(['History, web and TMDB checked · 9 s', 'Re-ranked your picks · 4 s']);
    await expect(cards(answers(page).first())).toHaveCount(5);
    await expect(cards(answers(page).nth(1))).toHaveCount(3);
    await expect(page.getByRole('group', { name: 'Refine these picks' })).toHaveCount(1);
    await expectAccessibleAndContained(page);

    // Another conversation: the rail on desktop, Past conversations on phone
    if (isDesktop(testInfo)) {
      await expect(rail(page).getByRole('link', { name: /^Something like Severance/ })).toHaveAttribute('aria-current', 'page');
      await rail(page).getByRole('link', { name: /^A 90-minute thriller for tonight/ }).click();
    } else {
      await page.getByRole('button', { name: 'Past conversations' }).click();
      const sheet = page.getByRole('dialog', { name: 'Conversations' });
      await expect(sheet.getByRole('link', { name: /^Something like Severance/ })).toHaveAttribute('aria-current', 'page');
      await expect(sheet.locator('.ask-budget')).toContainText('AI this month');
      await sheet.getByRole('link', { name: /^A 90-minute thriller for tonight/ }).click();
    }
    await expect(page).toHaveURL(/\/ask\/11$/);
    await expect(cards(answers(page).first()).getByRole('heading')).toHaveText(['Paprika', 'Arrival', 'Slow Horses']);

    // A thread that isn't there
    await page.goto('/ask/999');
    await expect(page.getByRole('heading', { name: 'This conversation isn’t here' })).toBeVisible();
  });
});

test.describe('Ask entry points', () => {
  test('the top-bar pill, the palette, Search and Discover', async ({ page }, testInfo) => {
    // Five pages, then an answer streams
    test.slow();
    await signIn(page, '/search');
    if (isDesktop(testInfo)) {
      await expect(page.locator('header.topbar').getByRole('link', { name: 'Ask momoshtrem' })).toHaveAttribute('href', '/ask');
    }
    // Search: the idle chip, the "Not quite it?" card, and the empty state's button
    await expect(page.locator('.srch-moods').getByRole('link', { name: 'Ask momoshtrem' })).toHaveAttribute('href', '/ask');
    await page.goto('/search?q=dune');
    await expect(page.getByRole('link', { name: /Not quite it\?\s*Ask momoshtrem about “dune”/ })).toHaveAttribute('href', '/ask?q=dune');
    await page.goto('/search?q=zzxq');
    await expect(page.locator('.srch-none .empty').getByRole('link', { name: 'Ask momoshtrem' })).toHaveAttribute('href', '/ask?q=zzxq');

    // Discover's empty state asks for the filters in words
    await page.goto('/discover?type=tv&anime=1&genres=comedy&from=2020&rating=8&status=ended');
    await expect(page.locator('.disc-empty').getByRole('link', { name: 'Ask momoshtrem' })).toHaveAttribute(
      'href',
      `/ask?q=${encodeURIComponent('an anime comedy show from the 2020s, rated 8+, that has finished airing')}`
    );

    // The palette: "Ask momoshtrem: “q”" is always there while typing, and asks at once
    await page.goto('/library');
    const palette = page.getByRole('dialog', { name: 'Search or jump to' });
    if (isDesktop(testInfo)) {
      // ⌘K works once the shell has hydrated
      await expect(async () => {
        await page.keyboard.press('ControlOrMeta+k');
        await expect(palette).toBeVisible({ timeout: 1000 });
      }).toPass();
    } else {
      await page.locator('header.phone-bar').getByRole('button', { name: /Account menu/ }).click();
      await page.getByRole('dialog', { name: 'Account' }).getByRole('button', { name: 'Command palette' }).click();
    }
    await expect(palette.getByRole('option', { name: /^Ask momoshtrem\s*Conversational picks/ })).toBeVisible();
    await page.keyboard.type('anime with a Frieren vibe');
    const askRow = palette.getByRole('option', { name: /^Ask momoshtrem: “anime with a Frieren vibe”/ });
    await expect(askRow).toContainText('Picks with a reason for each');
    await askRow.click();
    await expect(page).toHaveURL(/\/ask\/13$/);
    await expect(cards(answers(page).first()).getByRole('heading')).toHaveText(['Mushishi', 'My Neighbor Totoro', 'Dan Da Dan', 'Paprika'], { timeout: 20_000 });
  });

  test('without an Anthropic key every entry hides, and /ask says what it needs', async ({ page }, testInfo) => {
    await scenario(page, 'ai-off');
    await signIn(page, '/search?q=dune');
    await expect(page.getByRole('heading', { name: 'In your library' })).toBeVisible();
    await expect(page.getByRole('link', { name: /Not quite it\?/ })).toHaveCount(0);
    await expect(page.locator('header.topbar').getByRole('link', { name: 'Ask momoshtrem' })).toHaveCount(0);
    await page.goto('/search');
    await expect(page.getByRole('heading', { name: 'Not sure what you want?' })).toBeVisible();
    await expect(page.locator('.srch-moods').getByRole('link', { name: 'Ask momoshtrem' })).toHaveCount(0);
    await page.goto('/discover?type=tv&anime=1&genres=comedy&from=2020&rating=8&status=ended');
    await expect(page.getByRole('heading', { name: 'No shows match all 5 filters' })).toBeVisible();
    await expect(page.locator('.disc-empty').getByRole('link', { name: 'Ask momoshtrem' })).toHaveCount(0);
    if (isDesktop(testInfo)) {
      await expect(async () => {
        await page.keyboard.press('ControlOrMeta+k');
        await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeVisible({ timeout: 1000 });
      }).toPass();
      await page.keyboard.type('cosy');
      await expect(page.getByRole('option', { name: /Search all results for “cosy”/ })).toBeVisible();
      await expect(page.getByRole('option', { name: /^Ask momoshtrem/ })).toHaveCount(0);
      await page.keyboard.press('Escape');
    }

    await page.goto('/ask');
    await expect(page.getByRole('heading', { name: 'Ask momoshtrem needs an Anthropic API key' })).toBeVisible();
    await expect(field(page)).toBeDisabled();
  });

  test('with picks switched off, Ask still works', async ({ page }, testInfo) => {
    await scenario(page, 'recs-off');
    await signIn(page, '/search?q=dune');
    await expect(page.getByRole('link', { name: /Not quite it\?/ })).toBeVisible();
    if (isDesktop(testInfo)) await expect(page.locator('header.topbar').getByRole('link', { name: 'Ask momoshtrem' })).toBeVisible();
    await page.getByRole('link', { name: /Not quite it\?/ }).click();
    await expect(page).toHaveURL(/\/ask\/13$/);
    await expect(cards(answers(page).first())).toHaveCount(5, { timeout: 20_000 });
  });
});
