import { createElement, type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { pollWhileDownloading, useTitleActions } from '@/hooks/useTitle';
import type { Title } from '@/lib/api/contract';
import { buildTitle } from '@/mocks/fixtures/titles';

function title(mediaType: 'movie' | 'tv', tmdbId: number): Title {
  const built = buildTitle(mediaType, tmdbId);
  if (!built) throw new Error(`No fixture for ${mediaType} ${tmdbId}`);
  return built;
}

describe('pollWhileDownloading', () => {
  it('polls every 5 s while a movie or show downloads', () => {
    expect(pollWhileDownloading(title('movie', 157336))).toBe(5000);
    expect(pollWhileDownloading(title('tv', 247767))).toBe(5000);
  });

  it('stops once the stage moves on, and before anything loaded', () => {
    const interstellar = title('movie', 157336);
    const ready: Title = { ...interstellar, library: { ...interstellar.library, availability_stage: 'ready', download: null } };
    expect(pollWhileDownloading(ready)).toBe(false);
    expect(pollWhileDownloading(title('movie', 693134))).toBe(false);
    expect(pollWhileDownloading(title('movie', 666277))).toBe(false);
    expect(pollWhileDownloading(undefined)).toBe(false);
  });
});

describe('useTitleActions', () => {
  const originalFetch = global.fetch;
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('add refetches the title pages, the library and /me, so For You’s cards know the show is in', async () => {
    const answer = { ok: true, status: 201, json: async () => ({ id: 7, tmdb_id: 66573, title: 'The Good Place', year: 2016 }) } as Response;
    global.fetch = jest.fn(async () => ({ ...answer, clone: () => answer })) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = jest.spyOn(client, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, createElement(ToastProvider, null, createElement(ConfirmProvider, null, children)));
    const { result } = renderHook(() => useTitleActions(title('tv', 66573)), { wrapper });

    act(() => result.current.add());
    await waitFor(() => expect(invalidate).toHaveBeenCalledTimes(3));
    expect(invalidate.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([['titles'], ['library'], ['me']]);
  });
});
