import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { KeepSheet } from '@/components/title/KeepSheet';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import type { DownloadEstimate, MediaType } from '@/lib/api/contract';
import { buildTitle, seedLibrary } from '@/mocks/fixtures/titles';

/**
 * The Keep sheet (#206) behind a fetch stub (jsdom has no fetch for MSW): the
 * size line from the estimate, what blocks a Keep before and after it's sent,
 * the durations POST and PATCH send, and the backend before /downloads (#202).
 */

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn(), back: jest.fn(), forward: jest.fn(), refresh: jest.fn(), prefetch: jest.fn() }),
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

interface Call {
  method: string;
  path: string;
  body: unknown;
}
let calls: Call[];

function stub({ estimate = answer(200, FITS), create = answer(201, {}), keep = true }: { estimate?: Response; create?: Response; keep?: boolean } = {}) {
  calls = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost');
    const method = init?.method ?? 'GET';
    calls.push({ method, path: url.pathname, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const title = /^\/api\/titles\/(movie|tv)\/(\d+)$/.exec(url.pathname);
    if (title) {
      const built = buildTitle(title[1] as MediaType, Number(title[2]), seedLibrary());
      // The backend before #204 sends no `keep`
      if (built && !keep) delete (built.library as Partial<typeof built.library>).keep;
      return answer(200, built);
    }
    if (url.pathname === '/api/settings/storage') return answer(503, { error: { code: 'unavailable', message: 'No storage governor' } });
    if (url.pathname === '/api/downloads/estimate') return estimate;
    if (url.pathname === '/api/downloads' && method === 'POST') return create;
    if (/^\/api\/downloads\/\d+$/.test(url.pathname) && method === 'PATCH') return answer(200, { reason: 'kept', state: 'done', until_watched: false, expires_at: null });
    return answer(404, { error: { code: 'not_found', message: `No fixture for ${url.pathname}` } });
  }) as typeof fetch;
}

afterEach(() => {
  global.fetch = originalFetch;
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

describe('KeepSheet', () => {
  it('says what a Keep needs, starts on until watched, and leaves the default to the server', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=693134');
    expect(await screen.findByText('Needs 18.4 GB · 289 GB left for kept downloads')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: /Until I watch it/ })).toBeChecked();
    fireEvent.click(keepButton());
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ scope: 'movie', id: 1 });
  });

  it('sends another duration from now', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=693134');
    fireEvent.click(await screen.findByRole('radio', { name: /Until I remove it/ }));
    fireEvent.click(keepButton());
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ scope: 'movie', id: 1, until_watched: false, expires_at: null });
  });

  it('hides the size line when the backend has no /downloads yet (404), and reads a missing keep as none', async () => {
    stub({ estimate: answer(404, { error: { code: 'not_found', message: 'Not found' } }), keep: false });
    renderSheet('sheet=keep&movie=693134');
    expect(await screen.findByRole('radio', { name: /Until I watch it/ })).toBeChecked();
    await waitFor(() => expect(calls.some((c) => c.path === '/api/downloads/estimate')).toBe(true));
    expect(screen.queryByText(/left for kept downloads/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Couldn’t check/)).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Download & keep' })).toBeInTheDocument();
    expect(keepButton()).toBeEnabled();
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
    fireEvent.click(keepButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('This plays from HDRezka, so it can’t be kept on disk.');
  });

  it('changes a kept download’s duration with PATCH, once one is picked', async () => {
    stub();
    const close = renderSheet('sheet=keep&movie=157336');
    expect(await screen.findByRole('heading', { name: 'Kept on disk' })).toBeInTheDocument();
    expect(screen.getByText(/^Kept until watched · by /)).toBeInTheDocument();
    const change = screen.getByRole('button', { name: 'Change duration' });
    expect(change).toBeDisabled();
    expect(calls.some((c) => c.path === '/api/downloads/estimate')).toBe(false);
    fireEvent.click(screen.getByRole('radio', { name: /Until I remove it/ }));
    fireEvent.click(change);
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ path: '/api/downloads/5', body: { until_watched: false, expires_at: null } });
    expect(screen.getByRole('button', { name: 'Stop keeping' })).toBeInTheDocument();
  });
});
