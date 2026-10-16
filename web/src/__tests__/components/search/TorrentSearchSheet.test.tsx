import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TorrentSearchSheet } from '@/components/search/TorrentSearchSheet';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { MediaType } from '@/lib/api/contract';
import { clearDraft } from '@/lib/search/session';
import { jobDetail, newJob } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';
import { findMockRelease, searchFixture } from '@/mocks/fixtures/search';
import { parseSearchParams, searchAnswer } from '@/mocks/fixtures/search-rules';
import { QUALITY_PROFILE } from '@/mocks/fixtures/settings';
import { systemStatus } from '@/mocks/fixtures/system';
import { buildSeason, buildTitle, type MockLibrary, seedLibrary } from '@/mocks/fixtures/titles';

/**
 * The torrent search sheet (#92) against the search fixtures and rules
 * (src/mocks/fixtures/search*.ts) behind a fetch stub: jsdom has no fetch for
 * MSW to intercept.
 */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as unknown as Response;

interface Call {
  method: string;
  path: string;
  query: URLSearchParams;
  body: unknown;
}

let library: MockLibrary;
let calls: Call[];

function fixtures(url: URL, method: string, body: unknown): Response {
  const title = /^\/api\/titles\/(movie|tv)\/(\d+)$/.exec(url.pathname);
  const season = /^\/api\/titles\/tv\/(\d+)\/seasons\/(\d+)$/.exec(url.pathname);
  if (url.pathname === '/api/status') return answer(200, systemStatus());
  if (title) return answer(200, buildTitle(title[1] as MediaType, Number(title[2]), library));
  if (season) return answer(200, buildSeason(Number(season[1]), Number(season[2]), library));
  if (url.pathname === '/api/search/torrents') {
    const parsed = parseSearchParams(url.searchParams);
    if ('error' in parsed) return answer(400, { error: { code: 'bad_request', message: parsed.error } });
    const { params } = parsed;
    const fixture = searchFixture(params.media_type, params.tmdb_id, params);
    const show = buildTitle(params.media_type, params.tmdb_id, library);
    if (!fixture) return answer(404, { error: { code: 'not_found', message: 'No such title' } });
    return answer(
      200,
      searchAnswer({
        parsed,
        found: fixture.releases.filter((entry) => !params.q || entry.release.raw_title.includes(params.q)),
        profile: QUALITY_PROFILE,
        queries: params.q ? [{ q: params.q, label: null }] : fixture.queries,
        indexers: fixture.indexers.map((name) => ({ name, status: 'ok' as const, ms: 200 })),
        complete: true,
        missingRun: show?.media_type === 'tv' ? show.library.missing : null,
        cachedAt: '2026-10-14T00:00:00Z',
      })
    );
  }
  if (method === 'POST' && url.pathname === '/api/search/torrents/resolve') {
    const entry = findMockRelease((body as { release_id: string }).release_id);
    return entry?.resolves ? answer(200, entry.resolves) : answer(404, { error: { code: 'not_found', message: 'Expired' } });
  }
  if (method === 'POST' && /^\/api\/(shows|movies)\/\d+\/assign-torrent$/.test(url.pathname)) return answer(202, { job_id: 104 });
  if (method === 'POST' && url.pathname === '/api/search/torrents/inspect') {
    // A release without a magnet: its .torrent can't be fetched
    const entry = findMockRelease((body as { release_id: string }).release_id);
    if (entry?.resolves) return answer(502, { error: { code: 'torrent_unavailable', message: 'The indexer answered 500 for the .torrent file.' } });
    return answer(202, { job_id: 900 });
  }
  if (url.pathname === '/api/assignment-jobs/900') return answer(200, frierenDryRun());
  return answer(404, { error: { code: 'not_found', message: `No fixture for ${url.pathname}` } });
}

/** Inspect's job: Frieren's batch as a classified dry run (fixtures/assignment.ts). */
function frierenDryRun() {
  const job = newJob({ id: 900, media_type: 'tv', tmdb_id: 209867, library_id: 1, magnet_uri: 'magnet:?xt=urn:btih:' + 'a'.repeat(40), info_hash: 'a'.repeat(40), dry_run: true, ai: true, now: 0 });
  Object.assign(job, { walk: null, reached: 4, ai_files: job.first_run_files, status: 'needs_review' });
  return jobDetail(job, { entry: findTitle('tv', 209867)!, ai_available: true, currentFile: () => null });
}

beforeAll(() => {
  // Desktop presentation (Radix Dialog panel); jsdom can't run Vaul's drag physics.
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

beforeEach(() => {
  library = seedLibrary();
  calls = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost');
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, path: url.pathname, query: url.searchParams, body });
    return fixtures(url, method, body);
  }) as typeof fetch;
});

afterEach(() => {
  global.fetch = originalFetch;
  for (const key of ['show:209867:1:29', 'movie:1054867']) clearDraft(key);
});

function renderSheet(query: string) {
  const close = jest.fn();
  const view = render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <ConfirmProvider>
          <TorrentSearchSheet params={new URLSearchParams(query)} close={close} />
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
  return { close, ...view };
}

const searches = () => calls.filter((call) => call.path === '/api/search/torrents');
const BATCH = '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]';
const FRIEREN = 'sheet=torrent&show=209867&se=S01E29';

describe('TorrentSearchSheet', () => {
  it('opens on the episode: the guessed query, the scope, then the best match and the other results', async () => {
    renderSheet(FRIEREN);
    const dialog = await screen.findByRole('dialog', { name: /^Find a source · Frieren: Beyond Journey’s End/ });
    expect(within(dialog).getByRole('tab', { name: 'Torrents' })).toHaveAttribute('aria-selected', 'true');
    // The first frame already has the query: the short title and the absolute number
    expect(within(dialog).getByRole('searchbox', { name: 'Search releases' })).toHaveValue('Frieren 29');
    expect(within(dialog).getByRole('button', { name: 'Episode' })).toHaveAttribute('aria-pressed', 'true');

    const best = await screen.findByRole('article', { name: BATCH });
    expect(best).toHaveClass('is-best');
    expect(within(best).getByText(/Best match/)).toHaveTextContent('Best match · score 96');
    expect(within(best).getByText('Covers all 10 missing episodes')).toBeInTheDocument();
    expect(within(best).getByText('Episodes 29–38')).toHaveClass('chip--pack');
    expect(within(best).getByRole('button', { name: 'Add & review mapping' })).toHaveClass('btn-primary');
    expect(dialog.querySelectorAll('.btn-primary')).toHaveLength(1);

    expect(screen.getByText('27 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();
    expect(screen.getByText('Also searched: Sousou no Frieren 29 · Frieren S02E01 (scene numbering)')).toBeInTheDocument();
    expect(screen.getByText('10 episodes are missing')).toBeInTheDocument();
    expect(screen.getByText('A pack gets 29–38 in one go. The best match below already does.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Other results · sorted by best' })).toBeInTheDocument();
    expect(screen.getByText('Showing 25 of 27')).toBeInTheDocument();

    // q isn't sent until the user types
    expect(searches()[0].query.get('q')).toBeNull();
    expect(searches()[0].query.get('scope')).toBe('episode');
  });

  it('loads more, switches scope and sends a typed query', async () => {
    renderSheet(FRIEREN);
    await screen.findByRole('article', { name: BATCH });
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    expect(await screen.findByText('Showing 27 of 27')).toBeInTheDocument();
    expect(searches().at(-1)?.query.get('page')).toBe('2');
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Season packs' }));
    await waitFor(() => expect(searches().at(-1)?.query.get('scope')).toBe('season'));
    expect(await screen.findByText('13 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Season pack' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByText('10 episodes are missing')).not.toBeInTheDocument();

    const field = screen.getByRole('searchbox', { name: 'Search releases' });
    fireEvent.change(field, { target: { value: 'Sousou' } });
    fireEvent.submit(field);
    await waitFor(() => expect(searches().at(-1)?.query.get('q')).toBe('Sousou'));
    expect(field).toHaveValue('Sousou');
  });

  it('adds through the mapping job with the episode hint and the indexer, resolving a magnet first when there is none', async () => {
    renderSheet(FRIEREN);
    const best = await screen.findByRole('article', { name: BATCH });
    fireEvent.click(within(best).getByRole('button', { name: 'Add & review mapping' }));
    await waitFor(() => expect(calls.some((call) => call.path === '/api/shows/1/assign-torrent')).toBe(true));
    expect(calls.find((call) => call.path === '/api/shows/1/assign-torrent')?.body).toEqual({
      magnet_uri: findMockRelease('rel-0')?.release.magnet_uri,
      hint: { season: 1, episode: 29 },
      indexer: 'Nyaa',
    });

    calls = [];
    const fetchNeeded = screen.getByRole('article', { name: /S01E29\.Lets\.Go\.Somewhere/ });
    fireEvent.click(within(fetchNeeded).getByRole('button', { name: 'Add & review mapping' }));
    await waitFor(() => expect(calls.some((call) => call.path.endsWith('/assign-torrent'))).toBe(true));
    const posts = calls.filter((call) => call.method === 'POST');
    expect(posts.map((call) => call.path)).toEqual(['/api/search/torrents/resolve', '/api/shows/1/assign-torrent']);
    expect((posts[1].body as { magnet_uri: string }).magnet_uri).toContain('dn=Frieren.Beyond.Journeys.End.S01E29');
  });

  it('copies a magnet and says so', async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    renderSheet('sheet=torrent&movie=1054867');
    const best = await screen.findByRole('article', { name: 'One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX' });
    expect(within(best).getByRole('button', { name: 'Add & assign to movie' })).toBeInTheDocument();
    fireEvent.click(within(best).getByRole('button', { name: 'Copy magnet link' }));
    expect(await screen.findByText('Magnet link copied')).toBeInTheDocument();
    expect(writeText).toHaveBeenCalledWith(expect.stringMatching(/^magnet:\?xt=urn:btih:[0-9a-f]{40}&dn=/));
    // A movie has no scope switch, and the profile hid the HDCAM
    expect(screen.queryByRole('group', { name: 'Search scope' })).not.toBeInTheDocument();
    expect(screen.queryByRole('article', { name: /HDCAM/ })).not.toBeInTheDocument();
    expect(screen.getByRole('article', { name: /CAMRip/ })).toHaveTextContent('CAM recording · filmed in a cinema');
  });

  it('picks up where it left off after closing', async () => {
    const first = renderSheet(FRIEREN);
    await screen.findByRole('article', { name: BATCH });
    fireEvent.click(screen.getByRole('button', { name: 'Season pack' }));
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search releases' }), { target: { value: 'Frieren batch' } });
    first.unmount();

    renderSheet(FRIEREN);
    expect(await screen.findByText('Picked up where you left off')).toBeInTheDocument();
    expect(screen.getByRole('searchbox', { name: 'Search releases' })).toHaveValue('Frieren batch');
    expect(screen.getByRole('button', { name: 'Season pack' })).toHaveAttribute('aria-pressed', 'true');
    await waitFor(() => expect(searches().at(-1)?.query.get('q')).toBe('Frieren batch'));
  });

  it('filters from a quick chip and sorts from the Sort sheet, in the search’s params', async () => {
    renderSheet(FRIEREN);
    await screen.findByRole('article', { name: BATCH });
    const chips = screen.getByRole('group', { name: 'Filter and sort' });
    fireEvent.click(within(chips).getByRole('button', { name: /^2160p/ }));
    await waitFor(() => expect(searches().at(-1)?.query.getAll('resolution')).toEqual(['2160p']));
    expect(await screen.findByText('3 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();
    expect(within(chips).getByRole('button', { name: /^2160p/ })).toHaveAttribute('aria-pressed', 'true');
    expect(within(chips).getByRole('button', { name: 'Filters, 1 on' })).toBeInTheDocument();

    fireEvent.click(within(chips).getByRole('button', { name: 'Sort: Best' }));
    const sort = await screen.findByRole('dialog', { name: 'Sort by' });
    fireEvent.click(within(sort).getByRole('button', { name: /^Seeders/ }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Sort by' })).not.toBeInTheDocument());
    await waitFor(() => expect(searches().at(-1)?.query.get('sort')).toBe('seeders'));
    expect(await screen.findByRole('heading', { name: 'Results · sorted by seeders' })).toBeInTheDocument();
    expect(within(chips).getByRole('button', { name: 'Sort: Seeders' })).toBeInTheDocument();
    expect(screen.queryByText(/Best match/)).not.toBeInTheDocument();
  });

  it('counts the Filters sheet’s draft with page_size=1, and applies it only on Show', async () => {
    renderSheet(FRIEREN);
    await screen.findByRole('article', { name: BATCH });
    fireEvent.click(screen.getByRole('button', { name: 'Filters' }));
    const sheet = await screen.findByRole('dialog', { name: 'Filters' });
    expect(within(sheet).getByRole('button', { name: 'Show 27 results' })).toBeInTheDocument();
    fireEvent.click(within(sheet).getByRole('button', { name: /^Season \d/ }));
    await waitFor(() => expect(within(sheet).getByRole('button', { name: 'Show 7 results' })).toBeInTheDocument());
    const count = searches().at(-1);
    expect(count?.query.get('page_size')).toBe('1');
    expect(count?.query.getAll('pack')).toEqual(['season']);
    // Nothing applied yet
    expect(screen.getByText('27 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();

    fireEvent.click(within(sheet).getByRole('button', { name: 'Show 7 results' }));
    expect(await screen.findByText('7 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();
    expect(searches().at(-1)?.query.get('page_size')).toBe('25');
  });

  it('names the filters that hide everything; removing one brings results back', async () => {
    renderSheet(FRIEREN);
    await screen.findByRole('article', { name: BATCH });
    fireEvent.click(screen.getByRole('button', { name: 'Filters' }));
    const sheet = await screen.findByRole('dialog', { name: 'Filters' });
    fireEvent.click(within(sheet).getByRole('button', { name: /^2160p/ }));
    fireEvent.click(within(sheet).getByRole('button', { name: '100+' }));
    fireEvent.click(await within(sheet).findByRole('button', { name: 'Show 0 results' }));
    expect(await screen.findByRole('heading', { name: 'Your filters hide all 27 results' })).toBeInTheDocument();
    expect(screen.getByText(/together match nothing for this episode/)).toHaveTextContent('2160p and 100+ seeders together match nothing for this episode.');

    fireEvent.click(within(screen.getByRole('group', { name: 'Active filters' })).getByRole('button', { name: '100+ seeders' }));
    expect(await screen.findByText('3 results · Nyaa, TorrentLeech, The Pirate Bay')).toBeInTheDocument();
  });

  it('inspects a release: the request, the dry run’s groups', async () => {
    renderSheet(FRIEREN);
    const best = await screen.findByRole('article', { name: BATCH });
    fireEvent.click(within(best).getByRole('button', { name: 'Inspect' }));
    const sheet = await screen.findByRole('dialog', { name: /^Inspect · nothing downloads yet/ });
    await waitFor(() => expect(calls.some((call) => call.path === '/api/search/torrents/inspect')).toBe(true));
    expect(calls.find((call) => call.path === '/api/search/torrents/inspect')?.body).toEqual({
      media_type: 'tv',
      tmdb_id: 209867,
      release_id: 'rel-0',
      hint: { season: 1, episode: 29 },
    });
    expect(await within(sheet).findByRole('heading', { name: 'Will map · 9 episodes' })).toBeInTheDocument();
    expect(within(sheet).getByText('+ 5 more, all high confidence')).toBeInTheDocument();
    expect(within(sheet).getByRole('heading', { name: 'You’ll review · 3 files' })).toBeInTheDocument();
    expect(within(sheet).getByText('Asked AI about 3 tricky ones')).toBeInTheDocument();
    expect(within(sheet).getByRole('button', { name: 'Add & review mapping' })).toHaveClass('btn-primary');
    // Read-only: nothing writes to the dry run
    expect(calls.filter((call) => call.path.startsWith('/api/assignment-jobs/900/'))).toEqual([]);
  });

  it('says why Inspect couldn’t start, inline, with Retry', async () => {
    renderSheet(FRIEREN);
    const fetchNeeded = await screen.findByRole('article', { name: /S01E29\.Lets\.Go\.Somewhere/ });
    fireEvent.click(within(fetchNeeded).getByRole('button', { name: 'Inspect' }));
    const sheet = await screen.findByRole('dialog', { name: /^Inspect/ });
    const alert = await within(sheet).findByRole('alert');
    expect(alert).toHaveTextContent('Couldn’t fetch the torrent');
    expect(alert).toHaveTextContent('The indexer answered 500 for the .torrent file.');
    fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(calls.filter((call) => call.path === '/api/search/torrents/inspect')).toHaveLength(2));
  });

  it('says what the quality profile hid when nothing else is left', async () => {
    renderSheet('sheet=torrent&movie=1054867');
    await screen.findByRole('article', { name: /CAMRip/ });
    const field = screen.getByRole('searchbox', { name: 'Search releases' });
    fireEvent.change(field, { target: { value: 'HDCAM' } });
    fireEvent.submit(field);
    expect(await screen.findByRole('heading', { name: 'No releases for “HDCAM”' })).toBeInTheDocument();
    expect(screen.getByText(/1 hidden by your quality profile/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Quality settings' })).toHaveAttribute('href', '/settings?section=quality');
  });
});
