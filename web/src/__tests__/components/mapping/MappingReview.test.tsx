import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MappingReview } from '@/components/mapping';
import { ShellContext } from '@/components/shell/ShellContext';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { AssignmentJobDetail, ProposalsPatch } from '@/lib/api/contract';
import { withDecisions } from '@/lib/mapping/review';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

function seeded(id: number): AssignmentJobDetail {
  const job = seedJobs().get(id);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error(`No seeded job ${id}`);
  return jobDetail(job, { entry, ai_available: true, currentFile: () => null });
}

/** The job, the picker's catalog and PATCH (which answers the job with its decisions). */
function stubBackend(detail: AssignmentJobDetail) {
  let current = detail;
  const calls: Array<{ method: string; url: string; body?: unknown }> = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, url, body });
    if (method === 'PATCH') {
      current = { ...current, proposals: withDecisions(current.proposals, (body as ProposalsPatch).items) };
      return answer(200, current);
    }
    if (url.includes('/episodes')) return answer(200, { items: [] });
    if (url.endsWith(`/assignment-jobs/${detail.job.id}`)) return answer(200, current);
    return answer(404, { error: { code: 'not_found', message: 'No such thing' } });
  }) as typeof fetch;
  return calls;
}

// Desktop presentation: the pane, the pre-selected first pending row
beforeAll(() => {
  window.matchMedia = ((query: string) => ({
    matches: query.includes('min-width: 1024px'),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
});

afterEach(() => {
  global.fetch = originalFetch;
});

function renderPage(jobId: number) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ShellContext.Provider value={{ openPalette: jest.fn(), setOverArt: jest.fn() }}>
        <ToastProvider>
          <ConfirmProvider>
            <MappingReview jobId={jobId} />
          </ConfirmProvider>
        </ToastProvider>
      </ShellContext.Provider>
    </QueryClientProvider>
  );
}

const apply = () => screen.getByRole('button', { name: /^Apply \d+ mappings?$/ });

describe('MappingReview', () => {
  it('hands focus on when the button you pressed goes away: Ignore → Undo, Accept all → the first Accepted', async () => {
    stubBackend(seeded(101));
    renderPage(101);
    const ignore = await screen.findByRole('button', { name: 'Ignore Special Extra' });
    ignore.focus();
    fireEvent.click(ignore);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Undo: map Special Extra again' })).toHaveFocus());

    const acceptAll = screen.getByRole('button', { name: 'Accept all 2' });
    acceptAll.focus();
    fireEvent.click(acceptAll);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toHaveFocus());
    expect(screen.queryByRole('button', { name: /^Accept all/ })).not.toBeInTheDocument();
  });

  it('accepts at once and sends the batch PATCH; the foot follows', async () => {
    const calls = stubBackend(seeded(101));
    renderPage(101);
    fireEvent.click(await screen.findByRole('button', { name: 'Accept #35v2' }));
    expect(await screen.findByRole('button', { name: 'Accepted: #35v2. Tap to undo' })).toHaveAttribute('aria-pressed', 'true');
    const patch = calls.find((call) => call.method === 'PATCH');
    expect(patch?.url).toBe('/api/assignment-jobs/101/proposals');
    expect(patch?.body).toEqual({ items: [{ id: expect.any(Number), decision: 'accepted' }] });
    await waitFor(() => expect(screen.getByText('Decide on 1 file to apply')).toBeInTheDocument());
    expect(apply()).toHaveAttribute('aria-describedby', screen.getByText('Decide on 1 file to apply').closest('p')?.id);
    // On desktop the selection moves on to the next file to decide
    expect(within(screen.getByRole('complementary', { name: 'Episode picker' })).getByText(/28\.5/)).toBeInTheDocument();
  });

  it('a disabled Apply applies nothing and focuses the first conflict', async () => {
    const calls = stubBackend(seeded(103));
    renderPage(103);
    await screen.findByText('Two files point at the same episode. Keep one.');
    expect(apply()).toHaveAttribute('aria-disabled', 'true');
    await act(async () => {
      fireEvent.click(apply());
    });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Change episode for S00E02' })).toHaveFocus());
    expect(calls.some((call) => call.url.endsWith('/apply'))).toBe(false);
  });

  it('a missing job is the not-found view', async () => {
    stubBackend(seeded(101));
    renderPage(999);
    expect(await screen.findByRole('heading', { name: 'Nothing here but the cat' })).toBeInTheDocument();
  });
});
