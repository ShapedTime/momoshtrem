import { expect, test } from '@playwright/test';
import { signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

const isDesktop = (projectName: string) => projectName === 'desktop';

test.describe('app shell', () => {
  test('navigation marks the current page and works on both layouts', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    if (isDesktop(testInfo.project.name)) {
      const topBar = page.locator('header.topbar');
      await expect(topBar).toBeVisible();
      await expect(page.locator('nav.tabbar')).toBeHidden();
      await expect(topBar.getByRole('link', { name: 'Library' })).toHaveAttribute('aria-current', 'page');
      await topBar.getByRole('link', { name: /Activity/ }).click();
    } else {
      const tabBar = page.locator('nav.tabbar');
      await expect(tabBar).toBeVisible();
      await expect(page.locator('header.topbar')).toBeHidden();
      await expect(tabBar.getByRole('link', { name: 'Library' })).toHaveAttribute('aria-current', 'page');
      await tabBar.getByRole('link', { name: /Activity/ }).click();
    }
    await expect(page).toHaveURL(/\/torrents$/);
  });

  test('the Activity badge counts downloading torrents', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const nav = isDesktop(testInfo.project.name) ? page.locator('header.topbar') : page.locator('nav.tabbar');
    await expect(nav.getByLabel('1 downloading')).toBeVisible();
  });

  test('a skip link jumps to the content', async ({ page }) => {
    await signIn(page, '/library');
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('#main')).toBeFocused();
  });

  test('the user menu signs you out', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const scope = isDesktop(testInfo.project.name) ? page.locator('header.topbar') : page.locator('header.phone-bar');
    await scope.getByRole('button', { name: /Account menu/ }).click();
    if (isDesktop(testInfo.project.name)) {
      await expect(page.getByRole('menuitem', { name: 'Settings' })).toBeVisible();
      await page.getByRole('menuitem', { name: 'Log out' }).click();
    } else {
      await expect(page.getByRole('dialog', { name: 'Account' })).toBeVisible();
      await page.getByRole('button', { name: 'Log out' }).click();
    }
    await expect(page).toHaveURL(/\/login$/);
    await page.goto('/library');
    await expect(page).toHaveURL(/\/login\?redirect=%2Flibrary$/);
  });

  test('chrome is accessible and nothing scrolls sideways down to 320 px', async ({ page }) => {
    await signIn(page, '/library');
    await expectNoA11yViolations(page, { include: 'header, nav' });
    await expectNoHorizontalOverflow(page);
    await page.setViewportSize({ width: 320, height: 640 });
    await expectNoHorizontalOverflow(page);
  });
});

test.describe('command palette', () => {
  test.beforeEach(({}, testInfo) => test.skip(!isDesktop(testInfo.project.name), 'Keyboard shortcuts are a desktop affordance'));

  test('⌘K opens it, typing filters, Enter goes there', async ({ page }) => {
    await signIn(page, '/torrents');
    await page.keyboard.press('ControlOrMeta+k');
    const palette = page.getByRole('dialog', { name: 'Search or jump to' });
    await expect(palette).toBeVisible();
    await page.keyboard.type('library');
    await page.keyboard.press('Enter');
    await expect(palette).toBeHidden();
    await expect(page).toHaveURL(/\/library$/);
  });

  test('"/" opens it, and searching hands off to the search page', async ({ page }) => {
    await signIn(page, '/library');
    await page.keyboard.press('/');
    await expect(page.getByRole('dialog', { name: 'Search or jump to' })).toBeVisible();
    await page.keyboard.type('frieren');
    await expect(page.getByRole('option', { name: /Search all results for “frieren”/ })).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/search\?q=frieren$/);
  });

  test('the search field in the top bar opens it too, and Esc closes it', async ({ page }) => {
    await signIn(page, '/library');
    await page.getByRole('button', { name: 'Search titles, your library and actions' }).click();
    const palette = page.getByRole('dialog', { name: 'Search or jump to' });
    await expect(palette).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(palette).toBeHidden();
  });
});

test.describe('URL-driven sheets', () => {
  test('a deep link opens the sheet; closing removes it from the URL', async ({ page }) => {
    await signIn(page, '/library');
    await page.goto('/library?sheet=infuse-setup');
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet).toBeVisible();
    await expect(sheet.getByLabel('WebDAV address')).toHaveText('https://momoshtrem.duckdns.org:36911/');
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/library$/);
  });

  test('opened in the app, Back closes it', async ({ page }, testInfo) => {
    test.skip(!isDesktop(testInfo.project.name), 'Opened from the palette here');
    await signIn(page, '/library');
    await page.keyboard.press('ControlOrMeta+k');
    await page.keyboard.type('how to play');
    await page.keyboard.press('Enter');
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    await expect(sheet).toBeVisible();
    await expect(page).toHaveURL(/sheet=infuse-setup/);
    await page.goBack();
    await expect(sheet).toBeHidden();
    await expect(page).toHaveURL(/\/library$/);
  });

  test('an unknown sheet name opens nothing and breaks nothing (?sheet=toString)', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    await page.goto('/library?sheet=toString');
    // The badge comes from a client query: the shell hydrated and is still standing
    const nav = isDesktop(testInfo.project.name) ? page.locator('header.topbar') : page.locator('nav.tabbar');
    await expect(nav.getByLabel('1 downloading')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Library', level: 1 })).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'This page didn’t load' })).toHaveCount(0);
  });

  test('Copy works on plain HTTP: the fallback keeps its selection inside the sheet', async ({ page }) => {
    // No async Clipboard API (a LAN address over HTTP); record what execCommand('copy') sees
    await page.addInitScript(() => {
      Object.defineProperty(Navigator.prototype, 'clipboard', { get: () => undefined, configurable: true });
      const copies: string[] = [];
      Object.assign(window, { copies });
      document.execCommand = () => {
        const active = document.activeElement;
        copies.push(active instanceof HTMLTextAreaElement ? active.value.slice(active.selectionStart, active.selectionEnd) : '');
        return true;
      };
    });
    await signIn(page, '/library');
    await page.goto('/library?sheet=infuse-setup');
    const sheet = page.getByRole('dialog', { name: 'Play it in Infuse' });
    const copy = sheet.getByRole('button', { name: 'Copy' });
    await copy.click();
    expect(await page.evaluate(() => (window as unknown as { copies: string[] }).copies)).toEqual(['https://momoshtrem.duckdns.org:36911/']);
    await expect(copy).toBeFocused();
    // Dismissing the "Copied" toast leaves the sheet open
    await page.locator('.toast-region').getByRole('button', { name: 'Dismiss' }).click();
    await expect(sheet).toBeVisible();
  });

  test('respects reduced motion', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await signIn(page, '/library');
    await page.goto('/library?sheet=infuse-setup');
    const sheet = page.locator('.sheet');
    await expect(sheet).toBeVisible();
    const duration = await sheet.evaluate((el) => getComputedStyle(el).animationDuration);
    expect(Number.parseFloat(duration)).toBeLessThanOrEqual(0.001);
  });
});
