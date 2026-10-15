import type { Proposal, ShowEpisodesSeason } from '@/lib/api/contract';
import {
  episodeNumberLabel,
  initialSeason,
  matchesQuery,
  type PickerEpisode,
  pickerRows,
  seasonHeader,
  seasonShortLabel,
  tagFor,
  takenBy,
} from '@/lib/mapping/picker';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

const ep = (season: number, episode: number, extra: Partial<PickerEpisode> = {}): PickerEpisode => ({
  id: season * 1000 + episode,
  season,
  episode,
  absolute: null,
  name: `Episode ${episode}`,
  hasFile: false,
  ...extra,
});

const episodes = (season: number, count: number, extra: (n: number) => Partial<ShowEpisodesSeason['episodes'][number]> = () => ({})) =>
  Array.from({ length: count }, (_, i) => ({
    id: season * 1000 + i + 1,
    episode_number: i + 1,
    absolute_number: null,
    name: `Episode ${i + 1}`,
    air_date: null,
    has_file: false,
    ...extra(i + 1),
  }));

const CATALOG: ShowEpisodesSeason[] = [
  { season_number: 0, name: 'Specials', episodes: episodes(0, 3, (n) => ({ name: ['Recap: The Journey So Far', 'Mini Anime', 'Fan Letter'][n - 1] })) },
  { season_number: 1, name: 'Season 1', episodes: episodes(1, 4, (n) => ({ has_file: n === 1 })) },
];

describe('search', () => {
  it('matches episode and absolute numbers, absolute ones by prefix', () => {
    const wano = ep(21, 79, { absolute: 1071 });
    expect(matchesQuery(wano, '79')).toBe(true);
    expect(matchesQuery(wano, '#1071')).toBe(true);
    expect(matchesQuery(wano, '107')).toBe(true);
    expect(matchesQuery(wano, '108')).toBe(false);
    expect(matchesQuery(ep(1, 35), '35')).toBe(true);
    expect(matchesQuery(ep(1, 35), 'e35')).toBe(true);
  });

  it('matches SxxEyy, a whole season, and Specials', () => {
    expect(matchesQuery(ep(0, 3), 's00e3')).toBe(true);
    expect(matchesQuery(ep(0, 3), 'S00E03')).toBe(true);
    expect(matchesQuery(ep(0, 30), 's00e3')).toBe(false);
    expect(matchesQuery(ep(2, 5), 's2')).toBe(true);
    expect(matchesQuery(ep(0, 9), 'special')).toBe(true);
    expect(matchesQuery(ep(0, 9), 'sp9')).toBe(true);
    expect(matchesQuery(ep(0, 8), 'sp9')).toBe(false);
    expect(matchesQuery(ep(1, 9), 'sp')).toBe(false);
  });

  it('falls back to a part of the title, any case', () => {
    const recap = ep(0, 3, { name: 'Recap: The Journey So Far' });
    expect(matchesQuery(recap, 'journey')).toBe(true);
    expect(matchesQuery(recap, '  ')).toBe(true);
    expect(matchesQuery(recap, 'mimic')).toBe(false);
  });
});

describe('labels', () => {
  it('shortens seasons for the jump and names them in headers', () => {
    expect(seasonShortLabel({ season_number: 0, name: 'Specials' })).toBe('Specials');
    expect(seasonShortLabel({ season_number: 1, name: 'Season 1' })).toBe('Season 1');
    expect(seasonShortLabel({ season_number: 21, name: 'Wano Country' })).toBe('S21 Wano');
    expect(seasonShortLabel({ season_number: 2, name: 'Series 2' })).toBe('Series 2');
    expect(seasonHeader({ season_number: 0, name: 'Specials' })).toBe('Specials · Season 0');
    expect(seasonHeader({ season_number: 21, name: 'Wano Country' })).toBe('Season 21 · Wano Country');
    expect(seasonHeader({ season_number: 1, name: 'Season 1' })).toBe('Season 1');
  });

  it('numbers specials SP and the rest E', () => {
    expect(episodeNumberLabel(ep(0, 3))).toBe('SP03');
    expect(episodeNumberLabel(ep(1, 35))).toBe('E35');
  });
});

describe('rows and tags', () => {
  const filter = { query: '', hideTaken: false, current: new Set([1002]), taken: new Map([[1003, '#31']]) };

  it('tags Current, then Taken, then In library', () => {
    expect(tagFor(ep(1, 2), filter)).toEqual({ kind: 'current' });
    expect(tagFor(ep(1, 3), filter)).toEqual({ kind: 'taken', by: '#31' });
    expect(tagFor(ep(1, 1, { hasFile: true }), filter)).toEqual({ kind: 'library' });
    expect(tagFor(ep(1, 4), filter)).toBeNull();
  });

  it('puts a header before each season, Specials first as the API lists them', () => {
    const rows = pickerRows(CATALOG, filter);
    expect(rows.map((row) => (row.type === 'header' ? row.label : row.episode.id))).toEqual([
      'Specials · Season 0',
      1,
      2,
      3,
      'Season 1',
      1001,
      1002,
      1003,
      1004,
    ]);
  });

  it('Hide taken drops taken and filed episodes but keeps Current; empty seasons drop out', () => {
    const rows = pickerRows(CATALOG, { ...filter, hideTaken: true, query: 's1' });
    expect(rows.map((row) => (row.type === 'header' ? row.label : row.episode.id))).toEqual(['Season 1', 1002, 1004]);
    expect(pickerRows(CATALOG, { ...filter, query: 'zzz' })).toEqual([]);
  });
});

describe('taken and the starting season', () => {
  const job = seedJobs().get(101);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error('No seeded Frieren job');
  const { proposals } = jobDetail(job, { entry, ai_available: true, currentFile: () => null });
  const named = (part: string): Proposal => proposals.find((p) => p.file_path.includes(part)) as Proposal;

  it('lists episodes the job’s other non-ignored files use', () => {
    const selected = named('35v2');
    const taken = takenBy(proposals, selected.id, 'Frieren: Beyond Journey’s End');
    expect(taken.get(named(' - 31 ').targets[0].episode_id)).toBe('#31');
    expect(taken.has(selected.targets[0].episode_id)).toBe(false);
    // NCOP2 is ignored and has no episode anyway: 9 matched + 28.5 + Mini Anime 05
    expect(taken.size).toBe(11);
  });

  it('opens on the file’s season, else the first regular one', () => {
    expect(initialSeason(CATALOG, { targets: [named('28.5').targets[0]] })).toBe(0);
    expect(initialSeason(CATALOG, { targets: [] })).toBe(1);
    expect(initialSeason(CATALOG, null)).toBe(1);
  });
});
