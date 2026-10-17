import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { EpisodeSheet } from '@/components/title/show';
import type { EpisodeSummary, SeasonDetail } from '@/lib/api/contract';
import { buildSeason, buildTitle, seedLibrary } from '@/mocks/fixtures/titles';

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn(), back: jest.fn(), forward: jest.fn(), refresh: jest.fn(), prefetch: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/tv/82728',
}));

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body }) as Response;

/** The seeded show and season behind a fetch stub, with one episode changed. */
function stubEpisode(tmdbId: number, seasonNumber: number, change: (episode: EpisodeSummary, season: SeasonDetail) => EpisodeSummary) {
  global.fetch = jest.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), 'http://localhost').pathname;
    if (path === `/api/titles/tv/${tmdbId}`) return answer(200, buildTitle('tv', tmdbId, seedLibrary()));
    if (path === `/api/titles/tv/${tmdbId}/seasons/${seasonNumber}`) {
      const season = buildSeason(tmdbId, seasonNumber, seedLibrary());
      if (!season) return answer(404);
      return answer(200, { ...season, episodes: season.episodes.map((e) => (e.episode_number === 1 ? change(e, season) : e)) });
    }
    return answer(404, { error: { code: 'not_found', message: `No fixture for ${path}` } });
  }) as typeof fetch;
}

function renderEpisode(query: string) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <ConfirmProvider>
          <EpisodeSheet params={new URLSearchParams(query)} close={jest.fn()} />
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

const hdrezka = (episode: EpisodeSummary): EpisodeSummary => ({
  ...episode,
  source: 'hdrezka',
  stream: { translator: 'LostFilm', quality: '1080p' } as EpisodeSummary['stream'],
});

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

afterEach(() => {
  global.fetch = originalFetch;
});

describe('EpisodeSheet', () => {
  it('says when the episode didn’t load, instead of “not in your library”, and retries', async () => {
    global.fetch = jest.fn(() => Promise.resolve(answer(502, { error: { code: 'tmdb_unavailable', message: 'TMDB isn’t answering.' } }))) as typeof fetch;
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ToastProvider>
          <ConfirmProvider>
            <EpisodeSheet params={new URLSearchParams('sheet=episode&tmdb=209867&se=S01E28')} close={jest.fn()} />
          </ConfirmProvider>
        </ToastProvider>
      </QueryClientProvider>
    );
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Couldn’t load this episode');
    expect(alert).toHaveTextContent('TMDB isn’t answering.');
    expect(screen.queryByText(/isn’t in your library/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2));
  });
});

describe('EpisodeSheet: Download & keep (#206)', () => {
  const keepSection = () => document.querySelector('section[aria-labelledby="episode-keep-h"]');

  it('offers Download & keep on a torrent file nothing keeps', async () => {
    stubEpisode(95396, 1, (episode) => episode);
    renderEpisode('sheet=episode&tmdb=95396&se=S01E01');
    expect(await screen.findByRole('heading', { name: 'Download & keep' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Download & keep' })).toBeInTheDocument();
  });

  it('offers none on an HDRezka file: it streams from the CDN, and a Keep would be refused', async () => {
    stubEpisode(95396, 1, (episode) => hdrezka(episode));
    renderEpisode('sheet=episode&tmdb=95396&se=S01E01');
    expect(await screen.findByText(/^Source:/)).toHaveTextContent('HDRezka');
    expect(keepSection()).toBeNull();
  });

  it('doesn’t say a kept season holds an HDRezka episode; it says why it can’t', async () => {
    stubEpisode(82728, 3, (episode) => ({ ...hdrezka(episode), keep: episode.keep && { ...episode.keep, state: 'unavailable', pct: 0 } }));
    renderEpisode('sheet=episode&tmdb=82728&se=S03E01');
    expect(await screen.findByRole('heading', { name: 'Kept', exact: true })).toBeInTheDocument();
    expect(screen.getByText('Can’t keep this file')).toBeInTheDocument();
    expect(screen.queryByText(/so this episode is too/)).not.toBeInTheDocument();
    expect(screen.getByText('This plays from HDRezka, so it can’t be kept on disk. Pick a torrent for it to keep it.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Change season 3’s keep' })).toBeInTheDocument();
  });

  it('offers none on a backend without downloads (no `keep` field)', async () => {
    stubEpisode(95396, 1, (episode) => {
      const without: Partial<EpisodeSummary> = { ...episode };
      delete without.keep;
      return without as EpisodeSummary;
    });
    renderEpisode('sheet=episode&tmdb=95396&se=S01E01');
    await screen.findByRole('heading', { name: 'Play in Infuse' });
    expect(keepSection()).toBeNull();
  });
});
