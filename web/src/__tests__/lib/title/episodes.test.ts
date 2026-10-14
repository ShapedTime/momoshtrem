import type { ShowTitle } from '@/lib/api/contract';
import { episodeLabel, episodesSummary, nextUpEpisodeId, parseEpisodeCode, seasonLabel, selectedSeason } from '@/lib/title/episodes';
import { buildSeason, buildTitle, episodeId } from '@/mocks/fixtures/titles';

function show(tmdbId: number): ShowTitle {
  const title = buildTitle('tv', tmdbId);
  if (title?.media_type !== 'tv') throw new Error(`No fixture for show ${tmdbId}`);
  return title;
}
const episodes = (tmdbId: number, season: number) => buildSeason(tmdbId, season)?.episodes ?? [];

describe('nextUpEpisodeId', () => {
  it('marks the first episode of the frontier in its season', () => {
    expect(nextUpEpisodeId(show(209867), 1, episodes(209867, 1))).toBe(episodeId(209867, 1, 29));
  });

  it('marks nothing in other seasons, or when nothing is missing', () => {
    expect(nextUpEpisodeId(show(209867), 0, episodes(209867, 0))).toBeNull();
    expect(nextUpEpisodeId(show(95396), 2, episodes(95396, 2))).toBeNull();
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
  it('shows absolute numbers with the code for absolute ordering only', () => {
    const episode = { season_number: 22, episode_number: 15, absolute_number: 1100 };
    expect(episodeLabel(episode, 'absolute')).toBe('#1100 · S22E15');
    expect(episodeLabel(episode, 'aired')).toBe('S22E15');
    expect(episodeLabel({ ...episode, absolute_number: null }, 'absolute')).toBe('S22E15');
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

  it('sums up what is ready', () => {
    expect(episodesSummary(show(95396))).toMatch(/^\d+ of \d+ ready$/);
    expect(episodesSummary(show(37854))).toMatch(/^Absolute numbering · [\d,]+ episodes · [\d,]+ ready$/);
    expect(episodesSummary(show(66573))).toBeNull();
  });
});
