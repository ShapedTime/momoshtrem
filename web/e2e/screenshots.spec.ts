import path from 'node:path';
import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { E2E_USER, signIn } from './helpers/auth';

/**
 * PR screenshots at the design's check widths (390 phone, 1440 desktop),
 * written to docs/screenshots/v2/. Skipped in normal runs:
 *   SCREENSHOTS=1 npx playwright test screenshots && node scripts/optimize-screenshots.mjs
 */
test.skip(!process.env.SCREENSHOTS, 'Set SCREENSHOTS=1 to capture PR screenshots');

const OUT = path.resolve(__dirname, '../../docs/screenshots/v2');

async function shoot(page: Page, testInfo: TestInfo, name: string, { fullPage = false } = {}) {
  const width = testInfo.project.name === 'mobile' ? 390 : 1440;
  await page.evaluate(() => document.fonts.ready);
  if (fullPage) {
    // A viewport as tall as the page, so fixed parts (dock, tab bar) sit at the bottom as in the design exports
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    await page.setViewportSize({ width, height });
    await page.waitForTimeout(300);
  }
  await page.screenshot({ path: path.join(OUT, `${name}-${width}.png`), animations: 'disabled' });
}

test.describe('sign in', () => {
  test('default', async ({ page }, testInfo) => {
    await page.goto('/login');
    await shoot(page, testInfo, 'signin/default');
  });

  test('error', async ({ page }, testInfo) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill(E2E_USER.username);
    await page.getByLabel('Password', { exact: true }).fill('correct-horse-battery');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText('Wrong username or password')).toBeVisible();
    await shoot(page, testInfo, 'signin/error');
  });

  test('rate limited', async ({ page }, testInfo) => {
    await page.goto('/login?error=rate_limited&retry=272');
    await expect(page.getByText('Too many attempts')).toBeVisible();
    await shoot(page, testInfo, 'signin/ratelimited');
  });
});

test.describe('app shell', () => {
  test('frame', async ({ page }, testInfo) => {
    await signIn(page, '/torrents');
    await expect(page.getByText('Frieren.S02.1080p.WEB-DL.x264-SubsPlease')).toBeVisible();
    await shoot(page, testInfo, 'shell/frame');
  });

  test('account menu', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const scope = testInfo.project.name === 'desktop' ? page.locator('header.topbar') : page.locator('header.phone-bar');
    await scope.getByRole('button', { name: /Account menu/ }).click();
    await page.getByText('Signed in').waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'shell/menu');
  });

  test('command palette', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'Desktop affordance');
    await signIn(page, '/library');
    await page.keyboard.press('ControlOrMeta+k');
    await page.getByRole('dialog', { name: 'Search or jump to' }).waitFor();
    await page.waitForTimeout(300);
    await shoot(page, testInfo, 'shell/palette');
  });

  test('infuse sheet', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.goto('/library?sheet=infuse-setup');
    await page.getByLabel('WebDAV address').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'shell/sheet');
  });
});

test.describe('system states and settings', () => {
  test('settings playback', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=playback');
    await page.locator('code[aria-label="WebDAV address"]').waitFor();
    await shoot(page, testInfo, 'settings/playback');
  });

  test('settings status', async ({ page }, testInfo) => {
    await signIn(page, '/settings?section=status');
    await page.getByRole('list', { name: 'Services' }).waitFor();
    await shoot(page, testInfo, 'settings/status');
  });

  test('settings list (phone)', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'mobile', 'Phone only');
    await signIn(page, '/settings');
    await page.getByText('WebDAV ready').waitFor();
    await shoot(page, testInfo, 'settings/list');
  });

  test('not found', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.goto('/no/such/page');
    await page.getByRole('heading', { name: 'Nothing here but the cat' }).waitFor();
    await shoot(page, testInfo, 'system/notfound');
  });

  test('offline', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'backend-down'));
    await signIn(page, '/library');
    await page.goto('/offline?from=%2Flibrary');
    await page.getByText(/Trying again in/).waitFor();
    await shoot(page, testInfo, 'system/offline');
  });

  test('offline banner', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'backend-down'));
    await signIn(page, '/library');
    await page.getByText('Can’t reach momoshtrem').waitFor();
    await shoot(page, testInfo, 'system/banner');
  });
});

test.describe('movie page', () => {
  for (const [state, tmdbId, panel] of [
    ['none', 666277, 'Not in your library'],
    ['needs_torrent', 1054867, 'In your library · no torrent yet'],
    ['downloading', 157336, 'Downloading'],
  ] as const) {
    test(state, async ({ page }, testInfo) => {
      await signIn(page, `/movie/${tmdbId}`);
      await page.getByRole('heading', { name: panel }).waitFor();
      await shoot(page, testInfo, `movie/${state}`, { fullPage: true });
    });
  }

  test('ready', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    const subtitles = page.getByRole('region', { name: 'Subtitles' });
    await subtitles.getByRole('button', { name: 'Search' }).click();
    await subtitles.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' }).waitFor();
    await shoot(page, testInfo, 'movie/ready', { fullPage: true });
  });

  test('how to play', async ({ page }, testInfo) => {
    await signIn(page, '/movie/693134');
    await page.getByRole('heading', { name: 'Ready in Infuse' }).waitFor();
    await page.getByRole('button', { name: testInfo.project.name === 'mobile' ? 'Play in Infuse' : 'How to play in Infuse' }).click();
    await page.getByLabel('WebDAV path of this title').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'movie/howto');
  });
});

test.describe('show page', () => {
  for (const [state, path, ready] of [
    ['frieren', '/tv/209867', 'Episodes 29–38 have no torrent'],
    ['severance', '/tv/95396', 'Ready in Infuse'],
    ['specials', '/tv/57243?season=0', 'Ready in Infuse'],
    ['not-in-library', '/tv/66573', 'Not in your library'],
    ['onepiece', '/tv/37854', 'Ready in Infuse'],
    ['rich', '/tv/209867?view=rich', 'Episodes 29–38 have no torrent'],
  ] as const) {
    test(state, async ({ page }, testInfo) => {
      await signIn(page, path);
      await page.getByRole('heading', { name: ready }).waitFor();
      if (state !== 'not-in-library') await page.locator('.ep-list').waitFor();
      await shoot(page, testInfo, `show/${state}`, { fullPage: true });
    });
  }

  test('settings', async ({ page }, testInfo) => {
    await signIn(page, '/tv/209867?sheet=show-settings&tmdb=209867');
    await page.getByRole('radio', { name: /Absolute \(anime\)/ }).waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'show/settings');
  });
});

test.describe('mapping review', () => {
  const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';

  test('progress', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'job-hold:classifying'));
    await signIn(page, '/tv/209867');
    // A new job, opened without a reload so the browser's mock keeps it
    const jobId = await page.evaluate(async () => {
      const response = await fetch('/api/shows/1/assign-torrent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ magnet_uri: 'magnet:?xt=urn:btih:aaaabbbbccccddddeeeeffff0000111122223333', indexer: 'Nyaa' }),
      });
      return ((await response.json()) as { job_id: number }).job_id;
    });
    await page.evaluate((id) => (window as unknown as { next: { router: { push: (href: string) => void } } }).next.router.push(`/mapping/${id}`), jobId);
    await page.getByText('16 files · 13 videos').waitFor();
    await shoot(page, testInfo, 'mapping/progress', { fullPage: true });
  });

  test('review', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    await page.getByRole('button', { name: /^Needs review/ }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'mapping/review', { fullPage: true });
  });

  test('picker', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    if (phone(testInfo)) {
      await page.getByRole('button', { name: 'Change episode for #35v2' }).click();
      await page.getByRole('dialog', { name: 'Choose an episode' }).getByRole('option', { name: /S01E35/ }).waitFor();
    } else {
      await page.getByRole('button', { name: 'Change episode for #28.5' }).click();
      await page.getByRole('searchbox', { name: 'Search episodes' }).fill('recap');
    }
    await page.waitForTimeout(700);
    await shoot(page, testInfo, 'mapping/picker');
  });

  test('conflict', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/103');
    await page.getByText('Two files point at the same episode. Keep one.').waitFor();
    await shoot(page, testInfo, 'mapping/conflict', { fullPage: true });
  });

  test('applied', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    await page.getByRole('button', { name: 'Accept all 2' }).click();
    await page.getByText('1 file stays unmapped. You can map it later.').waitFor();
    await page.getByRole('button', { name: 'Apply 12 mappings' }).click();
    await page.getByRole('heading', { name: 'Ready in Infuse.' }).waitFor();
    await page.waitForTimeout(900);
    await shoot(page, testInfo, 'mapping/applied');
  });

  test('recheck: open', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    const panel = page.getByRole('region', { name: 'Re-check with AI' });
    await panel.getByRole('button', { name: phone(testInfo) ? 'Re-check' : 'Re-check with AI', exact: true }).click();
    await panel.getByRole('button', { name: '28.5 is a recap' }).click();
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'mapping/recheck-open', { fullPage: true });
  });

  test('recheck: working', async ({ page }, testInfo) => {
    // As the design: a hinted re-check of the three unresolved files, held while it works
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'job-hold:resolving'));
    await signIn(page, '/mapping/101');
    const panel = page.getByRole('region', { name: 'Re-check with AI' });
    await panel.getByRole('button', { name: phone(testInfo) ? 'Re-check' : 'Re-check with AI', exact: true }).click();
    await panel.getByRole('button', { name: '28.5 is a recap' }).click();
    await panel.locator('.map-ai-run').getByRole('button', { name: 'Re-check' }).click();
    await panel.getByText('Re-checking 3 files with your hint…').waitFor();
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'mapping/recheck', { fullPage: true });
  });

  test('onepiece', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/102');
    await page.getByRole('button', { name: /^Needs review/ }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'mapping/onepiece', { fullPage: true });
  });
});
