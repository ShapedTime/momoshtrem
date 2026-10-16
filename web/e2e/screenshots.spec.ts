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
    await signIn(page, '/activity');
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

test.describe('HDRezka sheet', () => {
  const BEST = 'Use Битва за битвой (2025)';
  const open = async (page: Page) => {
    await signIn(page, '/movie/1054867?sheet=hdrezka&movie=1054867');
    await page.getByRole('dialog', { name: /^Find a source/ }).waitFor();
  };
  const settle = async (page: Page) => {
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(600);
  };

  test('match', async ({ page }, testInfo) => {
    await open(page);
    await page.getByRole('button', { name: BEST }).waitFor();
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/match');
  });

  test('voice-over and quality', async ({ page }, testInfo) => {
    await open(page);
    await page.getByRole('button', { name: BEST }).click();
    await page.getByRole('radio', { name: 'Оригинал (+субтитры)' }).check();
    await page.getByText('Subtitles: English, Русский').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/steps');
  });

  test('none', async ({ page }, testInfo) => {
    await open(page);
    await page.getByRole('button', { name: BEST }).waitFor();
    await page.getByRole('searchbox', { name: 'Search HDRezka, or paste a link' }).fill('zzz Battle 2025');
    await page.keyboard.press('Enter');
    await page.getByText('No match for “zzz Battle 2025”').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/none');
  });

  test('blocked', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'hdrezka-blocked'));
    await open(page);
    await page.getByText('HDRezka is showing its bot check').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/blocked');
  });

  test('show: episodes', async ({ page }, testInfo) => {
    await signIn(page, '/tv/209867?sheet=hdrezka&show=209867&season=1');
    await page.getByRole('button', { name: 'Use Провожающая в последний путь Фрирен (2023)' }).click();
    await page.getByText('10 episodes selected').waitFor();
    await page.getByRole('radio', { name: '1080p, 640 MB an episode' }).waitFor();
    await page.getByRole('heading', { name: 'Episodes', exact: true }).evaluate((heading) => heading.scrollIntoView({ block: 'start' }));
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/show-episodes');
  });

  test('show: seasons', async ({ page }, testInfo) => {
    await signIn(page, '/tv/95396?sheet=hdrezka&show=95396&season=2');
    await page.getByRole('button', { name: 'Use Разделение (2022)' }).click();
    await page.getByRole('button', { name: /^Select all/ }).click();
    await page.getByRole('heading', { name: 'Episodes', exact: true }).evaluate((heading) => heading.scrollIntoView({ block: 'start' }));
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/show-seasons');
  });

  test('show: partial failure', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'hdrezka-partial'));
    await signIn(page, '/tv/209867?sheet=hdrezka&show=209867&season=1');
    await page.getByRole('button', { name: 'Use Провожающая в последний путь Фрирен (2023)' }).click();
    await page.getByRole('radio', { name: '1080p, 640 MB an episode' }).waitFor();
    await page.getByRole('button', { name: 'Stream 10 episodes' }).click();
    const alert = page.getByRole('alert');
    await alert.waitFor();
    await alert.evaluate((banner) => banner.scrollIntoView({ block: 'end' }));
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/show-partial');
  });

  test('episode sheet', async ({ page }, testInfo) => {
    await signIn(page, '/tv/95396?sheet=episode&tmdb=95396&se=S02E05');
    await page.getByText('Source: Torrent').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'hdrezka/episode-sheet');
  });

  test('ready with the stream', async ({ page }, testInfo) => {
    await open(page);
    await page.getByRole('button', { name: BEST }).click();
    await page.getByRole('radio', { name: 'Оригинал (+субтитры)' }).check();
    await page.getByRole('button', { name: 'Use HDRezka' }).click();
    await page.getByRole('heading', { name: 'Current stream' }).waitFor();
    await page.getByRole('status').filter({ hasText: 'Streaming from HDRezka' }).waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'movie/stream', { fullPage: true });
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
    // Watched rows (Trakt), then Up next on S03E04
    ['watched', '/tv/57243', 'Ready in Infuse'],
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

  test('ready to apply: the show’s way back', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    await page.getByRole('button', { name: 'Accept all 2' }).click();
    await page.getByText('1 file stays unmapped. You can map it later.').waitFor();
    await page.getByRole('link', { name: 'Back to Frieren' }).click();
    const banner = page.locator('.banner').filter({ hasText: 'Finish this torrent’s mapping' });
    await banner.waitFor();
    await banner.evaluate((el) => el.scrollIntoView({ block: 'center' }));
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'show/ready-to-apply');
  });

  test('recheck: open', async ({ page }, testInfo) => {
    await signIn(page, '/mapping/101');
    const panel = page.getByRole('region', { name: 'Re-check with AI' });
    await panel.getByRole('button', { name: 'Re-check with AI' }).click();
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
    await panel.getByRole('button', { name: 'Re-check with AI' }).click();
    await panel.getByRole('button', { name: '28.5 is a recap' }).click();
    await panel.locator('.map-ai-run').getByRole('button', { name: 'Re-check' }).click();
    await panel.getByText('Re-checking 3 files with your hint…', { exact: true }).waitFor();
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

test.describe('library', () => {
  const ready = async (page: Page) => {
    await page.locator('.lib-results').waitFor();
    await page.waitForTimeout(500);
  };
  const more = (page: Page, title: string) => page.getByRole('button', { name: `${title}: more actions` });

  test('default', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await ready(page);
    await shoot(page, testInfo, 'library/default', { fullPage: true });
  });

  test('list', async ({ page }, testInfo) => {
    await signIn(page, '/library?view=list');
    await ready(page);
    await shoot(page, testInfo, 'library/list', { fullPage: true });
  });

  test('watching', async ({ page }, testInfo) => {
    await signIn(page, '/library?filter=watching');
    await ready(page);
    await shoot(page, testInfo, 'library/watching', { fullPage: true });
  });

  test('menu', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await ready(page);
    await more(page, 'Frieren: Beyond Journey’s End').click();
    await page.getByText('Copy WebDAV path').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'library/menu');
  });

  test('remove', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await ready(page);
    await more(page, 'Interstellar').click();
    await page.getByText('Remove from library…').click();
    await page.getByRole('alertdialog').waitFor();
    await page.waitForTimeout(500);
    await shoot(page, testInfo, 'library/remove');
  });

  test('undo', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await ready(page);
    await more(page, 'Interstellar').click();
    await page.getByText('Remove from library…').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Remove', exact: true }).click();
    await page.getByText('Undo within 10 seconds').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'library/undo');
  });

  test('empty', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'library-empty'));
    await signIn(page, '/library');
    await page.getByRole('heading', { name: 'Your library is empty' }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'library/empty');
  });

  test('filtered empty', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'library-reviewed'));
    await signIn(page, '/library?filter=review');
    await page.getByRole('heading', { name: 'Nothing needs review' }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'library/filtered-empty');
  });

  test('loading', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'library-loading'));
    await signIn(page, '/library');
    await page.locator('.skel').first().waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'library/loading');
  });
});

test.describe('search', () => {
  const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';
  const recent = (page: Page) =>
    page.addInitScript(() =>
      window.localStorage.setItem('momoshtrem:recent-searches', JSON.stringify(['frieren', 'severance s02', 'past lives', 'wim wenders', 'one piece egghead']))
    );
  const settle = async (page: Page) => {
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(600);
  };
  const palette = async (page: Page, testInfo: TestInfo) => {
    if (phone(testInfo)) {
      await page.locator('header.phone-bar').getByRole('button', { name: /Account menu/ }).click();
      await page.getByRole('dialog', { name: 'Account' }).getByRole('button', { name: 'Command palette' }).click();
    } else {
      await page.keyboard.press('ControlOrMeta+k');
    }
    await page.getByRole('dialog', { name: 'Search or jump to' }).waitFor();
  };

  test('idle', async ({ page }, testInfo) => {
    await recent(page);
    await signIn(page, '/search');
    await page.getByRole('region', { name: 'Trending this week' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'search/idle', { fullPage: true });
  });

  test('results', async ({ page }, testInfo) => {
    await signIn(page, '/search?q=dune');
    await page.locator('.srch-hit article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'search/results', { fullPage: true });
  });

  test('none', async ({ page }, testInfo) => {
    await signIn(page, '/search?q=zzxq');
    await page.getByRole('region', { name: 'Popular right now' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'search/none', { fullPage: true });
  });

  test('palette', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.locator('.lib-results').waitFor();
    await palette(page, testInfo);
    await page.keyboard.type('sev');
    await page.getByRole('option', { name: /^Seven Samurai/ }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'search/palette');
  });

  test('palette, before typing', async ({ page }, testInfo) => {
    await recent(page);
    await signIn(page, '/library');
    await page.locator('.lib-results').waitFor();
    await palette(page, testInfo);
    await page.getByRole('option', { name: /^Go to Activity/ }).waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'search/palette-empty');
  });
});

test.describe('discover', () => {
  const phone = (testInfo: TestInfo) => testInfo.project.name === 'mobile';
  // Tall enough for the first rows, short enough that the infinite-scroll footer stays out of reach
  const tall = (page: Page, testInfo: TestInfo) => page.setViewportSize({ width: phone(testInfo) ? 390 : 1440, height: 1400 });
  const settled = async (page: Page) => {
    await page.locator('.disc-grid article.pcard').first().waitFor();
    await page.waitForTimeout(600);
  };

  test('default', async ({ page }, testInfo) => {
    await tall(page, testInfo);
    await signIn(page, '/discover');
    await settled(page);
    await shoot(page, testInfo, 'discover/default');
  });

  test('filtered', async ({ page }, testInfo) => {
    await signIn(page, '/discover?type=tv&genres=comedy&from=2015&rating=8');
    await settled(page);
    await shoot(page, testInfo, 'discover/filtered', { fullPage: true });
  });

  test('filters sheet', async ({ page }, testInfo) => {
    test.skip(!phone(testInfo), 'The filter sheet is the phone layout');
    await signIn(page, '/discover?type=tv&genres=comedy&from=2015&rating=8');
    await settled(page);
    await page.getByRole('button', { name: /^Filters/ }).click();
    await page.getByRole('dialog', { name: 'Filters' }).getByText('Show 7 shows').waitFor();
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'discover/filters-sheet');
  });

  test('empty', async ({ page }, testInfo) => {
    await signIn(page, '/discover?type=tv&anime=1&genres=comedy&from=2020&rating=8&status=ended');
    await page.getByText('still none').first().waitFor();
    await page.waitForTimeout(400);
    await shoot(page, testInfo, 'discover/empty', { fullPage: true });
  });

  test('loading', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'discover-loading'));
    await signIn(page, '/discover');
    await page.locator('.skel').first().waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'discover/loading');
  });
});

test.describe('activity', () => {
  const SEVERANCE = 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1';
  const ready = async (page: Page) => {
    await page.getByRole('region', { name: 'Summary' }).waitFor();
    await page.waitForTimeout(600);
  };

  test('default', async ({ page }, testInfo) => {
    await signIn(page, '/activity');
    await page.getByText('Frieren.S02.1080p.WEB-DL.x264-SubsPlease').waitFor();
    await ready(page);
    await shoot(page, testInfo, 'activity/default', { fullPage: true });
  });

  test('detail', async ({ page }, testInfo) => {
    await signIn(page, `/activity/${SEVERANCE}?advanced=1`);
    await page.getByRole('img', { name: /Piece map/ }).waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'activity/detail', { fullPage: true });
  });

  test('empty', async ({ page }, testInfo) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'torrents-quiet'));
    await signIn(page, '/activity');
    await page.getByRole('heading', { name: 'Nothing downloading.' }).waitFor();
    await ready(page);
    await shoot(page, testInfo, 'activity/empty', { fullPage: true });
  });

  test('offline', async ({ page }, testInfo) => {
    await signIn(page, '/activity');
    await ready(page);
    await page.evaluate(() => window.localStorage.setItem('msw:scenario', 'torrents-offline'));
    await page.getByRole('alert').filter({ hasText: 'Can’t reach momoshtrem' }).waitFor({ timeout: 20_000 });
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'activity/offline', { fullPage: true });
  });

  test('downloads waiting for the stream', async ({ page }, testInfo) => {
    await signIn(page, '/activity');
    await ready(page);
    // Pause while streaming on: Severance streams, so the kept downloads wait (#77)
    await page.evaluate(() =>
      fetch('/api/settings/storage', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ background: { pause_while_streaming: true } }) })
    );
    await page.getByRole('status').filter({ hasText: 'Downloads wait for the stream to end' }).waitFor({ timeout: 15_000 });
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'activity/waiting', { fullPage: true });
  });

  test('kept torrent detail', async ({ page }, testInfo) => {
    await signIn(page, '/activity/c9d4a7b3e5f60718293a4b5c6d7e8f90a1b2c3d4');
    await page.getByRole('region', { name: 'Kept & smart' }).waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'activity/detail-kept', { fullPage: true });
  });

  test('remove dialog', async ({ page }, testInfo) => {
    await signIn(page, '/activity');
    await ready(page);
    await page.getByRole('button', { name: 'Remove Interstellar torrent' }).click();
    await page.getByRole('alertdialog', { name: 'Remove this torrent?' }).waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'activity/remove');
  });
});

test.describe('for you', () => {
  const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
  const settle = async (page: Page) => {
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(800);
  };

  test('default', async ({ page }, testInfo) => {
    await signIn(page, '/');
    await page.locator('section.home-hero h2.t-display-xl').waitFor();
    await page.getByRole('region', { name: 'Your Trakt watchlist' }).locator('article.pcard').first().waitFor();
    await page.getByRole('region', { name: 'Coming soon' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/default', { fullPage: true });
  });

  test('refreshing', async ({ page }, testInfo) => {
    await scenario(page, 'recs-refreshing');
    await signIn(page, '/');
    await page.locator('section.home-hero h2.t-display-xl').waitFor();
    await page.getByText('Refreshing your picks').waitFor();
    await page.getByRole('region', { name: 'Coming soon' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/refreshing', { fullPage: true });
  });

  test('dismissed', async ({ page }, testInfo) => {
    await signIn(page, '/');
    const shelf = page.getByRole('region', { name: 'Because you loved Severance' });
    const andor = shelf.locator('article.pcard').filter({ hasText: 'Andor' });
    await shelf.scrollIntoViewIfNeeded();
    await andor.getByRole('button', { name: 'Less like this' }).click();
    await page.getByText('Got it, fewer like this').waitFor();
    await page.waitForTimeout(800);
    await shoot(page, testInfo, 'home/dismissed');
  });

  test('feedback sheet', async ({ page }, testInfo) => {
    await signIn(page, '/');
    const silo = page.getByRole('region', { name: 'Because you loved Severance' }).locator('article.pcard').filter({ hasText: 'Silo' });
    await silo.getByRole('button', { name: 'More feedback' }).click();
    await page.getByRole('dialog', { name: 'Silo' }).getByText('Tune your picks').waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'home/feedback');
  });

  test('first run', async ({ page }, testInfo) => {
    await scenario(page, 'recs-first-run');
    await signIn(page, '/');
    await page.getByText('Writing your shelves').waitFor();
    await page.getByRole('region', { name: 'Up next' }).locator('article.lcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/first-run', { fullPage: true });
  });

  test('trakt off', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-off');
    await signIn(page, '/');
    await page.locator('section.home-hero h2.t-display-xl').waitFor();
    await page.getByRole('region', { name: 'Popular movies' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/trakt-off', { fullPage: true });
  });

  test('first sync', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-syncing');
    await signIn(page, '/');
    await page.getByRole('region', { name: 'Up next' }).locator('article.lcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/syncing', { fullPage: true });
  });

  test('first sync stopped', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-sync-error');
    await signIn(page, '/');
    await page.getByText('The first import didn’t finish').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/sync-error', { fullPage: true });
  });

  test('first time', async ({ page }, testInfo) => {
    await scenario(page, 'first-time');
    await signIn(page, '/');
    await page.getByRole('region', { name: 'Trending this week' }).locator('article.pcard').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/empty', { fullPage: true });
  });

  test('loading', async ({ page }, testInfo) => {
    await scenario(page, 'trakt-loading');
    await signIn(page, '/');
    await page.locator('.home-body .skel').first().waitFor();
    await settle(page);
    await shoot(page, testInfo, 'home/loading');
  });
});

test.describe('Ask momoshtrem (#156)', () => {
  const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
  const SEVERANCE = '/ask?q=something%20like%20Severance%20but%20lighter';
  const settle = async (page: Page) => {
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.waitForTimeout(600);
  };

  test('empty', async ({ page }, testInfo) => {
    await signIn(page, '/ask');
    await page.getByRole('list', { name: 'Example asks' }).waitFor();
    await settle(page);
    await shoot(page, testInfo, 'ask/empty');
  });

  test('working', async ({ page }, testInfo) => {
    // As the design's frame: history and the web read, TMDB being checked, the first pick in and a skeleton
    await scenario(page, 'ask-working');
    await signIn(page, SEVERANCE);
    await page.locator('article.ask-card').first().waitFor();
    await page.locator('.ask-card.is-skel').waitFor();
    await settle(page);
    await shoot(page, testInfo, 'ask/working');
  });

  test('results', async ({ page }, testInfo) => {
    await signIn(page, SEVERANCE);
    await page.getByRole('group', { name: 'Refine these picks' }).waitFor({ timeout: 20_000 });
    await settle(page);
    await shoot(page, testInfo, 'ask/results', { fullPage: true });
  });

  test('budget reached', async ({ page }, testInfo) => {
    await scenario(page, 'ask-budget');
    await signIn(page, '/ask?q=a%2090-minute%20thriller%20for%20tonight');
    await page.getByRole('heading', { name: 'This month’s AI budget is used up' }).waitFor();
    await settle(page);
    await shoot(page, testInfo, 'ask/error');
  });

  test('Search: Not quite it?', async ({ page }, testInfo) => {
    await signIn(page, '/search?q=dune');
    await page.getByRole('link', { name: /Not quite it\?/ }).waitFor();
    await settle(page);
    await shoot(page, testInfo, 'ask/search', { fullPage: true });
  });

  test('palette: Ask momoshtrem about what you typed', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'Desktop affordance');
    await signIn(page, '/library');
    await page.keyboard.press('ControlOrMeta+k');
    await page.getByRole('dialog', { name: 'Search or jump to' }).waitFor();
    await page.keyboard.type('sev');
    await page.getByRole('option', { name: /^Ask momoshtrem: “sev”/ }).waitFor();
    await page.getByRole('group', { name: 'Your library' }).getByRole('option').first().waitFor();
    await page.waitForTimeout(600);
    await shoot(page, testInfo, 'ask/palette');
  });
});
