import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { useItemSubtitles } from '@/hooks/useItemSubtitles';
import type { DownloadSubtitleRequest, Subtitle } from '@/lib/api/contract';
import { languageName, seedSubtitles, subtitleItemKey, subtitleSearch } from '@/mocks/fixtures/subtitles';

const DUNE_RELEASE = 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX';
const originalFetch = global.fetch;
let stored: Subtitle[] = [];

const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body }) as Response;

/** Dune's subtitle endpoints: list, search and download (the mock rules, without MSW in jsdom). */
function fakeBackend(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = new URL(String(input), 'http://localhost');
  if (url.pathname === '/api/movies/1/subtitles/search') {
    const languages = (url.searchParams.get('languages') ?? '').split(',');
    return Promise.resolve(answer(200, subtitleSearch('movie', 1, languages, DUNE_RELEASE)));
  }
  if (url.pathname === '/api/movies/1/subtitles' && init?.method === 'POST') {
    const body = JSON.parse(String(init.body)) as DownloadSubtitleRequest;
    const subtitle: Subtitle = {
      ...stored[1],
      id: 99,
      language_code: body.language_code,
      language_name: languageName(body.language_code),
      release_name: body.release_name ?? null,
    };
    stored = [...stored, subtitle];
    return Promise.resolve(answer(201, subtitle));
  }
  if (url.pathname === '/api/movies/1/subtitles') return Promise.resolve(answer(200, { items: stored }));
  return Promise.resolve(answer(404, { error: { code: 'not_found', message: 'No such route' } }));
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>
        <ConfirmProvider>{children}</ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  stored = seedSubtitles().get(subtitleItemKey('movie', 1)) ?? [];
  global.fetch = jest.fn(fakeBackend) as typeof fetch;
});

afterEach(() => {
  global.fetch = originalFetch;
});

describe('useItemSubtitles', () => {
  it('starts on the first language without subtitles, and stays on it after Get', async () => {
    const { result } = renderHook(() => useItemSubtitles('movie', 1, 'dune-hash'), { wrapper });
    await waitFor(() => expect(result.current.stored).toHaveLength(2));
    expect(result.current.language).toBe('az');

    act(() => result.current.onSearch());
    await waitFor(() => expect(result.current.search.status).toBe('success'));
    const search = result.current.search;
    if (search.status !== 'success') throw new Error('expected results');

    act(() => result.current.onGet(search.items[0]));
    await waitFor(() => expect(result.current.stored).toHaveLength(3));
    // Azerbaijani is stored now, so the default would move to Turkish; the picker stays on what was searched
    expect(result.current.language).toBe('az');
    expect(result.current.search).toMatchObject({ status: 'success', language: 'az' });
  });

  it('clears the search when the release changes', async () => {
    const { result, rerender } = renderHook(({ release }) => useItemSubtitles('movie', 1, release), {
      wrapper,
      initialProps: { release: 'old-hash' },
    });
    await waitFor(() => expect(result.current.stored).toBeDefined());
    act(() => result.current.onSearch());
    await waitFor(() => expect(result.current.search.status).toBe('success'));

    rerender({ release: 'new-hash' });
    expect(result.current.search).toEqual({ status: 'idle' });
  });
});
