import { expect, type Page } from '@playwright/test';

export const E2E_USER = { username: 'e2e', password: 'e2e-password' };

/** Sign in through the real form and wait for the full navigation. */
export async function signIn(page: Page, redirect = '/dev/ui') {
  await page.goto(`/login?redirect=${encodeURIComponent(redirect)}`);
  await page.getByLabel('Username').fill(E2E_USER.username);
  await page.getByLabel('Password', { exact: true }).fill(E2E_USER.password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL((url) => url.pathname === redirect.split('?')[0]);
}
