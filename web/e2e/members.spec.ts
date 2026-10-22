import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { MEMBER_COOKIE, signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Household members (#239, W1): "Who's watching?", the account menu's
 * switcher and Settings › Trakt's note, against the MSW household
 * (src/mocks/fixtures/members.ts: Teymur 1, Sam 2). The list survives the
 * reloads a switch does (localStorage); Sam's For You leads with "Because you
 * loved The Bear", Teymur's with "Because you loved Severance".
 */

const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';
const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
const picker = (page: Page) => page.getByRole('heading', { name: 'Who’s watching?', level: 1 });
const shelf = (page: Page, title: string) => page.getByRole('region', { name: title });
const SAMS = 'Because you loved The Bear';
const TEYMURS = 'Because you loved Severance';

async function memberCookie(page: Page): Promise<string | undefined> {
  return (await page.context().cookies()).find((cookie) => cookie.name === MEMBER_COOKIE)?.value;
}

/** The account menu's trigger where it's on screen: the top bar on desktop, the phone bar (the hero's on For You) on phone. */
function accountButton(page: Page, testInfo: TestInfo) {
  const scope = isDesktop(testInfo) ? page.locator('header.topbar') : page.locator('header.phone-bar');
  return scope.getByRole('button', { name: /^Account menu/ });
}

/** Open the account menu and pick "Switch to <name>". */
async function switchTo(page: Page, testInfo: TestInfo, name: string) {
  await accountButton(page, testInfo).click();
  if (isDesktop(testInfo)) {
    await page.getByRole('menuitem', { name: `Switch to ${name}` }).click();
  } else {
    await page.getByRole('dialog', { name: 'Account' }).getByRole('button', { name: `Switch to ${name}` }).click();
  }
}

/** axe and no sideways scrolling at this width and at 320 px. */
async function expectAccessibleAndContained(page: Page) {
  await expectNoA11yViolations(page);
  await expectNoHorizontalOverflow(page);
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

test.describe('Who’s watching?', () => {
  test('without a member cookie: the picker, outside the app; pick Sam, then his For You, and no picker on reload', async ({ page }) => {
    await signIn(page, '/', { member: null });
    await expect(picker(page)).toBeVisible();
    await expect(page).toHaveTitle('Who’s watching? · momoshtrem');
    await expect(page.getByRole('navigation', { name: 'Main' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Teymur' })).toBeVisible();
    await expectAccessibleAndContained(page);

    await page.getByRole('button', { name: 'Sam' }).click();
    await expect(shelf(page, SAMS)).toBeVisible();
    expect(await memberCookie(page)).toBe('2');
    await page.reload();
    await expect(shelf(page, SAMS)).toBeVisible();
    await expect(picker(page)).toHaveCount(0);
  });

  test('picking keeps the deep link it was opened with', async ({ page }) => {
    await signIn(page, '/library', { member: null });
    await page.getByRole('button', { name: 'Teymur' }).click();
    await expect(page.getByRole('heading', { name: 'Library', level: 1 })).toBeVisible();
    await expect(page).toHaveURL(/\/library$/);
  });

  test('members-one: straight to For You, and the only member is chosen', async ({ page }) => {
    await scenario(page, 'members-one');
    await signIn(page, '/', { member: null });
    await expect(shelf(page, TEYMURS)).toBeVisible();
    await expect(picker(page)).toHaveCount(0);
    await expect.poll(() => memberCookie(page)).toBe('1');
  });

  test('a cookie that names nobody: forgotten, then the picker', async ({ page }) => {
    await signIn(page, '/', { member: 9 });
    await expect(picker(page)).toBeVisible();
    await expect.poll(() => memberCookie(page)).toBeUndefined();
  });

  test('members-error without a cookie: the picker says so, with Try again', async ({ page }) => {
    await scenario(page, 'members-error');
    await signIn(page, '/', { member: null });
    const alert = page.getByRole('alert').filter({ hasText: 'Couldn’t load who’s watching' });
    await expect(alert).toContainText('momoshtrem didn’t answer. Check that it’s running, then try again.', { timeout: 20_000 });
    await expect(alert.getByRole('button', { name: 'Try again' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('members-absent (a backend before households): the app as before, even without a cookie', async ({ page }, testInfo) => {
    await scenario(page, 'members-absent');
    await signIn(page, '/library', { member: null });
    await expect(page.getByRole('heading', { name: 'Library', level: 1 })).toBeVisible();
    await expect(picker(page)).toHaveCount(0);
    await accountButton(page, testInfo).click();
    await expect(page.getByText('Signed in')).toBeVisible();
    await expect(page.getByText(/^Switch to/)).toHaveCount(0);
  });
});

test.describe('the account menu', () => {
  test('says who’s watching, and switches to Sam: For You, his shelves and his Trakt', async ({ page }, testInfo) => {
    await signIn(page, '/');
    await expect(shelf(page, TEYMURS)).toBeVisible();
    await expect(accountButton(page, testInfo)).toHaveAccessibleName('Account menu, Teymur: switch, settings, log out');
    await accountButton(page, testInfo).click();
    await expect(page.getByText('Trakt · teymur')).toBeVisible();
    // The open menu hides the page behind it from assistive tech, so check the menu itself
    await expectNoA11yViolations(page, { include: isDesktop(testInfo) ? '[role="menu"]' : '[role="dialog"]' });
    await page.keyboard.press('Escape');

    await switchTo(page, testInfo, 'Sam');
    await expect(shelf(page, SAMS)).toBeVisible();
    await expect(page).toHaveURL(/\/$/);
    await expect(accountButton(page, testInfo)).toHaveAccessibleName('Account menu, Sam: switch, settings, log out');

    await page.goto('/settings?section=trakt');
    await expect(page.getByText('This is Sam’s Trakt.')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Trakt account sam' })).toBeVisible();
    await expectAccessibleAndContained(page);
  });

  test('switching goes to For You from anywhere, keeping no page state', async ({ page }, testInfo) => {
    await signIn(page, '/library?view=list');
    await expect(page.getByRole('heading', { name: 'Library', level: 1 })).toBeVisible();
    await switchTo(page, testInfo, 'Sam');
    await expect(shelf(page, SAMS)).toBeVisible();
    expect(new URL(page.url()).pathname + new URL(page.url()).search).toBe('/');
  });

  test('Settings › Trakt offers the other person', async ({ page }) => {
    await signIn(page, '/settings?section=trakt');
    await expect(page.getByText('This is Teymur’s Trakt.')).toBeVisible();
    await page.getByRole('button', { name: 'Switch to Sam' }).click();
    await expect(page.getByText('This is Sam’s Trakt.')).toBeVisible();
    await expect(page).toHaveURL(/section=trakt/);
  });

  test('members-fresh: Sam hasn’t connected Trakt, so his For You asks him to', async ({ page }) => {
    await scenario(page, 'members-fresh');
    await signIn(page, '/', { member: 2 });
    await expect(page.getByRole('link', { name: 'Connect Trakt' }).first()).toBeVisible();
    await expect(shelf(page, SAMS)).toHaveCount(0);
  });
});
