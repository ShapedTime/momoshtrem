import { expect, test } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

const isDesktop = (name: string) => name === 'desktop';

test.describe('settings', () => {
  test('desktop shows the sidebar and the playback section by default', async ({ page }, testInfo) => {
    test.skip(!isDesktop(testInfo.project.name), 'Desktop layout');
    await signIn(page, '/settings');
    const nav = page.getByRole('navigation', { name: 'Settings sections' });
    await expect(nav.getByRole('link', { name: /Infuse & WebDAV/ })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('heading', { name: 'Infuse & WebDAV' })).toBeVisible();
    await expect(page.locator('code[aria-label="WebDAV address"]')).toHaveText('https://momoshtrem.duckdns.org:36911/');
    await expect(page.getByText('Set up Infuse (once)')).toBeVisible();
  });

  test('phone shows the list first, then one section with a way back', async ({ page }, testInfo) => {
    test.skip(isDesktop(testInfo.project.name), 'Phone layout');
    await signIn(page, '/settings');
    await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Infuse & WebDAV' })).toBeHidden();
    await page.getByRole('link', { name: /System status/ }).click();
    await expect(page).toHaveURL(/section=status/);
    await expect(page.getByRole('heading', { name: 'System status' })).toBeVisible();
    await page.getByRole('link', { name: 'Settings' }).click();
    await expect(page).toHaveURL(/\/settings$/);
  });

  test('system status lists every service with its state', async ({ page }) => {
    await signIn(page, '/settings?section=status');
    const services = page.getByRole('list', { name: 'Services' });
    for (const name of ['momoshtrem backend', 'WebDAV', 'Jackett', 'TMDB', 'Trakt', 'AI', 'OpenSubtitles']) {
      await expect(services.getByText(name, { exact: true })).toBeVisible();
    }
    await expect(page.getByRole('status').filter({ hasText: 'All good' })).toBeVisible();
    await expect(services.getByText('Not connected').first()).toBeVisible();
    await page.getByRole('button', { name: 'Re-check' }).click();
    await expect(page.getByText(/Checked \d/)).toBeVisible();
  });

  test('is accessible and never scrolls sideways', async ({ page }) => {
    await signIn(page, '/settings?section=status');
    await expect(page.getByRole('list', { name: 'Services' })).toBeVisible();
    await expectNoA11yViolations(page);
    await page.setViewportSize({ width: 320, height: 640 });
    await expectNoHorizontalOverflow(page);
  });
});

test.describe('not found', () => {
  test('an unknown path 404s inside the shell', async ({ page }) => {
    await signIn(page, '/library');
    // (The dev server may flush a 200 before compiling the page; a production build answers 404.)
    await page.goto('/no/such/page');
    await expect(page.getByRole('heading', { name: 'Nothing here but the cat' })).toBeVisible();
    await expect(page.getByText('/no/such/page')).toBeVisible();
    await expectNoA11yViolations(page);
    await expectNoHorizontalOverflow(page);
    await page.getByRole('link', { name: 'Go to For You' }).click();
    await expect(page).toHaveURL(/\/$/);
  });
});

test.describe('backend offline', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => window.localStorage.setItem('msw:scenario', 'backend-down'));
  });

  test('the shell says so and links to the full page, which recovers on its own', async ({ page }) => {
    await signIn(page, '/library');
    const banner = page.getByRole('alert').filter({ hasText: 'Can’t reach momoshtrem' });
    await expect(banner).toBeVisible();
    await banner.getByRole('link', { name: 'Details' }).click();
    await expect(page).toHaveURL(/\/offline\?from=%2Flibrary$/);
    await expect(page.getByRole('heading', { name: 'momoshtrem isn’t responding' })).toBeVisible();
    await expect(page.getByRole('list', { name: 'What still works' })).toContainText('Infuse can’t play right now either');
    await expect(page.getByText(/Trying again in \d+ s · attempt 2/)).toBeVisible();
    await expectNoA11yViolations(page);
    await expectNoHorizontalOverflow(page);

    // The server comes back: Retry returns to where you were
    await page.evaluate(() => window.localStorage.removeItem('msw:scenario'));
    await page.getByRole('button', { name: 'Retry now' }).click();
    await expect(page).toHaveURL(/\/library$/);
  });
});

test.describe('device offline', () => {
  test('the shell explains that you are offline', async ({ page, context }) => {
    await signIn(page, '/library');
    await context.setOffline(true);
    await expect(page.getByRole('status').filter({ hasText: 'You’re offline' })).toBeVisible();
    await context.setOffline(false);
    await expect(page.getByText('You’re offline')).toBeHidden();
  });
});
