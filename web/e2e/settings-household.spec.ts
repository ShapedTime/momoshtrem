import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { MEMBER_COOKIE, signIn } from './helpers/auth';
import { expectNoA11yViolations, expectNoHorizontalOverflow } from './helpers/a11y';

/**
 * Settings › Household (#247, W2), against the MSW household
 * (src/mocks/fixtures/members.ts: Teymur 1 teal, Sam 2 rose; `members-full`
 * six people, `members-one` Teymur alone). The list is kept in localStorage,
 * so it survives the reloads a switch does.
 */

const isDesktop = (testInfo: TestInfo) => testInfo.project.name === 'desktop';
const scenario = (page: Page, name: string) => page.addInitScript((value) => window.localStorage.setItem('msw:scenario', value), name);
const people = (page: Page) => page.getByRole('region', { name: 'People' });
const row = (page: Page, name: string) => people(page).getByRole('listitem').filter({ has: page.getByRole('button', { name: `Edit ${name}` }) });
const sheet = (page: Page, title: string) => page.getByRole('dialog', { name: title });
const nameField = (page: Page, title: string) => sheet(page, title).getByRole('textbox', { name: 'Name' });

/** The settings nav: beside the section on desktop; on phone, Back to it (client-side, so the mocks keep what changed). */
async function sectionNav(page: Page) {
  const back = page.getByRole('link', { name: 'Settings', exact: true });
  if (await back.isVisible()) await back.click();
  return page.getByRole('navigation', { name: 'Settings sections' });
}

async function openSection(page: Page, member: number = 1) {
  await signIn(page, '/settings?section=household', { member });
  await expect(page.getByRole('heading', { name: 'Household', level: 2 })).toBeVisible();
  await expect(people(page).getByRole('listitem').first()).toBeVisible();
}

/** The account menu where it's on screen: the top bar's dropdown on desktop, the phone bar's sheet on phone. */
async function openAccountMenu(page: Page, testInfo: TestInfo) {
  const scope = isDesktop(testInfo) ? page.locator('header.topbar') : page.locator('header.phone-bar');
  await scope.getByRole('button', { name: /^Account menu/ }).click();
  return isDesktop(testInfo) ? page.getByRole('menu') : page.getByRole('dialog', { name: 'Account' });
}

async function memberCookie(page: Page): Promise<string | undefined> {
  return (await page.context().cookies()).find((cookie) => cookie.name === MEMBER_COOKIE)?.value;
}

/**
 * axe and no sideways scrolling at this width and at 320 px. `overlay` scopes axe to an open sheet or
 * dialog (it hides the page behind it); 320 px is the phone layout, where a desktop panel would remount
 * as a bottom sheet, so an overlay is checked there on the phone project only.
 */
async function expectAccessibleAndContained(page: Page, testInfo: TestInfo, overlay?: string) {
  await page.waitForFunction(() =>
    document.getAnimations().every((animation) => animation.playState !== 'running' || animation.effect?.getTiming().iterations === Infinity)
  );
  await expectNoA11yViolations(page, overlay ? { include: overlay } : {});
  await expectNoHorizontalOverflow(page);
  if (overlay && isDesktop(testInfo)) return;
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: 320, height: 640 });
  await expectNoHorizontalOverflow(page);
  if (viewport) await page.setViewportSize(viewport);
}

test.describe('Settings › Household', () => {
  test('the list; add someone, with the name’s checks and a taken name, then switch to them', async ({ page }, testInfo) => {
    await openSection(page);
    await expect(page.getByText('Who watches here. Each person has their own Trakt, For You and Ask.')).toBeVisible();
    await expect(row(page, 'Teymur')).toContainText('Watching now');
    await expect(row(page, 'Teymur')).toContainText('Trakt · teymur');
    await expect(row(page, 'Sam')).toContainText('Trakt · sam');
    await expect(row(page, 'Sam')).not.toContainText('Watching now');
    await expect(page.getByText(/^Everyone shares the library, downloads, Activity and settings\./)).toBeVisible();
    await expect((await sectionNav(page)).getByRole('link', { name: /Household/ })).toContainText('Teymur · Sam');
    await page.goto('/settings?section=household');
    await expectAccessibleAndContained(page, testInfo);

    await people(page).getByRole('button', { name: 'Add someone' }).click();
    const add = sheet(page, 'Add someone');
    await expect(add.getByRole('radio', { name: 'Orchid' })).toBeChecked();
    await add.getByRole('button', { name: 'Add', exact: true }).click();
    await expect(nameField(page, 'Add someone')).toHaveAccessibleDescription('Give them a name.');
    await expect(nameField(page, 'Add someone')).toBeFocused();
    await nameField(page, 'Add someone').fill('x'.repeat(33));
    await expect(nameField(page, 'Add someone')).toHaveAccessibleDescription('Keep it to 32 characters.');

    await nameField(page, 'Add someone').fill('sam');
    await add.getByRole('button', { name: 'Add', exact: true }).click();
    await expect(nameField(page, 'Add someone')).toHaveAccessibleDescription('Someone here is already called Sam.');
    await expectAccessibleAndContained(page, testInfo, '[role="dialog"]');

    await nameField(page, 'Add someone').fill('Alex');
    await add.getByRole('radio', { name: 'Lime' }).check();
    const post = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith('/api/members'));
    await add.getByRole('button', { name: 'Add', exact: true }).click();
    expect((await post).postDataJSON()).toEqual({ name: 'Alex', color: 'lime' });
    const toast = page.getByRole('status').filter({ hasText: 'Alex was added' });
    await expect(row(page, 'Alex')).toContainText('No Trakt yet');

    await toast.getByRole('button', { name: 'Switch to Alex' }).click();
    await page.waitForURL((url) => url.pathname === '/');
    await expect.poll(() => memberCookie(page)).toBe('3');
    // Alex has no Trakt yet: For You asks him to connect
    await expect(page.getByRole('link', { name: 'Connect Trakt' }).first()).toBeVisible();
  });

  test('members-full: six people, so Add is off', async ({ page }) => {
    await scenario(page, 'members-full');
    await openSection(page);
    await expect(people(page).getByRole('listitem')).toHaveCount(6);
    await expect(people(page).getByRole('button', { name: 'Add someone' })).toBeDisabled();
    await expect(people(page)).toContainText('A household can have up to six people.');
    await expect((await sectionNav(page)).getByRole('link', { name: /Household/ })).toContainText('6 people');
  });

  test('rename and recolour: the list and the account menu follow', async ({ page }, testInfo) => {
    await openSection(page);
    await row(page, 'Teymur').getByRole('button', { name: 'Edit Teymur' }).click();
    const edit = sheet(page, 'Edit Teymur');
    await expect(nameField(page, 'Edit Teymur')).toHaveValue('Teymur');
    await expect(edit.getByRole('radio', { name: 'Teal' })).toBeChecked();
    await expectAccessibleAndContained(page, testInfo, '[role="dialog"]');

    await nameField(page, 'Edit Teymur').fill('Tey');
    await edit.getByRole('radio', { name: 'Sand' }).check();
    const patch = page.waitForRequest((request) => request.method() === 'PATCH' && request.url().endsWith('/api/members/1'));
    await edit.getByRole('button', { name: 'Save' }).click();
    expect((await patch).postDataJSON()).toEqual({ name: 'Tey', color: 'sand' });
    await expect(page.getByRole('status').filter({ hasText: 'Tey was updated' })).toBeVisible();
    await expect(row(page, 'Tey')).toContainText('Watching now');
    await expect(row(page, 'Tey').locator('.member-avatar')).toHaveAttribute('data-color', 'sand');

    const menu = await openAccountMenu(page, testInfo);
    await expect(menu.getByText('Tey', { exact: true })).toBeVisible();
    await expect(menu.locator('.member-avatar[data-color="sand"]').first()).toBeVisible();
  });

  test('remove someone, confirmed with what goes and what stays', async ({ page }, testInfo) => {
    await openSection(page);
    await row(page, 'Sam').getByRole('button', { name: 'Edit Sam' }).click();
    await sheet(page, 'Edit Sam').getByRole('button', { name: 'Remove Sam' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Remove Sam from the household?' });
    await expect(confirm).toContainText(
      'Sam’s Trakt connection and imported history, For You picks, Ask threads and picks feedback are deleted from momoshtrem. The library, downloads and settings stay. Nothing changes on Trakt.'
    );
    await expectAccessibleAndContained(page, testInfo, '[role="alertdialog"]');

    await confirm.getByRole('button', { name: 'Remove Sam' }).click();
    await expect(page.getByRole('status').filter({ hasText: 'Sam was removed from the household' })).toBeVisible();
    await expect(people(page).getByRole('listitem')).toHaveCount(1);
    await expect(row(page, 'Sam')).toHaveCount(0);
  });

  test('members-one: the last person can’t be removed', async ({ page }) => {
    await scenario(page, 'members-one');
    await openSection(page);
    await row(page, 'Teymur').getByRole('button', { name: 'Edit Teymur' }).click();
    const remove = sheet(page, 'Edit Teymur').getByRole('button', { name: 'Remove Teymur' });
    await expect(remove).toBeDisabled();
    await expect(remove).toHaveAccessibleDescription('The household needs at least one person.');
  });

  test('removing yourself: For You as whoever is left', async ({ page }) => {
    await openSection(page, 2);
    await expect(row(page, 'Sam')).toContainText('Watching now');
    await row(page, 'Sam').getByRole('button', { name: 'Edit Sam' }).click();
    await sheet(page, 'Edit Sam').getByRole('button', { name: 'Remove Sam' }).click();
    const confirm = page.getByRole('alertdialog', { name: 'Remove Sam from the household?' });
    await expect(confirm).toContainText('You’ll continue as Teymur.');
    await confirm.getByRole('button', { name: 'Remove Sam' }).click();

    await page.waitForURL((url) => url.pathname === '/');
    await expect(page.getByRole('region', { name: 'Because you loved Severance' })).toBeVisible();
    await expect.poll(() => memberCookie(page)).toBe('1');
  });

  test('the account menu’s Manage household opens it', async ({ page }, testInfo) => {
    await signIn(page, '/library');
    const menu = await openAccountMenu(page, testInfo);
    await menu.getByRole(isDesktop(testInfo) ? 'menuitem' : 'link', { name: 'Manage household' }).click();
    await page.waitForURL((url) => url.pathname === '/settings' && url.searchParams.get('section') === 'household');
    await expect(page.getByRole('heading', { name: 'Household', level: 2 })).toBeVisible();
  });

  test('members-absent: households need a newer server', async ({ page }) => {
    await scenario(page, 'members-absent');
    await signIn(page, '/settings?section=household');
    await expect(page.getByText('Households need a newer momoshtrem')).toBeVisible();
    await expect(page.getByText('Update the server to add people with their own Trakt and For You.')).toBeVisible();
  });
});
