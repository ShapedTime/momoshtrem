import type { HDRezkaLibraryEpisode, HDRezkaSeason, StreamEpisodeTarget } from '@/lib/api/contract';
import { ApiError } from '@/lib/api/client';
import {
  allKeys,
  assignInChunks,
  chunk,
  defaultSeasonNumber,
  defaultSelection,
  episodeRowState,
  episodeSourceLabel,
  episodeTargets,
  missingKeys,
  sampleEpisode,
} from '@/lib/hdrezka/episodes';

const NOW = '2026-10-14';

const lib = (season: number, episode: number, patch: Partial<HDRezkaLibraryEpisode> = {}): HDRezkaLibraryEpisode => ({
  episode_id: season * 1000 + episode,
  season,
  episode,
  name: `Episode ${episode}`,
  air_date: '2026-01-01',
  has_file: false,
  source: null,
  stream: null,
  ...patch,
});

/** HDRezka's season 2 is the library's season 1, episodes 11–13 (a second cour), plus one the library doesn't have. */
const seasons: HDRezkaSeason[] = [
  {
    number: 1,
    episodes: [
      { number: 1, library: lib(1, 1, { has_file: true, source: 'torrent' }) },
      { number: 2, library: lib(1, 2, { has_file: true, source: 'hdrezka', stream: { translator: 'LostFilm', quality: '1080p' } }) },
      { number: 3, library: lib(1, 3) },
      { number: 4, library: lib(1, 4, { air_date: '2026-11-01' }) },
    ],
  },
  {
    number: 2,
    episodes: [
      { number: 1, library: lib(1, 11) },
      { number: 2, library: lib(1, 12, { air_date: null }) },
      { number: 3, library: null },
    ],
  },
];

describe('episode rows', () => {
  it('reads a state and the current source from the library episode', () => {
    expect(seasons[0].episodes.map((e) => e.library && [episodeRowState(e.library, NOW), episodeSourceLabel(e.library)])).toEqual([
      ['ready', 'Torrent'],
      ['ready', 'HDRezka · LostFilm'],
      ['missing', null],
      ['upcoming', null],
    ]);
    expect(episodeRowState(lib(1, 1, { air_date: null }), NOW)).toBe('upcoming');
  });

  it('selects missing ones, or all that map to the library', () => {
    expect(missingKeys(seasons[0], NOW)).toEqual(['1:3']);
    expect(allKeys(seasons[1])).toEqual(['2:1', '2:2']);
  });
});

describe('what opens first', () => {
  it('opens the HDRezka season holding the library season, else the same number, else the first', () => {
    expect(defaultSeasonNumber(seasons, { season: 1, episode: 12 })).toBe(1);
    expect(defaultSeasonNumber(seasons, { season: 2, episode: null })).toBe(2);
    expect(defaultSeasonNumber(seasons, { season: 9, episode: null })).toBe(1);
    expect(defaultSeasonNumber(seasons, { season: null, episode: null }, 1)).toBe(1);
    expect(defaultSeasonNumber([], { season: 1, episode: null })).toBeNull();
  });

  it('selects the scope’s episode wherever HDRezka has it, else the open season’s missing ones', () => {
    expect(defaultSelection(seasons, 1, { season: 1, episode: 11 }, NOW)).toEqual(['2:1']);
    expect(defaultSelection(seasons, 1, { season: 1, episode: null }, NOW)).toEqual(['1:3']);
    expect(defaultSelection(seasons, 2, { season: null, episode: null }, NOW)).toEqual(['2:1']);
  });

  it('turns keys into requests in HDRezka’s numbers, skipping what maps to nothing', () => {
    expect(episodeTargets(seasons, ['2:3', '2:1', '1:3']).map(({ episode_id, season, episode }) => ({ episode_id, season, episode }))).toEqual([
      { episode_id: 1003, season: 1, episode: 3 },
      { episode_id: 1011, season: 2, episode: 1 },
    ]);
  });

  it('prices qualities by the first picked episode, else the open season’s first', () => {
    expect(sampleEpisode(seasons, 1, ['2:2'])).toEqual({ season: 2, episode: 2 });
    expect(sampleEpisode(seasons, 2, [])).toEqual({ season: 2, episode: 1 });
    expect(sampleEpisode([], null, [])).toBeNull();
  });
});

describe('assignInChunks', () => {
  const targets: StreamEpisodeTarget[] = Array.from({ length: 64 }, (_, i) => ({ episode_id: i + 1, season: 1, episode: i + 1 }));

  it('sends 30 a request, one after another, reporting progress', async () => {
    expect(chunk(targets).map((c) => c.length)).toEqual([30, 30, 4]);
    const sizes: number[] = [];
    const progress: Array<[number, number]> = [];
    const result = await assignInChunks(
      targets,
      async (items) => {
        sizes.push(items.length);
        return { assigned: items, failed: [] };
      },
      (done, total) => progress.push([done, total])
    );
    expect(sizes).toEqual([30, 30, 4]);
    expect(progress).toEqual([
      [0, 64],
      [30, 64],
      [60, 64],
      [64, 64],
    ]);
    expect(result).toEqual({ assigned: 64, failed: [] });
  });

  it('collects per-episode failures, and fails a whole chunk with its error while the next one still goes', async () => {
    let call = 0;
    const result = await assignInChunks(targets.slice(0, 40), async (items) => {
      call += 1;
      if (call === 1) return { assigned: items.slice(1), failed: [{ episode_id: items[0].episode_id, code: 'timeout', message: 'Too slow.' }] };
      throw new ApiError(503, 'hdrezka_blocked', 'Bot check.');
    });
    expect(result.assigned).toBe(29);
    expect(result.failed).toHaveLength(11);
    expect(result.failed[0]).toEqual({ episode_id: 1, code: 'timeout', message: 'Too slow.' });
    expect(result.failed[1]).toEqual({ episode_id: 31, code: 'hdrezka_blocked', message: 'Bot check.' });
  });
});
