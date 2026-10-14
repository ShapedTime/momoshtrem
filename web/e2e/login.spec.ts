import { expect, test } from '@playwright/test';
import { E2E_USER, signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

test.describe('sign in', () => {
  test('the form is server-rendered with the attributes password managers look for', async ({ page, request }) => {
    // In the initial HTML, before any JavaScript runs
    const html = await (await request.get('/login')).text();
    expect(html).toMatch(/<form[^>]*action="\/api\/auth"[^>]*method="post"/);
    expect(html).toMatch(/<input[^>]*autoComplete="username"[^>]*name="username"/i);
    expect(html).toMatch(/<input[^>]*type="password"[^>]*autoComplete="current-password"[^>]*name="password"/i);

    await page.goto('/login');
    const username = page.getByLabel('Username');
    await expect(username).toHaveAttribute('autocomplete', 'username');
    await expect(username).toHaveAttribute('autocapitalize', 'none');
    await expect(username).toHaveAttribute('spellcheck', 'false');
    await expect(page.getByLabel('Password', { exact: true })).toHaveAttribute('autocomplete', 'current-password');
  });

  test('a protected page sends you to sign in and back, query included', async ({ page }) => {
    await page.goto('/dev/ui?from=e2e');
    await expect(page).toHaveURL(/\/login\?redirect=%2Fdev%2Fui%3Ffrom%3De2e$/);
    await page.getByLabel('Username').fill(E2E_USER.username);
    await page.getByLabel('Password', { exact: true }).fill(E2E_USER.password);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL(/\/dev\/ui\?from=e2e$/);
    await expect(page.getByRole('heading', { name: 'UI gallery' })).toBeVisible();
  });

  test('wrong credentials explain what happened and keep the fields usable', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill(E2E_USER.username);
    const password = page.getByLabel('Password', { exact: true });
    await password.fill('not-the-password');
    await page.getByRole('button', { name: 'Sign in' }).click();

    const alert = page.getByRole('alert').filter({ hasText: 'Wrong username or password' });
    await expect(alert).toBeVisible();
    await expect(alert).toContainText('caps lock');
    await expect(password).toHaveAttribute('aria-invalid', 'true');
    await expect(password).toBeEnabled();
    await expect(password).toBeFocused();
  });

  test('missing fields are named', async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByText('Enter your username and password.')).toBeVisible();
    await expect(page.getByLabel('Username')).toBeFocused();
  });

  test('never redirects off-site after signing in', async ({ page }) => {
    await page.goto('/login?redirect=%2F%2Fevil.example');
    await expect(page.locator('input[name="redirect"]')).toHaveValue('/');
  });

  test('a signed-in visit to /login goes straight to the destination', async ({ page }) => {
    await signIn(page);
    await page.goto('/login?redirect=%2Fdev%2Fui');
    await expect(page).toHaveURL(/\/dev\/ui$/);
  });

  test('the password can be shown and hidden', async ({ page }) => {
    await page.goto('/login');
    const password = page.getByLabel('Password', { exact: true });
    const toggle = page.getByRole('button', { name: 'Show password' });
    await toggle.click();
    await expect(password).toHaveAttribute('type', 'text');
    await expect(page.getByRole('button', { name: 'Hide password' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('the rate-limited state counts down and re-enables sign-in', async ({ page }) => {
    await page.goto('/login?error=rate_limited&retry=2');
    await expect(page.getByRole('status').filter({ hasText: 'Too many attempts' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Try again in 0:0\d/ })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeEnabled({ timeout: 5000 });
  });

  test('is accessible and never scrolls sideways, down to 320 px', async ({ page }) => {
    await page.goto('/login');
    await expectNoA11yViolations(page);
    await expectNoHorizontalOverflow(page);
    await page.setViewportSize({ width: 320, height: 640 });
    await expectNoHorizontalOverflow(page);
  });
});

test.describe('sign in without JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  test('the plain form post signs in and redirects', async ({ page }) => {
    await page.goto('/login?redirect=%2Fdev%2Fui');
    await page.getByLabel('Username').fill(E2E_USER.username);
    await page.getByLabel('Password', { exact: true }).fill(E2E_USER.password);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL(/\/dev\/ui$/);
  });

  test('a failed plain form post comes back with the error', async ({ page }) => {
    await page.goto('/login?redirect=%2Fdev%2Fui');
    await page.getByLabel('Username').fill(E2E_USER.username);
    await page.getByLabel('Password', { exact: true }).fill('nope');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL(/\/login\?error=invalid_credentials&redirect=%2Fdev%2Fui$/);
    await expect(page.getByText('Wrong username or password')).toBeVisible();
  });
});

test.describe('expired sessions', () => {
  test('API calls get 401 JSON instead of a login page', async ({ request }) => {
    const response = await request.get('/api/torrents', { maxRedirects: 0 });
    expect(response.status()).toBe(401);
    expect(await response.json()).toMatchObject({ error: { code: 'unauthorized' } });
  });

  test('signing out ends the session', async ({ page }) => {
    await signIn(page);
    const status = await page.evaluate(async () => (await fetch('/api/auth', { method: 'DELETE' })).status);
    expect(status).toBe(200);
    await page.goto('/dev/ui');
    await expect(page).toHaveURL(/\/login\?redirect=%2Fdev%2Fui$/);
  });
});
