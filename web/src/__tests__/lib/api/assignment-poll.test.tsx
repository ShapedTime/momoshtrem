import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { useMappingActions } from '@/hooks/useMappingJob';
import type { AssignmentJobDetail } from '@/lib/api/contract';
import { assignmentJobQuery, assignmentKeys, isOlderDetail, keepNewerDetail } from '@/lib/api/endpoints';
import { withDecisions } from '@/lib/mapping/review';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/**
 * While a re-check runs the job polls every 1.5 s, so a poll can land after
 * a PATCH answer that's newer than it: the PATCH answer must win (#5).
 */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function frieren(updatedAt: string): AssignmentJobDetail {
  const job = seedJobs().get(101);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error('No seeded job 101');
  const detail = jobDetail(job, { entry, ai_available: true, currentFile: () => null });
  return { ...detail, job: { ...detail.job, status: 'resolving', updated_at: updatedAt } };
}

const at = (detail: AssignmentJobDetail, updatedAt: string): AssignmentJobDetail => ({ ...detail, job: { ...detail.job, updated_at: updatedAt } });

afterEach(() => {
  global.fetch = originalFetch;
});

describe('isOlderDetail', () => {
  const base = frieren('2026-10-15T12:00:00Z');
  it('compares updated_at to the nanosecond, with or without a fraction (Go trims trailing zeros)', () => {
    expect(isOlderDetail(at(base, '2026-10-15T12:00:00.123456Z'), at(base, '2026-10-15T12:00:00.123456789Z'))).toBe(true);
    expect(isOlderDetail(at(base, '2026-10-15T12:00:00.123456789Z'), at(base, '2026-10-15T12:00:00.123456Z'))).toBe(false);
    expect(isOlderDetail(at(base, '2026-10-15T12:00:00Z'), at(base, '2026-10-15T12:00:00.5Z'))).toBe(true);
    expect(isOlderDetail(at(base, '2026-10-15T14:00:00+02:00'), at(base, '2026-10-15T12:00:01Z'))).toBe(true);
  });

  it('never calls the same instant, another job or an unreadable timestamp older', () => {
    expect(isOlderDetail(at(base, '2026-10-15T12:00:00.100Z'), at(base, '2026-10-15T12:00:00.1Z'))).toBe(false);
    expect(isOlderDetail(at(base, 'yesterday'), at(base, '2026-10-15T12:00:00Z'))).toBe(false);
    const other = { ...base, job: { ...base.job, id: 102, updated_at: '2026-10-14T12:00:00Z' } };
    expect(isOlderDetail(other, base)).toBe(false);
  });

  it('keepNewerDetail keeps the newer detail, and shares structure otherwise', () => {
    const newer = at(base, '2026-10-15T12:00:02Z');
    expect(keepNewerDetail(newer, at(base, '2026-10-15T12:00:01Z'))).toBe(newer);
    const next = at(base, '2026-10-15T12:00:03Z');
    const kept = keepNewerDetail(newer, next) as AssignmentJobDetail;
    expect(kept.job.updated_at).toBe('2026-10-15T12:00:03Z');
    // Unchanged proposals keep their identity, so rows don't re-render
    expect(kept.proposals).toBe(newer.proposals);
  });
});

describe('a poll never overwrites a newer PATCH answer', () => {
  const T0 = '2026-10-15T12:00:00Z';
  const T1 = '2026-10-15T12:00:01Z';

  function setup() {
    const before = frieren(T0);
    const v2 = before.proposals.find((p) => p.file_path.includes('35v2'));
    if (!v2) throw new Error('No 35v2 file');
    const change = [{ id: v2.id, decision: 'accepted' as const }];
    const patched = { ...at(before, T1), proposals: withDecisions(before.proposals, change) };
    const gets: Array<ReturnType<typeof deferred<Response>>> = [];
    const patch = deferred<Response>();
    let reads = 0;
    global.fetch = jest.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'PATCH') return patch.promise;
      reads += 1;
      if (reads === 1) return answer(200, before);
      const get = deferred<Response>();
      gets.push(get);
      return get.promise;
    }) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <ToastProvider>
          <ConfirmProvider>{children}</ConfirmProvider>
        </ToastProvider>
      </QueryClientProvider>
    );
    return { before, patched, change, gets, patch, client, wrapper };
  }

  it('a poll still on its way when the PATCH answers is cancelled, and its stale answer dropped', async () => {
    const { before, patched, change, gets, patch, client, wrapper } = setup();
    const { result } = renderHook(
      () => ({ query: useQuery(assignmentJobQuery(101)), actions: useMappingActions(before.job) }),
      { wrapper }
    );
    await waitFor(() => expect(result.current.query.data).toBeDefined());
    const key = assignmentKeys.job(101);

    let changing: Promise<unknown> = Promise.resolve();
    act(() => {
      changing = result.current.actions.change(change);
    });
    // The 1.5 s poll fires while the PATCH is on its way: it reads the job before the change
    act(() => {
      void client.refetchQueries({ queryKey: key, exact: true });
    });
    await waitFor(() => expect(gets).toHaveLength(1));
    expect(client.getQueryState(key)?.fetchStatus).toBe('fetching');

    await act(async () => {
      patch.resolve(answer(200, patched));
      await changing;
    });
    expect(client.getQueryState(key)?.fetchStatus).toBe('idle');
    expect(client.getQueryData<AssignmentJobDetail>(key)?.job.updated_at).toBe(T1);

    // The stale poll answers last
    await act(async () => {
      gets[0].resolve(answer(200, before));
    });
    const detail = client.getQueryData<AssignmentJobDetail>(key);
    expect(detail?.job.updated_at).toBe(T1);
    expect(detail?.proposals.find((p) => p.id === change[0].id)?.decision).toBe('accepted');
    // What the page renders follows (observers hear of it on the next tick)
    await waitFor(() => expect(result.current.query.data?.job.updated_at).toBe(T1));
  });

  it('a poll read before the PATCH but answered after it is ignored; a newer one lands', async () => {
    const { before, patched, client } = setup();
    const key = assignmentKeys.job(101);
    client.setQueryData(key, patched);
    global.fetch = jest.fn(async () => answer(200, before)) as typeof fetch;
    await client.fetchQuery({ ...assignmentJobQuery(101), staleTime: 0 });
    expect(client.getQueryData<AssignmentJobDetail>(key)?.job.updated_at).toBe(T1);

    const done = { ...at(patched, '2026-10-15T12:00:02Z'), job: { ...patched.job, status: 'needs_review' as const, updated_at: '2026-10-15T12:00:02Z' } };
    global.fetch = jest.fn(async () => answer(200, done)) as typeof fetch;
    await client.fetchQuery({ ...assignmentJobQuery(101), staleTime: 0 });
    expect(client.getQueryData<AssignmentJobDetail>(key)?.job.status).toBe('needs_review');
  });
});
