import type { Keep, MovieTitle, SeasonDetail, ShowTitle, Title } from '@/lib/api/contract';
import {
  changedToast,
  downloadsAvailable,
  episodeKeepContext,
  heldByAction,
  heldByContext,
  heldByCopy,
  KEEP_PARAMS,
  keepContext,
  keepSheetMode,
  keepSheetParams,
  keepSubject,
  keepTargetName,
  keptToast,
  stopKeepingConfirm,
  stoppedToast,
} from '@/lib/title/keep-sheet';
import { buildSeason, buildTitle, seedLibrary } from '@/mocks/fixtures/titles';

const NOW = new Date(2026, 9, 17, 12, 0);
const keep = (fields: Partial<Keep> = {}): Keep => ({
  download_id: 1,
  reason: 'kept',
  scope: 'season',
  state: 'done',
  pct: 100,
  until_watched: false,
  expires_at: null,
  ...fields,
});
const params = (query: string) => new URLSearchParams(query);

function title(mediaType: 'movie' | 'tv', tmdbId: number): Title {
  const built = buildTitle(mediaType, tmdbId, seedLibrary());
  if (!built) throw new Error(`No fixture for ${mediaType} ${tmdbId}`);
  return built;
}
function season(tmdbId: number, n: number): SeasonDetail {
  const built = buildSeason(tmdbId, n, seedLibrary());
  if (!built) throw new Error(`No season ${n} of ${tmdbId}`);
  return built;
}

describe('the Keep sheet’s URL', () => {
  it.each([
    [{ kind: 'movie', tmdbId: 157336 } as const, { movie: '157336' }],
    [{ kind: 'show', tmdbId: 82728 } as const, { show: '82728', scope: 'show' }],
    [{ kind: 'season', tmdbId: 82728, season: 3 } as const, { show: '82728', scope: 'season', season: '3' }],
    [{ kind: 'episode', tmdbId: 95396, season: 2, episode: 3 } as const, { show: '95396', scope: 'episode', se: 'S02E03' }],
  ])('round-trips %o', (context, expected) => {
    expect(keepSheetParams(context)).toEqual(expected);
    expect(keepContext(params(new URLSearchParams(expected).toString()))).toEqual(context);
  });

  it('lets `scope` decide, since the episode sheet’s `se` stays under a sheet it opens', () => {
    expect(keepContext(params('show=82728&se=S03E01&scope=season&season=3'))).toEqual({ kind: 'season', tmdbId: 82728, season: 3 });
    expect(keepContext(params('show=82728&se=S03E01&scope=show'))).toEqual({ kind: 'show', tmdbId: 82728 });
    expect(keepContext(params('show=82728'))).toEqual({ kind: 'show', tmdbId: 82728 });
  });

  it('opens on nothing for a bad link', () => {
    expect(keepContext(params(''))).toBeNull();
    expect(keepContext(params('movie=abc'))).toBeNull();
    expect(keepContext(params('show=82728&scope=season'))).toBeNull();
    expect(keepContext(params('show=82728&scope=episode&se=nope'))).toBeNull();
  });

  it('owns its params but the show page’s season tab', () => {
    expect(KEEP_PARAMS).not.toContain('season');
  });
});

describe('keepSubject', () => {
  it('keeps a movie by its library id, with its download', () => {
    const interstellar = title('movie', 157336) as MovieTitle;
    expect(keepSubject({ kind: 'movie', tmdbId: 157336 }, interstellar, undefined)).toEqual({
      status: 'ready',
      subject: { target: { scope: 'movie', id: 2 }, name: 'Interstellar', own: interstellar.library.keep, heldBy: null },
    });
  });

  it('reads a season’s own keep from the season', () => {
    const bluey = title('tv', 82728) as ShowTitle;
    const result = keepSubject({ kind: 'season', tmdbId: 82728, season: 3 }, bluey, season(82728, 3));
    expect(result).toMatchObject({
      status: 'ready',
      subject: { target: { scope: 'season', id: 5, season_number: 3 }, name: 'Bluey · Season 3', own: { download_id: 1, scope: 'season' }, heldBy: null },
    });
  });

  it('tells an episode’s own keep from the season or show keep that holds it', () => {
    const bluey = title('tv', 82728) as ShowTitle;
    const result = keepSubject({ kind: 'episode', tmdbId: 82728, season: 3, episode: 1 }, bluey, season(82728, 3));
    expect(result).toMatchObject({ status: 'ready', subject: { target: { scope: 'episode' }, name: 'Bluey · S03E01', own: null, heldBy: { download_id: 1, scope: 'season' } } });
  });

  it('says when it isn’t in the library, or the season doesn’t list the episode', () => {
    expect(keepSubject({ kind: 'movie', tmdbId: 666277 }, title('movie', 666277), undefined)).toEqual({ status: 'missing' });
    expect(keepSubject({ kind: 'episode', tmdbId: 82728, season: 3, episode: 999 }, title('tv', 82728), season(82728, 3))).toEqual({ status: 'missing' });
  });

  it('can’t keep anything on a backend without downloads (no `keep` field, before #202 and #204)', () => {
    const dune = title('movie', 693134) as MovieTitle;
    const before: MovieTitle = { ...dune, library: { ...dune.library, keep: undefined as unknown as null } };
    expect(keepSubject({ kind: 'movie', tmdbId: 693134 }, before, undefined)).toEqual({ status: 'unsupported' });
    const bluey = title('tv', 82728);
    const plain = season(82728, 3);
    const noKeeps: SeasonDetail = { ...plain, keep: undefined as unknown as null, episodes: plain.episodes.map((e) => ({ ...e, keep: undefined as unknown as null })) };
    expect(keepSubject({ kind: 'season', tmdbId: 82728, season: 3 }, bluey, noKeeps)).toEqual({ status: 'unsupported' });
    expect(keepSubject({ kind: 'episode', tmdbId: 82728, season: 3, episode: 1 }, bluey, noKeeps)).toEqual({ status: 'unsupported' });
  });

  it('tells a missing `keep` (no downloads) from `keep: null` (not kept)', () => {
    expect(downloadsAvailable({ keep: null })).toBe(true);
    expect(downloadsAvailable({ keep: keep() })).toBe(true);
    expect(downloadsAvailable({})).toBe(false);
    expect(downloadsAvailable(undefined)).toBe(false);
  });
});

describe('the sheet’s mode and copy', () => {
  it('picks the mode from the target’s own download, or the keep that holds an episode', () => {
    expect(keepSheetMode(null)).toBe('new');
    expect(keepSheetMode({ own: null, heldBy: null })).toBe('new');
    expect(keepSheetMode({ own: keep({ reason: 'smart', scope: 'episode' }), heldBy: null })).toBe('smart');
    expect(keepSheetMode({ own: keep(), heldBy: null })).toBe('manage');
    expect(keepSheetMode({ own: null, heldBy: keep() })).toBe('held');
  });

  it('opens the keep that holds an episode', () => {
    expect(heldByContext(82728, keep({ scope: 'season' }), 3)).toEqual({ kind: 'season', tmdbId: 82728, season: 3 });
    expect(heldByContext(82728, keep({ scope: 'show' }), 3)).toEqual({ kind: 'show', tmdbId: 82728 });
    expect(heldByAction(keep({ scope: 'season' }), 3)).toBe('Change season 3’s keep');
    expect(heldByAction(keep({ scope: 'season' }), 0)).toBe('Change the Specials keep');
    expect(heldByAction(keep({ scope: 'show' }), 3)).toBe('Change the show’s keep');
  });

  it('names seasons, specials and episodes', () => {
    expect(keepTargetName({ kind: 'season', tmdbId: 1, season: 0 }, 'Doctor Who')).toBe('Doctor Who · Specials');
    expect(keepTargetName({ kind: 'show', tmdbId: 1 }, 'Doctor Who')).toBe('Doctor Who');
  });

  it('opens an episode’s sheet on what holds it', () => {
    const episode = { season_number: 3, episode_number: 1 };
    expect(episodeKeepContext(82728, { ...episode, keep: keep({ scope: 'season' }) })).toEqual({ kind: 'season', tmdbId: 82728, season: 3 });
    expect(episodeKeepContext(82728, { ...episode, keep: keep({ scope: 'show' }) })).toEqual({ kind: 'show', tmdbId: 82728 });
    expect(episodeKeepContext(82728, { ...episode, keep: keep({ scope: 'episode' }) })).toEqual({ kind: 'episode', tmdbId: 82728, season: 3, episode: 1 });
    expect(episodeKeepContext(82728, { ...episode, keep: null })).toMatchObject({ kind: 'episode' });
  });

  it('says what holds an episode', () => {
    expect(heldByCopy(keep({ until_watched: true, expires_at: new Date(2026, 10, 16).toISOString() }), 2)).toMatch(/^Season 2 is kept until watched · by Nov 16, so this episode is too\.$/);
    expect(heldByCopy(keep({ scope: 'show' }), 2)).toBe('The whole show is kept, so this episode is too.');
    expect(heldByCopy(keep(), 0)).toBe('Specials are kept, so this episode is too.');
    // An HDRezka episode in a kept season: the keep can't hold it, so it doesn't say it does
    expect(heldByCopy(keep({ state: 'unavailable' }), 2)).toBeNull();
  });

  it('says what a Keep did, and what comes next', () => {
    expect(keptToast({ reason: 'kept', state: 'queued', until_watched: true, expires_at: null }, 'Dune: Part Two', NOW)).toEqual({
      title: 'Keeping Dune: Part Two on disk',
      body: 'Kept until watched. It downloads in the background.',
    });
    expect(keptToast({ reason: 'kept', state: 'done', until_watched: false, expires_at: null }, 'Bluey · Season 3', NOW).body).toBe('Kept. It’s already on disk.');
    expect(keptToast({ reason: 'kept', state: 'no_file', until_watched: true, expires_at: null }, 'X', NOW).body).toMatch(/once there’s a file/);
    expect(changedToast({ reason: 'kept', until_watched: false, expires_at: new Date(2026, 9, 24).toISOString() }, 'Interstellar', NOW)).toEqual({
      title: 'Changed how long Interstellar is kept',
      body: 'Kept until Oct 24',
    });
  });

  it('confirms Stop keeping with what goes and what stays; a smart pick is dismissed', () => {
    expect(stopKeepingConfirm({ reason: 'kept' }, 'Interstellar')).toEqual({
      title: 'Stop keeping Interstellar?',
      description: 'It’s no longer held on disk. What’s downloaded stays as cache and plays until it’s cleared to make room.',
      confirmLabel: 'Stop keeping',
    });
    expect(stopKeepingConfirm({ reason: 'smart' }, 'Frieren · S01E27')).toMatchObject({ title: 'Dismiss the smart pick for Frieren · S01E27?', confirmLabel: 'Dismiss' });
    expect(stoppedToast({ reason: 'kept' }, 'Interstellar').title).toBe('Stopped keeping Interstellar');
    expect(stoppedToast({ reason: 'smart' }, 'Frieren · S01E27').title).toBe('Dismissed the smart pick for Frieren · S01E27');
  });
});
