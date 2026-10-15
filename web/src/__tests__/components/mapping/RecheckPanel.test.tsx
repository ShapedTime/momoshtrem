import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MappingReview } from '@/components/mapping';
import { ShellContext } from '@/components/shell/ShellContext';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { AssignmentJobDetail, Proposal } from '@/lib/api/contract';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/**
 * "Re-check with AI" (#5): the panel's states against a scripted backend.
 * POST reclassify answers 202 and puts the job in `resolving`; `finish()`
 * ends the run as the worker would, and the panel's poll picks it up.
 */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

function seeded(id: number, aiAvailable = true): AssignmentJobDetail {
  const job = seedJobs().get(id);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error(`No seeded job ${id}`);
  return jobDetail(job, { entry, ai_available: aiAvailable, currentFile: () => null });
}

let clock = Date.parse('2026-10-15T12:00:00Z');
const tick = () => new Date((clock += 1000)).toISOString();

interface Backend {
  calls: Array<{ method: string; url: string; body?: unknown }>;
  /** The worker ends the run: `ids` change (hinted when the run had a hint), or `aiError` is set and nothing does. */
  finish: (change?: { ids?: number[]; aiError?: string }) => void;
}

function stubBackend(detail: AssignmentJobDetail, reclassify: { status: number; code?: string } = { status: 202 }): Backend {
  let current = detail;
  let hinted = false;
  const calls: Backend['calls'] = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, url, body });
    if (url.endsWith('/reclassify')) {
      if (reclassify.status !== 202) return answer(reclassify.status, { error: { code: reclassify.code ?? 'unavailable', message: 'No AI here.' } });
      hinted = Boolean((body as { hint?: string }).hint);
      current = {
        job: { ...current.job, status: 'resolving', ai_error: null, updated_at: tick() },
        proposals: current.proposals.map((p) => ({ ...p, updated: false, hinted: false })),
      };
      return answer(202, { job_id: detail.job.id });
    }
    if (url.includes('/episodes')) return answer(200, { items: [] });
    if (url.endsWith(`/assignment-jobs/${detail.job.id}`)) return answer(200, current);
    return answer(404, { error: { code: 'not_found', message: 'No such thing' } });
  }) as typeof fetch;
  return {
    calls,
    finish: ({ ids = [], aiError }: { ids?: number[]; aiError?: string } = {}) => {
      const touched = (p: Proposal): Proposal =>
        aiError || !ids.includes(p.id) ? p : { ...p, updated: true, hinted, source: 'ai', confidence: 'high', reason: 'Matched by air date' };
      current = {
        job: { ...current.job, status: 'needs_review', ai_error: aiError ?? null, updated_at: tick() },
        proposals: current.proposals.map(touched),
      };
    },
  };
}

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

const panel = () => screen.getByRole('region', { name: 'Re-check with AI' });
/** The run's poll is every 1.5 s. */
const POLL = { timeout: 4000 };

async function openPanel() {
  const toggle = await screen.findByRole('button', { name: /^Re-check with AI$/ });
  expect(toggle).toHaveAttribute('aria-expanded', 'false');
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'true');
}

describe('Re-check with AI', () => {
  it('is hidden when the job has no AI', async () => {
    stubBackend(seeded(101, false));
    renderPage(101);
    await screen.findByRole('heading', { name: 'Check the episodes' });
    expect(screen.queryByRole('region', { name: 'Re-check with AI' })).not.toBeInTheDocument();
  });

  it('collapsed, open, working while the groups stay usable, then Updated rows and the done line', async () => {
    const detail = seeded(101);
    const backend = stubBackend(detail);
    renderPage(101);
    await openPanel();
    const box = within(panel());
    expect(box.getByText('Not quite right?')).toBeInTheDocument();
    const hint = box.getByLabelText('Hint for Noir (optional)');
    expect(hint).toHaveAttribute('placeholder', 'e.g. these are absolute numbers');
    // The contract's scopes: pending and not decided by you (2 to review, 1 unmatched); everything not decided by you
    const scopes = box.getByRole('group', { name: 'Which files to re-check' });
    expect(within(scopes).getByRole('button', { name: 'Unresolved · 3' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(scopes).getByRole('button', { name: 'All files · 16' })).toHaveAttribute('aria-pressed', 'false');

    // A chip fills the hint; pressing it again clears it
    const chip = box.getByRole('button', { name: '28.5 is a recap' });
    fireEvent.click(chip);
    expect(hint).toHaveValue('28.5 is a recap');
    expect(chip).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(chip);
    expect(hint).toHaveValue('');
    fireEvent.click(chip);

    // Enter in the hint runs it
    fireEvent.keyDown(hint, { key: 'Enter' });
    const working = await box.findByRole('status');
    expect(working).toHaveTextContent('Re-checking 3 files with your hint…');
    expect(working).toHaveTextContent('“28.5 is a recap”');
    expect(within(working).getByRole('list')).toHaveTextContent(/Read file names.*Comparing with TMDB air dates.*Updating suggestions/);
    expect(backend.calls.find((call) => call.url.endsWith('/reclassify'))?.body).toEqual({ scope: 'unresolved', hint: '28.5 is a recap' });
    // The groups stay usable meanwhile
    expect(screen.getByRole('button', { name: 'Accept #35v2' })).toBeEnabled();

    const changed = detail.proposals.filter((p) => p.decision === 'pending').map((p) => p.id);
    backend.finish({ ids: changed });
    await waitFor(() => expect(within(panel()).getByRole('status')).toHaveTextContent('Updated 3 suggestions, marked Updated below. Accept the ones that look right.'), POLL);
    expect(screen.getByRole('button', { name: /^Re-check with AI$/ })).toHaveAttribute('aria-expanded', 'false');
    const recap = screen.getByText(/Frieren - 28\.5/).closest('li');
    if (!recap) throw new Error('No 28.5 row');
    expect(within(recap).getByText('Updated')).toBeInTheDocument();
    expect(within(recap).getByText('AI, with your hint:')).toBeInTheDocument();
  });

  it('without a hint, says what Noir compares; all files go when you pick that scope', async () => {
    const backend = stubBackend(seeded(101));
    renderPage(101);
    await openPanel();
    fireEvent.click(within(panel()).getByRole('button', { name: 'All files · 16' }));
    fireEvent.click(within(panel()).getByRole('button', { name: /^Re-check$/ }));
    const working = await within(panel()).findByRole('status');
    expect(working).toHaveTextContent('Re-checking 16 files…');
    expect(working).toHaveTextContent('Comparing names, air dates and runtimes');
    expect(backend.calls.find((call) => call.url.endsWith('/reclassify'))?.body).toEqual({ scope: 'all' });
    backend.finish();
    await waitFor(() => expect(within(panel()).getByRole('status')).toHaveTextContent('Noir looked again and kept every suggestion.'), POLL);
  });

  it('a failed AI run (ai_error) is a warning in the panel', async () => {
    const backend = stubBackend(seeded(101));
    renderPage(101);
    await openPanel();
    fireEvent.click(within(panel()).getByRole('button', { name: /^Re-check$/ }));
    await within(panel()).findByText('Re-checking 3 files…');
    backend.finish({ ids: [], aiError: 'The AI didn’t answer in time.' });
    await waitFor(() => expect(within(panel()).getByRole('status')).toHaveTextContent('The re-check didn’t finish. Your suggestions are unchanged. The AI didn’t answer in time.'), POLL);
    expect(within(panel()).queryByText(/^Updated/)).not.toBeInTheDocument();
  });

  it('503 is a calm line, and the panel stays open', async () => {
    stubBackend(seeded(101), { status: 503 });
    renderPage(101);
    await openPanel();
    fireEvent.click(within(panel()).getByRole('button', { name: /^Re-check$/ }));
    expect(await within(panel()).findByText(/AI isn’t available right now/)).toBeInTheDocument();
    expect(within(panel()).getByLabelText('Hint for Noir (optional)')).toBeVisible();
  });

  it('409 job_working waits for the re-check already running', async () => {
    const detail = seeded(101);
    stubBackend(detail, { status: 409, code: 'job_working' });
    renderPage(101);
    await openPanel();
    // Another tab started one: the job is resolving when this one asks
    const working: AssignmentJobDetail = { ...detail, job: { ...detail.job, status: 'resolving', updated_at: tick() } };
    (global.fetch as jest.Mock).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/reclassify')) return answer(409, { error: { code: 'job_working', message: 'Noir is still on it.' } });
      if (url.includes('/episodes')) return answer(200, { items: [] });
      return answer(200, init?.method ? undefined : working);
    });
    fireEvent.click(within(panel()).getByRole('button', { name: /^Re-check$/ }));
    expect(await within(panel()).findByText('Re-checking 3 files…')).toBeInTheDocument();
  });
});
