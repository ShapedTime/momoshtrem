import type { Page } from '@playwright/test';

export const E2E_USER = { username: 'e2e', password: 'e2e-password' };

/** The household member a test is signed in as (#239): the mock's Teymur is 1 and Sam 2. */
export const MEMBER_COOKIE = 'momoshtrem_member';

/** Choose who's watching for this browser, as the picker would. */
export async function setMember(page: Page, member: number) {
  await page.context().addCookies([{ name: MEMBER_COOKIE, value: String(member), domain: 'localhost', path: '/' }]);
}

/**
 * Sign in through the real form and wait for the full navigation. `member`
 * (Teymur, 1, by default) is set as the member cookie first, so the
 * two-person mock household never stops a test at "Who's watching?";
 * `member: null` leaves it unset (the picker's tests).
 */
export async function signIn(page: Page, redirect = '/dev/ui', { member = 1 }: { member?: number | null } = {}) {
  if (member !== null) await setMember(page, member);
  await page.goto(`/login?redirect=${encodeURIComponent(redirect)}`);
  await page.getByLabel('Username').fill(E2E_USER.username);
  await page.getByLabel('Password', { exact: true }).fill(E2E_USER.password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  // Generous: on a cold build the dev server compiles the redirect target's route first
  await page.waitForURL((url) => url.pathname === redirect.split('?')[0], { timeout: 60_000 });
  // In mock mode the UI mounts once the MSW worker is ready
  await page.locator('main').first().waitFor();
}
