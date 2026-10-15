import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HDRezkaSheet } from '@/components/hdrezka';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { MediaType } from '@/lib/api/contract';
import { clearDraft } from '@/lib/hdrezka/session';
import { buildHDRezkaTitle, buildStreams, findSeed, searchHDRezka, titlePath } from '@/mocks/fixtures/hdrezka';
import { systemStatus } from '@/mocks/fixtures/system';
import { buildTitle, type MockLibrary, seedLibrary } from '@/mocks/fixtures/titles';

/**
 * The HDRezka tab of the source sheet (#67), step by step, against the mock
 * fixtures (src/mocks/fixtures/hdrezka.ts) behind a fetch stub: jsdom has no
 * fetch for MSW to intercept.
 */

const BATTLE = 'https://rezka.ag/films/action/78655-bitva-za-bitvoy-2025.html';

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as unknown as Response;

interface Call {
  method: string;
  path: string;
  body: unknown;
}

let library: MockLibrary;
let calls: Call[];
/** Answers a request before the fixtures do (errors, empty results). */
let override: ((url: URL, method: string) => Response | undefined) | null;

function fixtures(url: URL, method: string): Response {
  const q = url.searchParams;
  const title = /^\/api\/titles\/(movie|tv)\/(\d+)$/.exec(url.pathname);
  if (url.pathname === '/api/status') return answer(200, systemStatus());
  if (title) return answer(200, buildTitle(title[1] as MediaType, Number(title[2]), library));
  if (url.pathname === '/api/hdrezka/search') {
    const items = searchHDRezka(q.get('media_type') as MediaType, Number(q.get('tmdb_id')), q.get('q'));
    return answer(200, { items, query: q.get('q') ?? items[0]?.original_title ?? '' });
  }
  const seed = findSeed(titlePath(q.get('url') ?? '') ?? '');
  if (url.pathname === '/api/hdrezka/titles') {
    if (!seed) return answer(404, { error: { code: 'not_found', message: 'No title at that address on HDRezka.' } });
    return answer(200, buildHDRezkaTitle(seed, library, { translatorId: null, showId: null, movieId: Number(q.get('movie_id')) || null, everyPremium: false }));
  }
  if (url.pathname === '/api/hdrezka/streams' && seed) {
    const translator = seed.translators.find((t) => t.id === Number(q.get('translator_id')));
    if (translator) return answer(200, buildStreams(seed, translator));
  }
  if (method === 'POST' && /^\/api\/movies\/\d+\/assign-hdrezka$/.test(url.pathname)) return answer(204);
  return answer(404, { error: { code: 'not_found', message: `No fixture for ${url.pathname}` } });
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
  override = null;
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost');
    const method = init?.method ?? 'GET';
    calls.push({ method, path: `${url.pathname}${url.search}`, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    return override?.(url, method) ?? fixtures(url, method);
  }) as typeof fetch;
});

afterEach(() => {
  global.fetch = originalFetch;
  for (const key of ['movie:1054867', 'movie:157336', 'movie:693134']) clearDraft(key);
});

function renderSheet(query: string) {
  const close = jest.fn();
  const view = render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <ConfirmProvider>
          <HDRezkaSheet params={new URLSearchParams(query)} close={close} />
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
  return { close, ...view };
}

const searches = () => calls.filter((call) => call.path.startsWith('/api/hdrezka/search'));

describe('HDRezkaSheet', () => {
  it('match → voice-over → quality → Use HDRezka', async () => {
    const { close } = renderSheet('sheet=hdrezka&movie=1054867');
    const dialog = await screen.findByRole('dialog', { name: 'Find a source · One Battle After Another 2025 · 2h 42m' });
    expect(within(dialog).getByRole('tab', { name: 'HDRezka' })).toHaveAttribute('aria-selected', 'true');
    expect(within(dialog).getByRole('button', { name: 'Close (your choices are kept)' })).toBeInTheDocument();
    expect(screen.getByRole('searchbox', { name: 'Search HDRezka, or paste a link' })).toHaveValue('One Battle After Another');

    // The best match leads, with its reasons and the only primary button; the weak one says so
    const best = await screen.findByRole('article', { name: 'Битва за битвой (2025)' });
    expect(within(best).getByText('Best match')).toBeInTheDocument();
    expect(within(best).getByText('Exact title')).toBeInTheDocument();
    expect(within(best).getByText('Rated 7.9')).toBeInTheDocument();
    expect(dialog.querySelectorAll('.btn-primary')).toHaveLength(1);
    expect(screen.getByRole('article', { name: 'Битва полов (2017)' })).toHaveTextContent('Weak match: check it’s the right movie');
    expect(screen.queryByRole('button', { name: /Use HDRezka/ })).not.toBeInTheDocument();
    fireEvent.click(within(best).getByRole('button', { name: 'Use Битва за битвой (2025)' }));

    // The page's default voice-over and the best quality without Premium come first
    const studio = await screen.findByRole('radio', { name: 'HDrezka Studio' });
    expect(studio).toBeChecked();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Voice-over' })).toHaveFocus());
    expect(screen.getByRole('radio', { name: 'Дубляж, Premium' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'Дубляж, Premium' })).toHaveAccessibleDescription(/Premium account/);
    expect(screen.getByRole('radio', { name: 'TVShows, Ads' })).toBeEnabled();
    expect(screen.getByRole('radio', { name: 'Red Head Sound, CAMRip' })).toBeEnabled();
    expect(screen.getByRole('radio', { name: 'Jaskier, Director’s cut' })).toBeEnabled();
    expect(await screen.findByRole('radio', { name: '1080p, 4.3 GB' })).toBeChecked();
    expect(screen.getByRole('radio', { name: '1080p Ultra, Premium' })).toBeDisabled();
    expect(screen.getByText('No subtitles with this voice-over')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('radio', { name: 'Оригинал (+субтитры)' }));
    expect(await screen.findByText('Subtitles: English, Русский')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('radio', { name: '720p, 2.7 GB' }));
    expect(screen.getByText('Оригинал (+субтитры) · 720p · 2.7 GB')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Use HDRezka' }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((call) => call.method === 'POST')).toEqual({
      method: 'POST',
      path: '/api/movies/3/assign-hdrezka',
      body: { url: BATTLE, translator_id: 238, quality: '720p' },
    });
    expect(await screen.findByText('Streaming from HDRezka')).toBeInTheDocument();
    expect(screen.getByText('One Battle After Another · ready in Infuse')).toBeInTheDocument();
  });

  it('shows HDRezka’s bot check as an error with Retry and the backend’s message', async () => {
    override = (url) =>
      url.pathname === '/api/hdrezka/search'
        ? answer(503, { error: { code: 'hdrezka_blocked', message: 'HDRezka answered with its anti-bot page instead of the title.' } })
        : undefined;
    renderSheet('sheet=hdrezka&movie=1054867');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('HDRezka is showing its bot check');
    expect(alert).toHaveTextContent('HDRezka answered with its anti-bot page instead of the title.');
    override = null;
    fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
    expect(await screen.findByRole('article', { name: 'Битва за битвой (2025)' })).toBeInTheDocument();
    expect(searches()).toHaveLength(2);
  });

  it('suggests a shorter query, the title and torrents when nothing matches', async () => {
    const { close } = renderSheet('sheet=hdrezka&movie=1054867');
    await screen.findByRole('article', { name: 'Битва за битвой (2025)' });
    const field = screen.getByRole('searchbox', { name: 'Search HDRezka, or paste a link' });
    fireEvent.change(field, { target: { value: 'zzz Battle 2025' } });
    fireEvent.submit(field);
    expect(await screen.findByRole('heading', { name: 'No match for “zzz Battle 2025”' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search “One Battle After Another”' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try “zzz Battle”' }));
    await waitFor(() => expect(searches().at(-1)?.path).toContain('q=zzz+Battle'));
    expect(field).toHaveValue('zzz Battle');
    fireEvent.click(await screen.findByRole('button', { name: 'Try torrents' }));
    expect(close).toHaveBeenCalled();
  });

  it('goes straight to the title for a pasted link', async () => {
    renderSheet('sheet=hdrezka&movie=1054867');
    const field = await screen.findByRole('searchbox', { name: 'Search HDRezka, or paste a link' });
    fireEvent.paste(field, { clipboardData: { getData: () => ' https://hdrezka.me/films/fiction/2259-interstellar-2014.html ' } });
    expect(await screen.findByRole('radio', { name: 'Дубляж' })).toBeChecked();
    expect(screen.getByRole('region', { name: 'Match' })).toHaveTextContent('Интерстеллар');
    expect(field).toHaveValue('https://hdrezka.me/films/fiction/2259-interstellar-2014.html');
  });

  it('picks up where it left off after closing', async () => {
    const first = renderSheet('sheet=hdrezka&movie=157336');
    fireEvent.click(await screen.findByRole('button', { name: 'Use Интерстеллар (2014)' }));
    fireEvent.click(await screen.findByRole('radio', { name: 'Оригинал (+субтитры)' }));
    first.unmount();

    renderSheet('sheet=hdrezka&movie=157336');
    expect(await screen.findByText('Picked up where you left off')).toBeInTheDocument();
    expect(await screen.findByRole('radio', { name: 'Оригинал (+субтитры)' })).toBeChecked();
  });

  it('asks before a stream replaces a torrent, saying Infuse may lose its place', async () => {
    renderSheet('sheet=hdrezka&movie=693134');
    fireEvent.click(await screen.findByRole('button', { name: 'Use Дюна: Часть вторая (2024)' }));
    await screen.findByRole('radio', { name: '1080p, 4.5 GB' });
    fireEvent.click(screen.getByRole('button', { name: 'Use HDRezka' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Stream Dune: Part Two from HDRezka?' });
    expect(confirm).toHaveTextContent('It may show up as a new file (it ends in .mp4), so Infuse may forget where you stopped.');
    await act(async () => fireEvent.click(within(confirm).getByRole('button', { name: 'Cancel' })));
    expect(calls.some((call) => call.method === 'POST')).toBe(false);
  });

  it('shows Premium-only answers inline, keeping the sheet open', async () => {
    override = (url, method) =>
      method === 'POST' ? answer(409, { error: { code: 'premium_only', message: 'This needs an HDRezka Premium account.' } }) : undefined;
    const { close } = renderSheet('sheet=hdrezka&movie=1054867');
    fireEvent.click(await screen.findByRole('button', { name: 'Use Битва за битвой (2025)' }));
    await screen.findByRole('radio', { name: '1080p, 4.3 GB' });
    fireEvent.click(screen.getByRole('button', { name: 'Use HDRezka' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('This needs HDRezka Premium');
    expect(close).not.toHaveBeenCalled();
  });

  it('leaves for torrents from the Torrents tab', async () => {
    const { close } = renderSheet('sheet=hdrezka&movie=1054867');
    fireEvent.click(await screen.findByRole('tab', { name: 'Torrents' }));
    expect(close).toHaveBeenCalled();
  });
});
