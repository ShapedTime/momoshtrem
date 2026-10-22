import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MembersGate } from '@/components/members/MembersGate';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { TraktSection } from '@/components/settings/TraktSection';
import type { Member, TraktIntegration } from '@/lib/api/contract';

/** Settings › Trakt (#134): the switches move on the tap and go back when the server refuses. */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

afterEach(() => {
  global.fetch = originalFetch;
});

const CONNECTED: TraktIntegration = {
  configured: true,
  state: 'connected',
  connected: true,
  username: 'teymur',
  pending: null,
  connect_error: null,
  reauth_reason: null,
  last_sync_at: new Date(Date.now() - 4 * 60_000).toISOString(),
  next_sync_at: new Date(Date.now() + 26 * 60_000).toISOString(),
  sync: {
    status: 'idle',
    kind: 'incremental',
    step: null,
    progress: { plays: 2184, plays_total: 2184, ratings: 312, watchlist: 41, shows: 18 },
    error: null,
    started_at: null,
    finished_at: null,
  },
  counts: { history: 2184, ratings: 312, watchlist: 41, watchlist_in_library: 6 },
  settings: { use_ratings: true, show_watchlist: true, include_paused: false },
};

/** GET answers `integration`; PUT /settings answers `put(body)`. */
function renderSection(put: (body: Record<string, boolean>) => Response) {
  const puts: Array<Record<string, boolean>> = [];
  global.fetch = jest.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as Record<string, boolean>;
      puts.push(body);
      return Promise.resolve(put(body));
    }
    return Promise.resolve(answer(200, CONNECTED));
  }) as typeof fetch;
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <ConfirmProvider>
          <TraktSection />
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
  return puts;
}

describe('TraktSection', () => {
  it('shows the account, what’s imported and the schedule', async () => {
    renderSection((body) => answer(200, { ...CONNECTED.settings, ...body }));
    expect(await screen.findByRole('heading', { name: 'Trakt account teymur' })).toBeInTheDocument();
    expect(screen.getByText('Last sync 4 min ago · syncs every 30 min')).toBeInTheDocument();
    expect(screen.getByText('41 titles · 6 already in your library')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sync now' })).toBeEnabled();
  });

  it('a switch saves only what changed and stays where it was put', async () => {
    const puts = renderSection((body) => answer(200, { ...CONNECTED.settings, ...body }));
    const paused = await screen.findByRole('switch', { name: 'Up next includes paused shows' });
    fireEvent.click(paused);
    expect(paused).toBeChecked();
    await waitFor(() => expect(puts).toEqual([{ include_paused: true }]));
    await waitFor(() => expect(paused).toBeChecked());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('a refused switch goes back and says so', async () => {
    renderSection(() => answer(500, { error: { code: 'internal_error', message: 'The database is locked.' } }));
    const ratings = await screen.findByRole('switch', { name: 'Ratings shape your picks' });
    fireEvent.click(ratings);
    expect(ratings).not.toBeChecked();
    expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t save that setting');
    await waitFor(() => expect(ratings).toBeChecked());
  });
});

describe('TraktSection in a household (#239)', () => {
  const person = (id: number, name: string): Member => ({
    id,
    name,
    color: id === 1 ? 'teal' : 'rose',
    created_at: '2026-01-01T00:00:00Z',
    trakt: { state: 'connected', username: name.toLowerCase() },
  });

  /** /api/members answers `members`; /api/integrations/trakt answers `integration`. */
  function renderInHousehold(members: Member[], integration: TraktIntegration = CONNECTED) {
    document.cookie = 'momoshtrem_member=1; Path=/';
    global.fetch = jest.fn((input: RequestInfo | URL) =>
      Promise.resolve(String(input) === '/api/members' ? answer(200, { items: members }) : answer(200, integration))
    ) as typeof fetch;
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ToastProvider>
          <ConfirmProvider>
            <MembersGate initialMemberId={1}>
              <TraktSection />
            </MembersGate>
          </ConfirmProvider>
        </ToastProvider>
      </QueryClientProvider>
    );
  }

  afterEach(() => {
    document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
  });

  it('says whose Trakt it is, offers the other person, and names them in the disconnect', async () => {
    renderInHousehold([person(1, 'Teymur'), person(2, 'Sam')]);
    expect(await screen.findByText('This is Teymur’s Trakt.')).toBeInTheDocument();
    expect(screen.getByText(/Everyone in the household connects their own\./)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Switch to Sam' })).toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Disconnect Teymur’s Trakt?' });
    expect(dialog).toHaveTextContent('Teymur’s imported history, ratings and watchlist will be deleted from momoshtrem.');
  });

  it('with three people there’s no single Switch to', async () => {
    renderInHousehold([person(1, 'Teymur'), person(2, 'Sam'), person(3, 'Alex')]);
    expect(await screen.findByText('This is Teymur’s Trakt.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Switch to/ })).not.toBeInTheDocument();
  });

  it('alone, it reads as before', async () => {
    renderInHousehold([person(1, 'Teymur')]);
    expect(await screen.findByRole('heading', { name: 'Trakt account teymur' })).toBeInTheDocument();
    expect(screen.queryByText(/This is .* Trakt\./)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }));
    expect(await screen.findByRole('alertdialog', { name: 'Disconnect Trakt?' })).toBeInTheDocument();
  });

  it('an account another member connected: the warning on the off screen', async () => {
    renderInHousehold([person(1, 'Teymur'), person(2, 'Sam')], {
      ...CONNECTED,
      state: 'off',
      connected: false,
      username: null,
      connect_error: 'account_in_use',
    });
    const banner = await screen.findByText('That Trakt account is already connected');
    expect(banner.closest('.banner')).toHaveTextContent(
      'Someone else in this household connected it. Each person connects their own: sign in to trakt.tv with your account, then connect again.'
    );
  });
});
