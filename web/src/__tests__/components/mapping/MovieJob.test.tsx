import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MappingReview } from '@/components/mapping';
import { ShellContext } from '@/components/shell/ShellContext';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { AssignmentJobDetail, ProposalsPatch } from '@/lib/api/contract';
import { withDecisions } from '@/lib/mapping/review';
import { jobDetail, newJob, settledStatus } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/** A movie job (#5): its candidates are one choice, Undo brings them back, and Applied names the file. */

const RELEASE = 'One.Battle.After.Another.2025.1080p.BluRay.x264-SPARKS';
const THEATRICAL = `${RELEASE}.mkv`;
const EXTENDED = `${RELEASE}.Extended.Cut.mkv`;

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

function movieJob(): AssignmentJobDetail {
  const entry = findTitle('movie', 1054867);
  if (!entry) throw new Error('No One Battle After Another');
  const job = newJob({
    id: 301,
    media_type: 'movie',
    tmdb_id: 1054867,
    library_id: 3,
    magnet_uri: `magnet:?xt=urn:btih:abc123&dn=${RELEASE}`,
    info_hash: 'abc123',
    ai: false,
    now: Date.parse('2026-10-15T12:00:00Z'),
  });
  job.walk = null;
  job.reached = 4;
  job.status = settledStatus(job.proposals);
  return jobDetail(job, { entry, ai_available: false, currentFile: () => null });
}

function stubBackend(detail: AssignmentJobDetail) {
  let current = detail;
  const patches: ProposalsPatch[] = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PATCH') {
      const body = JSON.parse(String(init.body)) as ProposalsPatch;
      patches.push(body);
      current = { ...current, proposals: withDecisions(current.proposals, body.items) };
      return answer(200, current);
    }
    if (url.endsWith(`/assignment-jobs/${detail.job.id}`)) return answer(200, current);
    if (url.includes('/titles/')) return answer(404, { error: { code: 'not_found', message: 'Not in this test' } });
    return answer(404, { error: { code: 'not_found', message: 'No such thing' } });
  }) as typeof fetch;
  return patches;
}

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

const row = (file: string) => {
  const found = screen.getAllByRole('listitem').find((item) => item.classList.contains('mrow') && item.textContent?.includes(file));
  if (!found) throw new Error(`No row for ${file}`);
  return found;
};

describe('a movie job', () => {
  it('is one choice: Use this file, the other steps aside, and Undo brings both back', async () => {
    const detail = movieJob();
    const [theatrical, extended] = detail.proposals.filter((p) => p.kind === 'movie');
    const patches = stubBackend(detail);
    renderPage(301);
    expect(await screen.findByRole('heading', { name: 'Check the files' })).toBeInTheDocument();
    // No AI for movies, no episodes to pick
    expect(screen.queryByRole('region', { name: 'Re-check with AI' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Change/ })).not.toBeInTheDocument();
    expect(screen.getByText('More than one file could be the movie. Use the right one; the others stay out of Infuse.')).toBeInTheDocument();
    expect(screen.getByText('Pick the file to use')).toBeInTheDocument();
    // Path, size, confidence and reason on each candidate
    expect(row(THEATRICAL)).toHaveTextContent(/14\.2 GB/);
    expect(row(THEATRICAL)).toHaveTextContent('Medium');
    expect(row(THEATRICAL)).toHaveTextContent('Title and year match; the runtime fits the theatrical cut');
    expect(row(EXTENDED)).toHaveTextContent('Low');

    fireEvent.click(screen.getByRole('button', { name: `Use this file: ${THEATRICAL}` }));
    const using = await screen.findByRole('button', { name: `Using this file: ${THEATRICAL}. Tap to undo` });
    expect(using).toHaveAttribute('aria-pressed', 'true');
    expect(patches[0]).toEqual({ items: [{ id: theatrical.id, decision: 'accepted' }] });
    // The other one steps aside, and can still be used instead
    await waitFor(() => expect(row(EXTENDED)).toHaveTextContent(/Not used\s*· not in Infuse/));
    expect(within(row(EXTENDED)).getByRole('button', { name: `Use this file: ${EXTENDED}` })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply 1 mapping' })).toHaveAttribute('aria-disabled', 'false');

    // Undo: both back to pending, in one PATCH
    fireEvent.click(using);
    await screen.findByRole('button', { name: `Use this file: ${THEATRICAL}` });
    expect(patches[1]).toEqual({
      items: [
        { id: theatrical.id, decision: 'pending' },
        { id: extended.id, decision: 'pending' },
      ],
    });
    await waitFor(() => expect(screen.getByText('Pick the file to use')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: `Use this file: ${EXTENDED}` })).toBeInTheDocument();
  });

  it('applied: "Ready in Infuse." with the file, no seasons', async () => {
    const detail = movieJob();
    const [theatrical] = detail.proposals.filter((p) => p.kind === 'movie');
    const proposals = withDecisions(detail.proposals, [{ id: theatrical.id, decision: 'accepted' }]);
    stubBackend({ job: { ...detail.job, status: 'applied', applied_at: '2026-10-15T12:01:00Z' }, proposals });
    renderPage(301);
    expect(await screen.findByRole('heading', { name: 'Ready in Infuse.' })).toBeInTheDocument();
    expect(screen.getByText('It streams while it downloads, and shows up after Infuse’s next library refresh.')).toBeInTheDocument();
    const done = screen.getByRole('list', { name: 'Mapped to Infuse' });
    const [file, ...rest] = within(done).getAllByRole('listitem');
    expect(file).toHaveTextContent('One Battle After Another');
    expect(file).toHaveTextContent(THEATRICAL);
    expect(done).not.toHaveTextContent(/Season|Specials/);
    expect(rest.map((item) => item.textContent)).toEqual([expect.stringMatching(/^Left out2 ignored/)]);
  });
});
