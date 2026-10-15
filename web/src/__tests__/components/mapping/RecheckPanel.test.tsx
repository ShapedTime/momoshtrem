import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MappingReview } from '@/components/mapping';
import { ShellContext } from '@/components/shell/ShellContext';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { AssignmentJobDetail, Proposal } from '@/lib/api/contract';
import { recheckCounts } from '@/lib/mapping/recheck';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/**
 * "Re-check with AI" (#5): the panel's states against a scripted backend.
 * POST reclassify answers 202 and puts the job in `resolving` as the server
 * does; `finish()` ends the run as the worker would, and the panel's poll
 * picks it up. Screen readers hear it all through one live region.
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
  /** Reads of the job fail (500) until `false`. */
  failReads: (fail: boolean) => void;
}

interface Script {
  /** The reclassify answer: 202 by default. */
  reclassify?: { status: number; code?: string };
  /** A 503 means AI went away: the job says so from then on. */
  aiGoesAway?: boolean;
}

function stubBackend(detail: AssignmentJobDetail, { reclassify = { status: 202 }, aiGoesAway = false }: Script = {}): Backend {
  let current = detail;
  let hinted = false;
  let failing = false;
  const calls: Backend['calls'] = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, url, body });
    if (url.endsWith('/reclassify')) {
      if (reclassify.status !== 202) {
        if (aiGoesAway) current = { ...current, job: { ...current.job, ai_available: false, updated_at: tick() } };
        return answer(reclassify.status, { error: { code: reclassify.code ?? 'unavailable', message: 'No AI here.' } });
      }
      const { scope, hint } = body as { scope: 'unresolved' | 'all'; hint?: string };
      hinted = Boolean(hint);
      current = {
        job: {
          ...current.job,
          status: 'resolving',
          ai_error: null,
          hint: hint ? { text: hint } : null,
          progress: { ...current.job.progress, ai_files: recheckCounts(current.proposals)[scope] },
          updated_at: tick(),
        },
        proposals: current.proposals.map((p) => ({ ...p, updated: false, hinted: false })),
      };
      return answer(202, { job_id: detail.job.id });
    }
    if (url.includes('/episodes')) return answer(200, { items: [] });
    if (url.endsWith(`/assignment-jobs/${detail.job.id}`)) {
      return failing ? answer(500, { error: { code: 'internal', message: 'Try again' } }) : answer(200, current);
    }
    return answer(404, { error: { code: 'not_found', message: 'No such thing' } });
  }) as typeof fetch;
  return {
    calls,
    failReads: (fail) => {
      failing = fail;
    },
    finish: ({ ids = [], aiError } = {}) => {
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
/** The one live region: it stays put while what it says changes. */
const live = () => within(panel()).getByRole('status');
const runButton = () => within(panel()).getByRole('button', { name: /^Re-check$/ });
/** The run's poll is every 1.5 s. */
const POLL = { timeout: 4000 };

async function openPanel() {
  const toggle = await screen.findByRole('button', { name: 'Re-check with AI' });
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
    const region = live();
    expect(region).toHaveAttribute('aria-live', 'polite');
    expect(region).toBeEmptyDOMElement();
    expect(box.getByText('Not quite right?')).toBeInTheDocument();
    const hint = box.getByLabelText('Hint for Noir (optional)');
    expect(hint).toHaveAttribute('placeholder', 'e.g. these are absolute numbers');
    // As the backend picks them: undecided episode files (13); pending or in a conflict (3)
    const scopes = box.getByRole('group', { name: 'Which files to re-check' });
    expect(within(scopes).getByRole('button', { name: 'Unresolved · 3' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(scopes).getByRole('button', { name: 'All files · 13' })).toHaveAttribute('aria-pressed', 'false');

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
    await waitFor(() => expect(region).toHaveTextContent('Re-checking 3 files with your hint… “28.5 is a recap”'));
    // The same live region, with the working state visible next to it
    expect(live()).toBe(region);
    expect(box.getByText('Re-checking 3 files with your hint…')).toBeVisible();
    expect(box.getByRole('list')).toHaveTextContent(/Read file names.*Comparing with TMDB air dates.*Updating suggestions/);
    expect(backend.calls.find((call) => call.url.endsWith('/reclassify'))?.body).toEqual({ scope: 'unresolved', hint: '28.5 is a recap' });
    // The groups stay usable meanwhile
    expect(screen.getByRole('button', { name: 'Accept #35v2' })).toBeEnabled();

    const changed = detail.proposals.filter((p) => p.decision === 'pending').map((p) => p.id);
    backend.finish({ ids: changed });
    await waitFor(() => expect(region).toHaveTextContent('Updated 3 suggestions, marked Updated below. Accept the ones that look right.'), POLL);
    expect(live()).toBe(region);
    expect(panel().querySelector('.map-ai-done')).toHaveTextContent('Updated 3 suggestions, marked Updated below.');
    expect(screen.getByRole('button', { name: 'Re-check with AI' })).toHaveAttribute('aria-expanded', 'false');
    const recap = screen.getByText(/Frieren - 28\.5/).closest('li');
    if (!recap) throw new Error('No 28.5 row');
    expect(within(recap).getByText('Updated')).toBeInTheDocument();
    expect(within(recap).getByText('AI, with your hint:')).toBeInTheDocument();
  });

  it('without a hint, says what Noir compares, and shows the server’s count once it works', async () => {
    const backend = stubBackend(seeded(101));
    renderPage(101);
    await openPanel();
    fireEvent.click(within(panel()).getByRole('button', { name: 'All files · 13' }));
    fireEvent.click(runButton());
    await waitFor(() => expect(live()).toHaveTextContent('Re-checking 13 files… Comparing names, air dates and runtimes'));
    expect(backend.calls.find((call) => call.url.endsWith('/reclassify'))?.body).toEqual({ scope: 'all' });
    backend.finish();
    await waitFor(() => expect(live()).toHaveTextContent('Noir looked again and kept every suggestion.'), POLL);
  });

  it('a failed AI run (ai_error) is a warning with a neutral lead and the backend’s words', async () => {
    const backend = stubBackend(seeded(101));
    renderPage(101);
    await openPanel();
    fireEvent.click(runButton());
    await within(panel()).findByText('Re-checking 3 files…');
    backend.finish({ ids: [], aiError: "The AI didn't answer in time, so nothing changed." });
    await waitFor(() => expect(live()).toHaveTextContent("Noir couldn’t finish. The AI didn't answer in time, so nothing changed."), POLL);
    expect(within(panel()).getByText('Noir couldn’t finish.')).toBeVisible();
    expect(within(panel()).queryByText(/^Updated/)).not.toBeInTheDocument();
  });

  it('an ai_error from the first run shows before any re-check', async () => {
    const detail = seeded(101);
    stubBackend({ ...detail, job: { ...detail.job, ai_error: 'The AI check failed, so nothing changed.' } });
    renderPage(101);
    await screen.findByRole('region', { name: 'Re-check with AI' });
    expect(panel().querySelector('.map-ai-note')).toHaveTextContent('Noir couldn’t finish. The AI check failed, so nothing changed.');
  });

  it('503: the panel stays, says why calmly and takes focus, though the job no longer has AI', async () => {
    stubBackend(seeded(101), { reclassify: { status: 503 }, aiGoesAway: true });
    renderPage(101);
    await openPanel();
    fireEvent.click(runButton());
    await waitFor(() => expect(panel().querySelector('.map-ai-note')).toHaveTextContent(/AI isn’t available right now/));
    await waitFor(() => expect(panel().querySelector('.map-ai-note')).toHaveFocus());
    expect(live()).toHaveTextContent('AI isn’t available right now');
    // Not "Noir can take another look" right above "Noir can’t take another look"
    expect(within(panel()).queryByText(/Noir can take another look/)).not.toBeInTheDocument();
    // The refetch says AI is gone: nothing to re-check with any more
    await waitFor(() => expect(within(panel()).queryByRole('button', { name: 'Re-check with AI' })).not.toBeInTheDocument());
    expect(panel()).toBeInTheDocument();
  });

  it('409 job_working waits for the run already going, with its own count and hint', async () => {
    const detail = seeded(101);
    stubBackend(detail, { reclassify: { status: 409, code: 'job_working' } });
    renderPage(101);
    await openPanel();
    // Another tab started one: the job is resolving when this one asks
    const elsewhere: AssignmentJobDetail = {
      ...detail,
      job: { ...detail.job, status: 'resolving', hint: { text: 'use TMDB air order' }, progress: { ...detail.job.progress, ai_files: 13 }, updated_at: tick() },
    };
    (global.fetch as jest.Mock).mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith('/reclassify')) return answer(409, { error: { code: 'job_working', message: 'Noir is still on it.' } });
      if (url.includes('/episodes')) return answer(200, { items: [] });
      return answer(200, elsewhere);
    });
    fireEvent.change(within(panel()).getByLabelText('Hint for Noir (optional)'), { target: { value: 'mine' } });
    fireEvent.click(runButton());
    await waitFor(() => expect(live()).toHaveTextContent('Re-checking 13 files with your hint… “use TMDB air order”'));
  });

  it('a failed read after the 202 doesn’t end the run: it keeps polling until the job settles', async () => {
    const detail = seeded(101);
    const backend = stubBackend(detail);
    renderPage(101);
    await openPanel();
    backend.failReads(true);
    fireEvent.click(runButton());
    await waitFor(() => expect(live()).toHaveTextContent('Re-checking 3 files…'));
    // Still working after the failed refetch
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(live()).toHaveTextContent('Re-checking 3 files…');
    backend.failReads(false);
    backend.finish({ ids: [detail.proposals[0].id] });
    await waitFor(() => expect(live()).toHaveTextContent('Updated 1 suggestion, marked Updated below.'), POLL);
  });

  it('Re-check is off when the scope has no files', async () => {
    const detail = seeded(101);
    // Every episode file decided by you: nothing for the AI to look at
    const decided = detail.proposals.map((p) => (p.kind === 'episode' ? { ...p, user_decided: true, decision: 'accepted' as const } : p));
    stubBackend({ ...detail, proposals: decided });
    renderPage(101);
    await openPanel();
    expect(within(panel()).getByRole('button', { name: 'Unresolved · 0' })).toBeInTheDocument();
    expect(within(panel()).getByRole('button', { name: 'All files · 0' })).toHaveAttribute('aria-pressed', 'true');
    expect(runButton()).toBeDisabled();
  });
});
