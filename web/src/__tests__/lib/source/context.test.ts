import type { MovieTitle, ShowTitle } from '@/lib/api/contract';
import { legacyScope, sourceContext, sourceHeading, sourceParams } from '@/lib/source/context';
import { buildTitle } from '@/mocks/fixtures/titles';

const params = (query: string) => new URLSearchParams(query);

describe('sourceContext', () => {
  it('reads a movie, a season and an episode, each with its own key', () => {
    expect(sourceContext(params('sheet=hdrezka&movie=157336'))).toEqual({ kind: 'movie', tmdbId: 157336, key: 'movie:157336' });
    expect(sourceContext(params('show=95396&season=2'))).toEqual({ kind: 'show', tmdbId: 95396, season: 2, episode: null, key: 'show:95396:2:' });
    expect(sourceContext(params('show=209867&season=1&se=S01E29'))).toEqual({ kind: 'show', tmdbId: 209867, season: 1, episode: 29, key: 'show:209867:1:29' });
  });

  it.each(['', 'movie=abc', 'movie=-1', 'show=0'])('opens nothing for “%s”', (query) => {
    expect(sourceContext(params(query))).toBeNull();
  });

  it('round-trips through the sheet’s params', () => {
    const context = sourceContext(params('show=209867&season=1&se=S01E29'));
    expect(context && sourceParams(context)).toEqual({ show: '209867', season: '1', se: 'S01E29' });
    expect(context && legacyScope(context)).toEqual({ season: 1, episode: 29 });
  });
});

describe('sourceHeading', () => {
  it('heads a movie with its year and runtime', () => {
    const movie = buildTitle('movie', 157336) as MovieTitle;
    expect(sourceHeading({ kind: 'movie', tmdbId: 157336, key: '' }, movie)).toEqual({ label: 'Interstellar', heading: '2014 · 2h 49m' });
  });

  it('heads a show with its season, or the episode and its name', () => {
    const show = buildTitle('tv', 209867) as ShowTitle;
    const context = { kind: 'show' as const, tmdbId: 209867, season: 1, episode: 29, key: '' };
    expect(sourceHeading(context, show, { name: 'Let’s Go Somewhere Warm' }).heading).toBe('S01E29 · Let’s Go Somewhere Warm');
    expect(sourceHeading(context, show).heading).toBe('S01E29');
    expect(sourceHeading({ ...context, episode: null }, show)).toEqual({ label: 'Frieren: Beyond Journey’s End', heading: 'Season 1' });
    expect(sourceHeading({ ...context, season: 0, episode: null }, show).heading).toBe('Specials');
  });
});
