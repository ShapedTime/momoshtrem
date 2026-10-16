import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { TraktSection } from '@/components/settings/TraktSection';
import type { TraktIntegration } from '@/lib/api/contract';

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
