import type { ShowTitle } from '@/lib/api/contract';
import { episodeLabel, episodesSummary, nextUpEpisode, nextUpEpisodeId, parseEpisodeCode, seasonLabel, selectedSeason } from '@/lib/title/episodes';
import { findTitle } from '@/mocks/fixtures/catalog';
import { addToLibrary, buildSeason, buildTitle, episodeId, seedLibrary } from '@/mocks/fixtures/titles';
import { setMockScenario } from '@/mocks/scenario';

function show(tmdbId: number): ShowTitle {
  const title = buildTitle('tv', tmdbId);
  if (title?.media_type !== 'tv') throw new Error(`No fixture for show ${tmdbId}`);
  return title;
}
const episodes = (tmdbId: number, season: number) => buildSeason(tmdbId, season)?.episodes ?? [];
/** Without the Trakt account's history and progress: no watch state, nothing watched. */
const withoutTrakt = (tmdbId: number) => ({ ...show(tmdbId), watch_state: null });

afterEach(() => setMockScenario(null));

describe('nextUpEpisode', () => {
  it('is the episode Trakt names when the title lists its season (Frieren S01E27, ready)', () => {
    expect(nextUpEpisode(show(209867))).toEqual({ season: 1, episode: 27 });
    expect(nextUpEpisode(show(37854))).toEqual({ season: 21, episode: 1 }); // One Piece's rewatch, under review
    expect(nextUpEpisode(show(95396))).toEqual({ season: 2, episode: 10 }); // Severance: caught up, the finale airs next week
  });

  it('falls back to the frontier without Trakt, or when the title doesn’t list that season', () => {
    expect(nextUpEpisode(withoutTrakt(209867))).toEqual({ season: 1, episode: 29 });
    const frieren = show(209867);
    const elsewhere = { ...frieren, watch_state: frieren.watch_state && { ...frieren.watch_state, next_episode: { season: 2, episode: 1, absolute_number: null, name: null, air_date: null } } };
    expect(nextUpEpisode(elsewhere)).toEqual({ season: 1, episode: 29 });
    expect(nextUpEpisode({ ...frieren, watch_state: frieren.watch_state && { ...frieren.watch_state, next_episode: null } })).toEqual({ season: 1, episode: 29 });
  });

  it('is null with neither: no Trakt and nothing missing', () => {
    expect(nextUpEpisode(withoutTrakt(95396))).toBeNull();
  });
});

describe('nextUpEpisodeId', () => {
  it('marks Trakt’s next episode in its season, else the first of the frontier', () => {
    expect(nextUpEpisodeId(show(209867), 1, episodes(209867, 1))).toBe(episodeId(209867, 1, 27));
    expect(nextUpEpisodeId(show(57243), 3, episodes(57243, 3))).toBe(episodeId(57243, 3, 4)); // Doctor Who: missing, after three watched
    expect(nextUpEpisodeId(withoutTrakt(209867), 1, episodes(209867, 1))).toBe(episodeId(209867, 1, 29));
  });

  it('marks nothing in other seasons, or with neither', () => {
    expect(nextUpEpisodeId(show(209867), 0, episodes(209867, 0))).toBeNull();
    expect(nextUpEpisodeId(show(37854), 22, episodes(37854, 22))).toBeNull(); // One Piece's is in Wano
    expect(nextUpEpisodeId(withoutTrakt(95396), 2, episodes(95396, 2))).toBeNull();
  });
});

describe('the mock’s watch state follows the Trakt account (PLAN §6 "Watch state rules")', () => {
  it('connected: plays are watched, a watched episode keeps its file, and Trakt-only plays have none', () => {
    const frieren = episodes(209867, 1);
    expect(frieren.filter((e) => e.state === 'watched').map((e) => e.episode_number)).toEqual(Array.from({ length: 26 }, (_, i) => i + 1));
    expect(frieren[0]).toMatchObject({ state: 'watched', source: 'torrent', webdav_path: expect.stringContaining('S01E01') });
    expect(show(209867).default_season).toBe(1);
    expect(show(57243).default_season).toBe(3); // Doctor Who opens on Trakt's S03E04
    expect(show(37854).default_season).toBe(21);
  });

  it('Trakt-only plays: The Good Place, just added, has S01E01–E05 watched without files, and Up next after them', () => {
    const library = seedLibrary();
    const entry = findTitle('tv', 66573);
    if (entry) addToLibrary(library, entry);
    const s1 = buildSeason(66573, 1, library)?.episodes ?? [];
    expect(s1.slice(0, 6).map((e) => [e.state, e.webdav_path])).toEqual([...Array(5).fill(['watched', null]), ['missing', null]]);
    const goodPlace = buildTitle('tv', 66573, library);
    expect(goodPlace?.library).toMatchObject({ availability_stage: 'needs_torrent', episodes_ready: 0, missing: { season_number: 1, from: 6, to: 13 } });
    expect(goodPlace?.media_type === 'tv' && nextUpEpisodeId(goodPlace, 1, s1)).toBe(episodeId(66573, 1, 6));
  });

  it('off: watch_state is null, nothing is watched, and the frontier and latest file pick the season', () => {
    setMockScenario('trakt-off');
    expect(show(209867).watch_state).toBeNull();
    expect(episodes(209867, 1).some((e) => e.state === 'watched')).toBe(false);
    expect(episodes(209867, 1)[0]).toMatchObject({ state: 'ready' });
    expect(show(57243).default_season).toBe(4);
    expect(show(37854).default_season).toBe(22);
  });
});

describe('selectedSeason', () => {
  const frieren = show(209867);
  it.each([
    ['a season the show has', '0', 0],
    ['nothing: the default season', null, 1],
    ['a season it lacks: the default', '7', 1],
    ['junk: the default', 'two', 1],
  ])('%s', (_name, param, expected) => {
    expect(selectedSeason(param, frieren)).toBe(expected);
  });
});

describe('labels', () => {
  it('shows absolute numbers with the code for absolute ordering, unless they match the in-season number', () => {
    const episode = { season_number: 22, episode_number: 15, absolute_number: 1100 };
    expect(episodeLabel(episode, 'absolute')).toBe('#1100 · S22E15');
    expect(episodeLabel(episode, 'aired')).toBe('S22E15');
    expect(episodeLabel({ ...episode, absolute_number: null }, 'absolute')).toBe('S22E15');
    expect(episodeLabel({ season_number: 1, episode_number: 29, absolute_number: 29 }, 'absolute')).toBe('S01E29');
  });

  it('names seasons: Specials, TMDB’s name, or Season N', () => {
    expect(seasonLabel({ season_number: 0, name: 'Specials' })).toBe('Specials');
    expect(seasonLabel({ season_number: 21, name: 'Wano Country' })).toBe('Wano Country');
    expect(seasonLabel({ season_number: 3, name: '' })).toBe('Season 3');
  });

  it('reads S01E05 codes', () => {
    expect(parseEpisodeCode('S01E05')).toEqual({ season: 1, episode: 5 });
    expect(parseEpisodeCode('s22e1100')).toEqual({ season: 22, episode: 1100 });
    expect(parseEpisodeCode('1x05')).toBeNull();
    expect(parseEpisodeCode(null)).toBeNull();
  });

  it('sums up what is ready, naming absolute numbering only when it differs from in-season numbers', () => {
    expect(episodesSummary(show(95396))).toMatch(/^\d+ of \d+ ready$/);
    expect(episodesSummary(show(209867))).toMatch(/^\d+ of 38 ready$/);
    expect(episodesSummary(show(37854))).toMatch(/^Absolute numbering · [\d,]+ episodes · [\d,]+ ready$/);
    expect(episodesSummary(show(66573))).toBeNull();
  });
});
