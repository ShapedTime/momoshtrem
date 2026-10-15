import { render, screen, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MappingJobsBanner } from '@/components/mapping';
import type { AssignmentJob } from '@/lib/api/contract';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/** Activity's banner for open mapping jobs (#5): one banner, a compact list for several, nothing for none. */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

function seededJob(id: number): AssignmentJob {
  const job = seedJobs().get(id);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error(`No seeded job ${id}`);
  return jobDetail(job, { entry, ai_available: true, currentFile: () => null }).job;
}

function stubJobs(items: AssignmentJob[]) {
  const urls: string[] = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return answer(200, { items });
  }) as typeof fetch;
  return urls;
}

afterEach(() => {
  global.fetch = originalFetch;
});

function renderBanner() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <div data-testid="slot">
        <MappingJobsBanner />
      </div>
    </QueryClientProvider>
  );
}

describe('MappingJobsBanner', () => {
  it('one open job: a review banner with its files to check, linking to the review', async () => {
    const urls = stubJobs([seededJob(101)]);
    renderBanner();
    expect(await screen.findByText('Frieren: 2 files to check')).toBeInTheDocument();
    expect(screen.getByText('[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/101');
    expect(urls).toEqual(['/api/assignment-jobs?status=open']);
  });

  it('several: a compact list; a running job says it’s mapping files', async () => {
    const running: AssignmentJob = { ...seededJob(102), id: 104, status: 'classifying' };
    stubJobs([running, seededJob(103), seededJob(101)]);
    renderBanner();
    const section = await screen.findByRole('region', { name: 'Episode mappings to check' });
    const rows = within(section).getAllByRole('listitem');
    expect(rows.map((row) => row.querySelector('.font-medium')?.textContent)).toEqual([
      'One Piece: mapping files…',
      'Doctor Who: 3 files to check',
      'Frieren: 2 files to check',
    ]);
    expect(within(rows[0]).getByRole('link', { name: 'Review mapping for One Piece' })).toHaveAttribute('href', '/mapping/104');
    expect(within(rows[2]).getByRole('link', { name: 'Review mapping for Frieren' })).toHaveAttribute('href', '/mapping/101');
  });

  it('a job with nothing left to decide is ready to apply', async () => {
    const ready = seededJob(101);
    stubJobs([{ ...ready, status: 'ready', counts: { ...ready.counts, needs_review: 0, pending: 0 } }]);
    renderBanner();
    expect(await screen.findByText('Frieren: ready to apply')).toBeInTheDocument();
  });

  it('nothing when no job is open', async () => {
    const urls = stubJobs([]);
    renderBanner();
    await screen.findByTestId('slot');
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(urls).toHaveLength(1);
    expect(screen.getByTestId('slot')).toBeEmptyDOMElement();
  });
});
