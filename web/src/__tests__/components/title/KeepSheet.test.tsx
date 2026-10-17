import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { KeepSheet } from '@/components/title/KeepSheet';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { DownloadEstimate, MediaType, Title } from '@/lib/api/contract';
import { buildSeason, buildTitle, seedLibrary } from '@/mocks/fixtures/titles';

/**
 * The Keep sheet (#206) behind a fetch stub (jsdom has no fetch for MSW): the
 * size line from the estimate, what blocks a Keep before and after it's sent,
 * the durations POST and PATCH send, Stop keeping, an episode its season
 * holds, and a backend without downloads (before #202 and #204).
 */

const mockPush = jest.fn();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: jest.fn(), back: jest.fn(), forward: jest.fn(), refresh: jest.fn(), prefetch: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/movie/693134',
}));

// Desktop presentation (Radix Dialog panel); jsdom can't run Vaul's drag physics.
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

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as unknown as Response;

const FITS: DownloadEstimate = { wanted_bytes: 18_400_000_000, needed_bytes: 18_400_000_000, available_bytes: 289_000_000_000, downloadable: true };
const STORAGE = { kept: { max_bytes: null, effective_max_bytes: 375_000_000_000, default_until_watched: true, default_days: 30 } };

interface Call {
  method: string;
  path: string;
  body: unknown;
}
let calls: Call[];

interface StubOptions {
  estimate?: Response;
  create?: Response;
  /** /settings/storage: the kept defaults, or a failure. */
  storage?: Response;
  /** False: the backend before #202 and #204, which sends no `keep`. */
  keep?: boolean;
}

function stub({ estimate = answer(200, FITS), create = answer(201, {}), storage = answer(200, STORAGE), keep = true }: StubOptions = {}) {
  calls = [];
  let released = false;
  const withoutKeep = (title: Title) => {
    if (!keep) delete (title.library as Partial<typeof title.library>).keep;
    // Stop keeping went through: the title's download is gone
    else if (released) title.library.keep = null;
    return title;
  };
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost');
    const method = init?.method ?? 'GET';
    calls.push({ method, path: url.pathname, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const title = /^\/api\/titles\/(movie|tv)\/(\d+)$/.exec(url.pathname);
    if (title) {
      const built = buildTitle(title[1] as MediaType, Number(title[2]), seedLibrary());
      return answer(200, built && withoutKeep(built));
    }
    const season = /^\/api\/titles\/tv\/(\d+)\/seasons\/(\d+)$/.exec(url.pathname);
    if (season) return answer(200, buildSeason(Number(season[1]), Number(season[2]), seedLibrary()));
    if (url.pathname === '/api/settings/storage') return storage;
    if (url.pathname === '/api/downloads/estimate') return estimate;
    if (url.pathname === '/api/downloads' && method === 'POST') return create;
    if (/^\/api\/downloads\/\d+$/.test(url.pathname) && method === 'PATCH') return answer(200, { reason: 'kept', state: 'done', until_watched: false, expires_at: null });
    if (/^\/api\/downloads\/\d+$/.test(url.pathname) && method === 'DELETE') {
      released = true;
      return answer(204);
    }
    return answer(404, { error: { code: 'not_found', message: `No fixture for ${url.pathname}` } });
  }) as typeof fetch;
}

afterEach(() => {
  global.fetch = originalFetch;
  mockPush.mockReset();
});

function renderSheet(query: string, close = jest.fn()) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <ConfirmProvider>
          <KeepSheet params={new URLSearchParams(query)} close={close} />
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
  return close;
}

const keepButton = () => screen.getByRole('button', { name: 'Download & keep' });
const estimates = () => calls.filter((c) => c.path === '/api/downloads/estimate').length;

describe('KeepSheet', () => {
  it('says what a Keep needs, starts on until watched, and leaves the default to the server', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=693134');
    expect(await screen.findByText('Needs 18.4 GB · 289 GB left for kept downloads')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: /Until I watch it/ })).toBeChecked();
    await waitFor(() => expect(keepButton()).toBeEnabled());
    fireEvent.click(keepButton());
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ scope: 'movie', id: 1 });
  });

  it('sends another duration from now', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=693134');
    fireEvent.click(await screen.findByRole('radio', { name: /Until I remove it/ }));
    await waitFor(() => expect(keepButton()).toBeEnabled());
    fireEvent.click(keepButton());
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ scope: 'movie', id: 1, until_watched: false, expires_at: null });
  });

  it('spells the default out when the storage defaults can’t be read, and waits for them before', async () => {
    let answerStorage: (response: Response) => void = () => {};
    const pending = new Promise<Response>((resolve) => (answerStorage = resolve));
    stub();
    const fetchStub = global.fetch;
    global.fetch = jest.fn((input: RequestInfo | URL, init?: RequestInit) =>
      String(input).includes('/api/settings/storage') ? pending : fetchStub(input, init)
    ) as typeof fetch;
    const close = renderSheet('sheet=keep&movie=693134');
    await screen.findByText(/left for kept downloads/);
    // The choice shown may not be the server's default yet
    expect(keepButton()).toBeDisabled();
    answerStorage(answer(503, { error: { code: 'unavailable', message: 'No storage governor' } }));
    await waitFor(() => expect(keepButton()).toBeEnabled());
    fireEvent.click(keepButton());
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ scope: 'movie', id: 1, until_watched: true, expires_at: expect.any(String) });
  });

  it('hides the size line when the estimate answers 404', async () => {
    stub({ estimate: answer(404, { error: { code: 'not_found', message: 'Not found' } }) });
    renderSheet('sheet=keep&movie=693134');
    expect(await screen.findByRole('radio', { name: /Until I watch it/ })).toBeChecked();
    await waitFor(() => expect(estimates()).toBe(1));
    expect(screen.queryByText(/left for kept downloads/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Couldn’t check/)).not.toBeInTheDocument();
    await waitFor(() => expect(keepButton()).toBeEnabled());
  });

  it('keeps nothing on a backend without downloads (no `keep` field), and says so', async () => {
    stub({ keep: false });
    renderSheet('sheet=keep&movie=693134');
    expect(await screen.findByText('This momoshtrem can’t keep things on disk yet. Everything still streams to Infuse on demand.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download & keep' })).not.toBeInTheDocument();
    expect(estimates()).toBe(0);
  });

  it('says a link that names nothing names nothing', async () => {
    stub();
    renderSheet('sheet=keep&show=abc');
    expect(await screen.findByText('This link doesn’t point to anything that can be kept on disk.')).toBeInTheDocument();
  });

  it('says it doesn’t fit before it’s sent, with a link to Settings › Storage', async () => {
    stub({ estimate: answer(200, { ...FITS, available_bytes: 1_200_000_000 }) });
    renderSheet('sheet=keep&movie=693134');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Not enough space: needs 18.4 GB, 1.2 GB left.');
    expect(alert).toHaveTextContent('Free some kept downloads or raise the limit in Settings › Storage.');
    expect(screen.getByRole('link', { name: 'Settings › Storage' })).toHaveAttribute('href', '/settings?section=storage');
    expect(keepButton()).toBeDisabled();
  });

  it('shows POST’s 409 storage_full in place', async () => {
    stub({ create: answer(409, { error: { code: 'storage_full', message: 'Not enough space', needed_bytes: 18_400_000_000, available_bytes: 2_000_000_000 } }) });
    renderSheet('sheet=keep&movie=693134');
    await screen.findByText(/left for kept downloads/);
    await waitFor(() => expect(keepButton()).toBeEnabled());
    fireEvent.click(keepButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Not enough space: needs 18.4 GB, 2.0 GB left.');
  });

  it('explains that an HDRezka stream can’t be kept', async () => {
    stub({ estimate: answer(200, { ...FITS, downloadable: false }) });
    renderSheet('sheet=keep&movie=693134');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Can’t keep this file');
    expect(alert).toHaveTextContent('This plays from HDRezka, so it can’t be kept on disk. Pick a torrent for it to keep it.');
    expect(keepButton()).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Find a torrent' })).toBeInTheDocument();
  });

  it('shows POST’s 422 not_downloadable when the estimate didn’t know', async () => {
    stub({ create: answer(422, { error: { code: 'not_downloadable', message: 'HDRezka' } }) });
    renderSheet('sheet=keep&movie=693134');
    await screen.findByText(/left for kept downloads/);
    await waitFor(() => expect(keepButton()).toBeEnabled());
    fireEvent.click(keepButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('This plays from HDRezka, so it can’t be kept on disk.');
  });

  it('changes a kept download’s duration with PATCH, once one is picked', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=157336');
    expect(await screen.findByRole('heading', { name: 'Kept', exact: true })).toBeInTheDocument();
    expect(screen.getByText(/^Kept until watched · by /)).toBeInTheDocument();
    const change = screen.getByRole('button', { name: 'Change duration' });
    expect(change).toBeDisabled();
    expect(estimates()).toBe(0);
    fireEvent.click(screen.getByRole('radio', { name: /Until I remove it/ }));
    fireEvent.click(change);
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ path: '/api/downloads/5', body: { until_watched: false, expires_at: null } });
  });

  it('stays as it was while Stop keeping goes through, with no new-Keep form or estimate', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=157336');
    fireEvent.click(await screen.findByRole('button', { name: 'Stop keeping' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Stop keeping Interstellar?' });
    fireEvent.click(within(confirm).getByRole('button', { name: 'Stop keeping' }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'DELETE')?.path).toBe('/api/downloads/5');
    // The refetch says it isn't kept any more; the closing sheet doesn't turn into a new Keep
    await waitFor(() => expect(calls.filter((c) => c.path === '/api/titles/movie/157336').length).toBeGreaterThan(1));
    expect(screen.getByRole('heading', { name: 'Kept', exact: true })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download & keep' })).not.toBeInTheDocument();
    expect(estimates()).toBe(0);
  });

  it('points an episode its season keeps to the season’s keep', async () => {
    stub();
    renderSheet('sheet=keep&show=82728&scope=episode&se=S03E01');
    expect(await screen.findByRole('heading', { name: 'Kept', exact: true })).toBeInTheDocument();
    expect(screen.getByText('Season 3 is kept, so this episode is too.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Change season 3’s keep' }));
    expect(mockPush).toHaveBeenCalledWith(expect.stringMatching(/[?&]sheet=keep&show=82728&scope=season&season=3$/), { scroll: false });
    expect(estimates()).toBe(0);
  });
});
