import type { UpNextItem, WatchlistItem } from '@/lib/api/contract';
import { heroPicks, localDateKey, MAX_HERO_PICKS } from '@/lib/for-you/hero';

function upNext(tmdbId: number, availability: UpNextItem['availability']): UpNextItem {
  return {
    show: { tmdb_id: tmdbId, title: `Show ${tmdbId}`, poster_path: null, backdrop_path: null, library_id: 1 },
    next_episode: { season: 1, episode: 27, absolute_number: null, name: 'An Ordinary Mage', air_date: '2026-10-01', still_path: null, tmdb_id: null },
    availability,
    airs_at: null,
    progress: { completed: 26, aired: 28 },
    last_watched_at: '2026-10-15T20:00:00.000Z',
    source: 'progress',
    webdav_path: availability === 'ready' ? '/TV Shows/x.mkv' : null,
    webdav_url: null,
    download: null,
    review_job_id: null,
  };
}

function watch(tmdbId: number, stage: 'ready' | 'needs_torrent' | null, mediaType: 'movie' | 'tv' = 'movie'): WatchlistItem {
  return {
    media_type: mediaType,
    tmdb_id: tmdbId,
    title: `Title ${tmdbId}`,
    year: 2024,
    poster_path: null,
    listed_at: '2026-10-01T00:00:00.000Z',
    library: stage && { library_id: tmdbId, availability_stage: stage, episodes_total: null, episodes_ready: null },
  };
}

const trending = Array.from({ length: 12 }, (_, i) => ({ media_type: (i % 2 ? 'movie' : 'tv') as 'movie' | 'tv', tmdb_id: 1000 + i }));
const keys = (picks: ReturnType<typeof heroPicks>) => picks.map((pick) => `${pick.source}:${pick.media_type}:${pick.tmdb_id}`);

describe('heroPicks', () => {
  it('takes one ready Up next show, then ready watchlist titles, then trending: three at most', () => {
    const picks = heroPicks({
      upNext: [upNext(1, 'needs_torrent'), upNext(2, 'ready'), upNext(3, 'ready')],
      watchlist: [watch(10, 'ready'), watch(11, 'needs_torrent'), watch(12, null)],
      trending,
      date: '2026-10-16',
    });
    expect(picks).toHaveLength(MAX_HERO_PICKS);
    expect(picks.map((pick) => pick.source)).toEqual(['up_next', 'watchlist', 'trending']);
    expect([2, 3]).toContain(picks[0].tmdb_id);
    expect(picks[0]).toMatchObject({ media_type: 'tv', kicker: 'Ready in Infuse', reason: 'Up next: S01E27 · An Ordinary Mage. It’s ready to play in Infuse.' });
    expect(picks[1]).toMatchObject({ media_type: 'movie', tmdb_id: 10, kicker: 'From your watchlist' });
    expect(picks[2]).toMatchObject({ kicker: 'Trending this week', reason: '' });
  });

  it('is stable for a day, whatever order the lists come in', () => {
    const input = { upNext: [upNext(2, 'ready'), upNext(3, 'ready'), upNext(4, 'ready')], watchlist: [watch(10, 'ready'), watch(20, 'ready')], trending };
    const today = heroPicks({ ...input, date: '2026-10-16' });
    const again = heroPicks({ upNext: [...input.upNext].reverse(), watchlist: [...input.watchlist].reverse(), trending: [...trending].reverse(), date: '2026-10-16' });
    expect(keys(again)).toEqual(keys(today));
    expect(keys(heroPicks({ ...input, date: new Date(2026, 9, 16, 23, 59) }))).toEqual(keys(today));
  });

  it('changes from one day to the next', () => {
    const input = { upNext: [], watchlist: [], trending };
    const days = Array.from({ length: 7 }, (_, i) => keys(heroPicks({ ...input, date: `2026-10-${String(10 + i).padStart(2, '0')}` })).join());
    expect(new Set(days).size).toBeGreaterThan(1);
  });

  it('takes trending picks from the top of the list and never repeats a title', () => {
    const picks = heroPicks({ upNext: [upNext(1000, 'ready')], watchlist: [watch(1001, 'ready')], trending, date: '2026-10-16' });
    expect(new Set(keys(picks).map((key) => key.split(':').slice(1).join(':'))).size).toBe(picks.length);
    // tv:1000 is the Up next pick; movie:1001 the watchlist one; neither comes back as trending
    expect(picks.filter((pick) => pick.source === 'trending').every((pick) => pick.tmdb_id >= 1002 && pick.tmdb_id < 1010)).toBe(true);
  });

  it('without Trakt, the hero is trending only; with nothing at all, it’s empty', () => {
    expect(heroPicks({ upNext: [], watchlist: [], trending, date: '2026-10-16' }).every((pick) => pick.source === 'trending')).toBe(true);
    expect(heroPicks({ upNext: [upNext(1, 'needs_torrent')], watchlist: [watch(2, 'needs_torrent')], trending: [], date: '2026-10-16' })).toEqual([]);
  });

  it('keys the day on the local calendar', () => {
    expect(localDateKey(new Date(2026, 0, 5, 0, 30))).toBe('2026-01-05');
    expect(localDateKey(new Date(2026, 11, 31, 23, 59))).toBe('2026-12-31');
  });
});
