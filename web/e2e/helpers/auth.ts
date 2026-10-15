import type { Page } from '@playwright/test';

export const E2E_USER = { username: 'e2e', password: 'e2e-password' };

/** Sign in through the real form and wait for the full navigation. */
export async function signIn(page: Page, redirect = '/dev/ui') {
  await page.goto(`/login?redirect=${encodeURIComponent(redirect)}`);
  await page.getByLabel('Username').fill(E2E_USER.username);
  await page.getByLabel('Password', { exact: true }).fill(E2E_USER.password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  // Generous: on a cold build the dev server compiles the redirect target's route first
  await page.waitForURL((url) => url.pathname === redirect.split('?')[0], { timeout: 60_000 });
  // In mock mode the UI mounts once the MSW worker is ready
  await page.locator('main').first().waitFor();
}
