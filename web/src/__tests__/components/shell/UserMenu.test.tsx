import { fireEvent, render, screen, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MembersGate } from '@/components/members/MembersGate';
import { ShellContext } from '@/components/shell/ShellContext';
import { accountMenuLabel, UserMenu } from '@/components/shell/UserMenu';
import { ToastProvider } from '@/components/ui';
import type { Member } from '@/lib/api/contract';
import { page, resetPage } from '@/lib/members/page';

/** The account menu with household members (#239): who's watching, and Switch to everyone else. */

const person = (id: number, name: string, state: Member['trakt']['state'] = 'connected'): Member => ({
  id,
  name,
  color: id === 1 ? 'teal' : 'rose',
  created_at: '2026-01-01T00:00:00Z',
  trakt: { state, username: state === 'off' ? null : name.toLowerCase() },
});

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown): Response =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;
const shell = { openPalette: () => undefined, setOverArt: () => undefined };

/** The phone sheet (jsdom's matchMedia never matches the desktop query). */
function renderMenu(members: { status: number; body?: unknown }, memberId: number | null = 1) {
  if (memberId !== null) document.cookie = `momoshtrem_member=${memberId}; Path=/`;
  global.fetch = jest.fn(async () => answer(members.status, members.body)) as typeof fetch;
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <MembersGate initialMemberId={memberId}>
          <ShellContext.Provider value={shell}>
            <UserMenu />
          </ShellContext.Provider>
        </MembersGate>
      </ToastProvider>
    </QueryClientProvider>
  );
}

let assign: jest.SpyInstance;
beforeEach(() => {
  assign = jest.spyOn(page, 'assign').mockImplementation(() => undefined);
  jest.spyOn(page, 'reload').mockImplementation(() => undefined);
  // Not For You itself, so the switch is a navigation to `/`
  window.history.replaceState(null, '', '/library');
});
afterEach(() => {
  global.fetch = originalFetch;
  document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
  resetPage();
  window.history.replaceState(null, '', '/');
  jest.restoreAllMocks();
});

describe('accountMenuLabel', () => {
  it('names the member and says what the menu does', () => {
    expect(accountMenuLabel(person(1, 'Teymur'), true)).toBe('Account menu, Teymur: switch, settings, log out');
    expect(accountMenuLabel(person(1, 'Teymur'), false)).toBe('Account menu, Teymur: settings, log out');
    expect(accountMenuLabel(null, false)).toBe('Account menu: settings, log out');
  });
});

describe('UserMenu', () => {
  it('shows who’s watching, their Trakt, and Switch to the others', async () => {
    renderMenu({ status: 200, body: { items: [person(1, 'Teymur'), person(2, 'Sam', 'off'), person(3, 'Alex')] } });
    const trigger = await screen.findByRole('button', { name: 'Account menu, Teymur: switch, settings, log out' });
    expect(trigger).toHaveTextContent('T');
    fireEvent.click(trigger);
    const sheet = await screen.findByRole('dialog', { name: 'Account' });
    expect(within(sheet).getByText('Teymur')).toBeInTheDocument();
    expect(within(sheet).getByText('Trakt · teymur')).toBeInTheDocument();
    const nav = within(sheet).getByRole('navigation', { name: 'Account' });
    const rows = within(nav).getAllByRole('button', { name: /^Switch to/ });
    expect(rows.map((row) => row.textContent)).toEqual(['SSwitch to Sam', 'ASwitch to Alex']);
    expect(within(nav).getByRole('link', { name: 'Manage household' })).toHaveAttribute('href', '/settings?section=household');
    expect(within(nav).getByRole('link', { name: 'Settings' })).toBeInTheDocument();
  });

  it('switching writes the cookie and goes to For You, the row busy meanwhile (and still focusable)', async () => {
    renderMenu({ status: 200, body: { items: [person(1, 'Teymur'), person(2, 'Sam'), person(3, 'Alex')] } });
    fireEvent.click(await screen.findByRole('button', { name: /Account menu, Teymur/ }));
    const row = await screen.findByRole('button', { name: 'Switch to Sam' });
    row.focus();
    fireEvent.click(row);
    expect(document.cookie).toContain('momoshtrem_member=2');
    expect(assign).toHaveBeenCalledTimes(1);
    expect(assign).toHaveBeenCalledWith('/');
    expect(row).toHaveAttribute('aria-busy', 'true');
    expect(row).toBeEnabled();
    expect(row).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Switch to Alex' })).toBeDisabled();
    fireEvent.click(row);
    expect(assign).toHaveBeenCalledTimes(1);
  });

  it('on desktop the dropdown has a Switch to item per other person, then Manage household', async () => {
    const matchMedia = window.matchMedia;
    window.matchMedia = ((query: string) => ({ ...matchMedia(query), matches: query === '(min-width: 1024px)' })) as typeof window.matchMedia;
    try {
      renderMenu({ status: 200, body: { items: [person(1, 'Teymur'), person(2, 'Sam')] } });
      const trigger = await screen.findByRole('button', { name: /Account menu, Teymur/ });
      fireEvent.keyDown(trigger, { key: 'Enter' });
      // Radix labels the menu by its trigger
      const menu = await screen.findByRole('menu', { name: /Account menu, Teymur/ });
      expect(within(menu).getByText('Trakt · teymur')).toBeInTheDocument();
      const item = within(menu).getByRole('menuitem', { name: 'Switch to Sam' });
      expect(within(menu).getAllByRole('menuitem').map((i) => i.textContent)).toEqual([
        'SSwitch to Sam',
        'Manage household',
        'Settings',
        'Infuse setup',
        'Command palette⌘K',
        'Log out',
      ]);
      expect(within(menu).getByRole('menuitem', { name: 'Manage household' })).toHaveAttribute('href', '/settings?section=household');
      fireEvent.click(item);
      expect(document.cookie).toContain('momoshtrem_member=2');
      expect(assign).toHaveBeenCalledWith('/');
    } finally {
      window.matchMedia = matchMedia;
    }
  });

  it('alone: the member and Manage household (to add someone), but no Switch rows', async () => {
    renderMenu({ status: 200, body: { items: [person(1, 'Teymur')] } });
    fireEvent.click(await screen.findByRole('button', { name: 'Account menu, Teymur: settings, log out' }));
    const sheet = await screen.findByRole('dialog', { name: 'Account' });
    expect(within(sheet).getByText('Teymur')).toBeInTheDocument();
    expect(within(sheet).queryByRole('button', { name: /Switch to/ })).not.toBeInTheDocument();
    expect(within(sheet).getByRole('link', { name: 'Manage household' })).toBeInTheDocument();
  });

  it('without household members (an older backend): the app, as before', async () => {
    renderMenu({ status: 404, body: { error: { code: 'not_found', message: 'No such API route.' } } }, null);
    fireEvent.click(await screen.findByRole('button', { name: 'Account menu: settings, log out' }));
    const sheet = await screen.findByRole('dialog', { name: 'Account' });
    expect(within(sheet).getByText('momoshtrem')).toBeInTheDocument();
    expect(within(sheet).getByText('Signed in')).toBeInTheDocument();
    expect(within(sheet).queryByRole('button', { name: /Switch to/ })).not.toBeInTheDocument();
    expect(within(sheet).queryByRole('link', { name: 'Manage household' })).not.toBeInTheDocument();
  });
});
