import type { ReactNode } from 'react';
import { act, renderHook, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAskLibrary } from '@/components/ask/useAskLibrary';
import { ToastProvider } from '@/components/ui';
import type { AskItem, LibraryItem } from '@/lib/api/contract';

/** Ask's cards and the library (#156, #160's review follow-ups): adds tracked per title, and a lookup that doesn't blink. */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

/** A request the test answers when it chooses. */
interface Held {
  url: string;
  method: string;
  respond: (status: number, body?: unknown) => void;
}
let held: Held[];
/** Lookups (`/library?ids=`) answer at once, with nothing in the library, unless a test holds them. */
let holdLookups: boolean;

beforeEach(() => {
  held = [];
  holdLookups = false;
  global.fetch = jest.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    if (method === 'GET' && !holdLookups) return Promise.resolve(answer(200, { items: [] }));
    return new Promise<Response>((resolve) => {
      held.push({ url: String(input), method, respond: (status, body) => resolve(answer(status, body)) });
    });
  }) as typeof fetch;
});
afterEach(() => {
  global.fetch = originalFetch;
});

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

const pick = (over: Partial<AskItem> & Pick<AskItem, 'media_type' | 'tmdb_id' | 'title'>): AskItem => ({
  year: 2016,
  year_end: null,
  poster_path: null,
  seasons: null,
  runtime_minutes: null,
  vote_average: null,
  release_date: null,
  badge: null,
  reason: 'Because you loved Severance.',
  library: null,
  on_watchlist: false,
  feedback: null,
  match_tag: 'Workplace · lighter',
  ...over,
});

const GOOD_PLACE = pick({ media_type: 'tv', tmdb_id: 66573, title: 'The Good Place', year_end: 2020, seasons: 4 });
const PADDINGTON = pick({ media_type: 'movie', tmdb_id: 346648, title: 'Paddington 2', year: 2017 });

const inLibrary = (item: AskItem, id: number) => ({ id, media_type: item.media_type, tmdb_id: item.tmdb_id, availability_stage: 'added' }) as LibraryItem;

/** The newest held request that matches. */
function waiting(match: (request: Held) => boolean): Held {
  const request = [...held].reverse().find(match);
  if (!request) throw new Error(`No such request in ${held.map((h) => `${h.method} ${h.url}`).join(', ')}`);
  return request;
}
/** Waits for a matching request to be sent, then answers it. */
async function respond(match: (request: Held) => boolean, status: number, body?: unknown) {
  const request = await waitFor(() => waiting(match));
  await act(async () => request.respond(status, body));
}
const post = (path: string) => (request: Held) => request.method === 'POST' && request.url.endsWith(path);
const lookupOf = (ids: string) => (request: Held) => request.method === 'GET' && request.url.includes(`ids=${encodeURIComponent(ids)}`);

it('two quick adds each keep their spinner until their own POST answers, and the toast names the seasons', async () => {
  const { result } = renderHook(() => useAskLibrary([GOOD_PLACE, PADDINGTON]), { wrapper });
  act(() => {
    result.current.add(GOOD_PLACE);
    result.current.add(PADDINGTON);
  });
  expect(result.current.adding(GOOD_PLACE)).toBe(true);
  expect(result.current.adding(PADDINGTON)).toBe(true);

  await respond(post('/api/movies'), 201, { id: 9 });
  await waitFor(() => expect(result.current.adding(PADDINGTON)).toBe(false));
  expect(result.current.stateOf(PADDINGTON)).toEqual({ kind: 'added' });
  // The first add's POST is still out: its card keeps its spinner
  expect(result.current.adding(GOOD_PLACE)).toBe(true);
  expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'none' });

  await respond(post('/api/shows'), 201, { id: 4 });
  await waitFor(() => expect(result.current.adding(GOOD_PLACE)).toBe(false));
  expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'added' });
  expect(await screen.findByText('The Good Place · 4 seasons')).toBeInTheDocument();
});

it('a failed add stops its spinner and leaves the card as it was', async () => {
  const { result } = renderHook(() => useAskLibrary([GOOD_PLACE]), { wrapper });
  act(() => result.current.add(GOOD_PLACE));
  await respond(post('/api/shows'), 400, { error: { code: 'bad_request', message: 'TMDB has no such title' } });
  await waitFor(() => expect(result.current.adding(GOOD_PLACE)).toBe(false));
  expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'none' });
  expect(await screen.findByText('Couldn’t add The Good Place')).toBeInTheDocument();
});

it('when a turn ends, a title added from its sheet stays added while the new lookup loads', async () => {
  holdLookups = true;
  const { result, rerender } = renderHook(({ answered }) => useAskLibrary(answered), { wrapper, initialProps: { answered: [GOOD_PLACE] } });
  // Added from its "…" sheet: only the lookup knows
  await respond(lookupOf('tv:66573'), 200, { items: [inLibrary(GOOD_PLACE, 4)] });
  await waitFor(() => expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'added' }));

  // The next turn ends: new ids, and their lookup hasn't answered yet
  rerender({ answered: [GOOD_PLACE, PADDINGTON] });
  expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'added' });
  // A title the last lookup didn't cover keeps its own state until the new one answers
  expect(result.current.stateOf(PADDINGTON)).toEqual({ kind: 'none' });

  await respond(lookupOf('movie:346648,tv:66573'), 200, { items: [inLibrary(GOOD_PLACE, 4)] });
  await waitFor(() => expect(result.current.stateOf(GOOD_PLACE)).toEqual({ kind: 'added' }));
  expect(result.current.stateOf(PADDINGTON)).toEqual({ kind: 'none' });
});
