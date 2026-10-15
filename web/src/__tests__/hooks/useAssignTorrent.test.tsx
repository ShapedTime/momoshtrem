import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { useAssignTorrent } from '@/hooks/useAssignTorrent';
import type { AssignmentJobDetail } from '@/lib/api/contract';
import { waitForJob } from '@/lib/api/endpoints';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';
import type { TorrentSearchContext } from '@/types/jackett';

/** Adding a movie's torrent waits on its job, but not forever (#5): a slow one hands over to its page. */

const push = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push, replace: jest.fn(), back: jest.fn(), forward: jest.fn(), refresh: jest.fn(), prefetch: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/',
}));

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

/** A job that never stops working (no peers yet). */
function stuckJob(id: number): AssignmentJobDetail {
  const job = seedJobs().get(101);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error('No seeded job');
  const detail = jobDetail(job, { entry, ai_available: false, currentFile: () => null });
  return { ...detail, job: { ...detail.job, id, status: 'fetching_metadata' } };
}

afterEach(() => {
  global.fetch = originalFetch;
  push.mockReset();
  jest.useRealTimers();
});

it('waitForJob gives up after its timeout with a TimeoutError', async () => {
  global.fetch = jest.fn(async () => answer(200, stuckJob(7))) as typeof fetch;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await expect(waitForJob(client, 7, { timeoutMs: 30 })).rejects.toMatchObject({ name: 'TimeoutError' });
});

it('a movie job still working after 40 s: the search closes and its page takes over, with no error', async () => {
  jest.useFakeTimers({ doNotFake: ['queueMicrotask', 'nextTick', 'setImmediate'] });
  global.fetch = jest.fn(async (input: RequestInfo | URL) =>
    String(input).endsWith('/assign-torrent') ? answer(202, { job_id: 7 }) : answer(200, stuckJob(7))
  ) as typeof fetch;
  const onLeave = jest.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
  const context: TorrentSearchContext = { mediaType: 'movie', query: 'Dune 2024', title: 'Dune', tmdbId: 1054867, libraryId: 3 };
  const { result } = renderHook(() => useAssignTorrent(context, { onLeave }), { wrapper });

  act(() => result.current.add('magnet:?xt=urn:btih:abc'));
  await waitFor(() => expect(result.current.isAdding('magnet:?xt=urn:btih:abc')).toBe(true));
  // Still working at 39 s
  await act(async () => {
    await jest.advanceTimersByTimeAsync(39_000);
  });
  expect(push).not.toHaveBeenCalled();
  await act(async () => {
    await jest.advanceTimersByTimeAsync(2_000);
  });
  await waitFor(() => expect(push).toHaveBeenCalledWith('/mapping/7'));
  expect(onLeave).toHaveBeenCalled();
  expect(document.body).not.toHaveTextContent('Couldn’t add the torrent');
});
