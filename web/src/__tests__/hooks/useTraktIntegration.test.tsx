import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { type Query, QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useTraktIntegration } from '@/hooks/useTraktIntegration';
import type { TraktIntegration, TraktState, TraktSyncStatus } from '@/lib/api/contract';
import { TRAKT_POLL_MS, traktIntegrationQuery } from '@/lib/api/endpoints';

/** useTraktIntegration (#130): polls while a code waits or a sync runs, and refetches what a finished sync changed. */

const originalFetch = global.fetch;
const answer = (body: unknown) => ({ ok: true, status: 200, json: async () => body, clone: () => answer(body) }) as Response;

function integration(state: TraktState, status: TraktSyncStatus): TraktIntegration {
  return {
    configured: true,
    state,
    connected: state === 'connected',
    username: state === 'off' ? null : 'teymur',
    pending: null,
    connect_error: null,
    reauth_reason: null,
    last_sync_at: null,
    next_sync_at: null,
    sync: {
      status,
      kind: status === 'syncing' ? 'initial' : null,
      step: status === 'syncing' ? 'history' : null,
      progress: { plays: 0, plays_total: 0, ratings: 0, watchlist: 0, shows: 0 },
      error: null,
      started_at: null,
      finished_at: null,
    },
    counts: { history: 0, ratings: 0, watchlist: 0, watchlist_in_library: 0 },
    settings: { use_ratings: true, show_watchlist: true, include_paused: false },
  };
}

afterEach(() => {
  global.fetch = originalFetch;
});

describe('traktIntegrationQuery', () => {
  const interval = (data: TraktIntegration | undefined) => {
    const options = traktIntegrationQuery();
    const refetchInterval = options.refetchInterval as (query: Query<TraktIntegration>) => number | false;
    return refetchInterval({ state: { data } } as Query<TraktIntegration>);
  };

  it('polls every 3 s while a code is pending or a sync runs, and never in a hidden tab', () => {
    expect(interval(integration('pending', 'idle'))).toBe(TRAKT_POLL_MS);
    expect(interval(integration('connected', 'syncing'))).toBe(TRAKT_POLL_MS);
    expect(interval(integration('connected', 'idle'))).toBe(false);
    expect(interval(integration('connected', 'error'))).toBe(false);
    expect(interval(integration('off', 'idle'))).toBe(false);
    expect(interval(undefined)).toBe(false);
    expect(traktIntegrationQuery().refetchIntervalInBackground).toBe(false);
  });
});

describe('useTraktIntegration', () => {
  function setup(answers: TraktIntegration[]) {
    let call = 0;
    global.fetch = jest.fn(async () => answer(answers[Math.min(call++, answers.length - 1)])) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = jest.spyOn(client, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useTraktIntegration(), { wrapper });
    const invalidated = () => invalidate.mock.calls.map(([filters]) => filters?.queryKey);
    return { result, invalidated };
  }

  it('refetches /me, titles and the library when a sync goes from syncing to idle', async () => {
    const { result, invalidated } = setup([integration('connected', 'syncing'), integration('connected', 'idle')]);
    await waitFor(() => expect(result.current.data?.sync.status).toBe('syncing'));
    expect(invalidated()).toEqual([]);
    await act(() => result.current.refetch());
    await waitFor(() => expect(result.current.data?.sync.status).toBe('idle'));
    expect(invalidated()).toEqual([['me'], ['titles'], ['library']]);
  });

  it('leaves them alone when nothing was syncing', async () => {
    const { result, invalidated } = setup([integration('connected', 'idle'), integration('off', 'idle')]);
    await waitFor(() => expect(result.current.data?.state).toBe('connected'));
    await act(() => result.current.refetch());
    await waitFor(() => expect(result.current.data?.state).toBe('off'));
    expect(invalidated()).toEqual([]);
  });
});
