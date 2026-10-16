import { type PersonCandidate, personMatches, personReason, rankTitleSearch, type TitleCandidate, titleMatch } from '@/lib/title-search/rank';

let nextId = 1;
function candidate(title: string, overrides: Partial<TitleCandidate> = {}): TitleCandidate {
  return {
    media_type: 'movie',
    tmdb_id: nextId++,
    title,
    original_title: null,
    year: 2020,
    poster_path: null,
    vote_average: 7,
    vote_count: 500,
    popularity: 10,
    ...overrides,
  };
}

const titles = (result: ReturnType<typeof rankTitleSearch>) => result.items.map((item) => `${item.title}${item.year ? ` (${item.year})` : ''}`);

describe('titleMatch', () => {
  it('ranks equal, prefix, every word, then the original title', () => {
    expect(titleMatch('dune', 'Dune', null)).toBe('exact');
    expect(titleMatch('dune', 'Dune: Part Two', null)).toBe('prefix');
    expect(titleMatch('dune', 'Jodorowsky’s Dune', null)).toBe('words');
    expect(titleMatch('frieren journey', 'Frieren: Beyond Journey’s End', null)).toBe('words');
    expect(titleMatch('himmel berlin', 'Wings of Desire', 'Der Himmel über Berlin')).toBe('original');
  });

  it('ignores case, accents and punctuation', () => {
    expect(titleMatch('SHOGUN', 'Shōgun', null)).toBe('exact');
    expect(titleMatch('journeys end', 'Frieren: Beyond Journey’s End', null)).toBe('words');
  });

  it('says nothing when only an alternative title can explain it', () => {
    expect(titleMatch('sousou no frieren', 'Frieren: Beyond Journey’s End', '葬送のフリーレン')).toBeNull();
    expect(titleMatch('', 'Dune', null)).toBeNull();
  });
});

describe('people', () => {
  it('match when every word starts one of the name’s words', () => {
    expect(personMatches('vince gilligan', 'Vince Gilligan')).toBe(true);
    expect(personMatches('gil', 'Vince Gilligan')).toBe(true);
    expect(personMatches('vince gil', 'Vince Gilligan')).toBe(true);
    expect(personMatches('illig', 'Vince Gilligan')).toBe(false);
    expect(personMatches('', 'Vince Gilligan')).toBe(false);
  });

  it('give a reason from their department', () => {
    expect(personReason('Zendaya', 'Acting', 'movie')).toBe('With Zendaya');
    expect(personReason('Zendaya', null, 'tv')).toBe('With Zendaya');
    expect(personReason('Denis Villeneuve', 'Directing', 'movie')).toBe('Directed by Denis Villeneuve');
    expect(personReason('Vince Gilligan', 'Writing', 'tv')).toBe('Created by Vince Gilligan');
    expect(personReason('Vince Gilligan', 'Writing', 'movie')).toBe('Written by Vince Gilligan');
    expect(personReason('Kathleen Kennedy', 'Production', 'movie')).toBe('From Kathleen Kennedy');
  });
});

describe('rankTitleSearch', () => {
  it('puts exact titles first, then prefixes and other matches, with the rating breaking ties', () => {
    const result = rankTitleSearch('dune', {
      movies: [
        candidate('Jodorowsky’s Dune', { year: 2013, vote_average: 7.9 }),
        candidate('Dune: Part Two', { year: 2024, vote_average: 8.2 }),
        candidate('Dune', { year: 1984, vote_average: 6.3 }),
        candidate('Dune', { year: 2021, vote_average: 7.8 }),
      ],
      shows: [candidate('Dune: Prophecy', { media_type: 'tv', year: 2024, vote_average: 7.6 })],
      people: [],
    });
    expect(titles(result)).toEqual(['Dune (2021)', 'Dune (1984)', 'Dune: Part Two (2024)', 'Dune: Prophecy (2024)', 'Jodorowsky’s Dune (2013)']);
    expect(result.counts).toEqual({ all: 5, movie: 4, tv: 1 });
    expect(result.items.every((item) => item.match_reason === null)).toBe(true);
    expect(result.query).toBe('dune');
  });

  it('ignores the rating of a title few people rated', () => {
    const rare = candidate('Dune', { tmdb_id: 900, vote_average: 10, vote_count: 3, popularity: 1 });
    const known = candidate('Dune', { tmdb_id: 901, vote_average: 6, popularity: 50 });
    const result = rankTitleSearch('dune', { movies: [rare, known], shows: [], people: [] });
    expect(result.items.map((item) => item.tmdb_id)).toEqual([901, 900]);
  });

  it('calls a title TMDB matched by another name an original title match', () => {
    const frieren = candidate('Frieren: Beyond Journey’s End', { media_type: 'tv', original_title: '葬送のフリーレン' });
    const [item] = rankTitleSearch('sousou no frieren', { movies: [], shows: [frieren], people: [] }).items;
    expect(item).toMatchObject({ match: 'original', match_reason: 'Original title match', original_title: '葬送のフリーレン' });
  });

  it('adds what a named person is known for, below the title matches', () => {
    const breakingBad = candidate('Breaking Bad', { media_type: 'tv', vote_average: 8.9 });
    const elCamino = candidate('El Camino: A Breaking Bad Movie', { vote_average: 6.9 });
    const vince: PersonCandidate = { name: 'Vince Gilligan', department: 'Writing', known_for: [breakingBad, elCamino] };
    const result = rankTitleSearch('vince gilligan', { movies: [candidate('Vince Gilligan’s Sketchbook')], shows: [], people: [vince] });
    expect(result.items.map((item) => [item.title, item.match, item.match_reason])).toEqual([
      ['Vince Gilligan’s Sketchbook', 'prefix', null],
      ['Breaking Bad', 'person', 'Created by Vince Gilligan'],
      ['El Camino: A Breaking Bad Movie', 'person', 'Written by Vince Gilligan'],
    ]);
  });

  it('lists each title once, with its best match', () => {
    const dune = candidate('Dune', { year: 2021 });
    const denis: PersonCandidate = { name: 'Dune Villeneuve', department: 'Directing', known_for: [dune] };
    const result = rankTitleSearch('dune', { movies: [dune], shows: [], people: [denis] });
    expect(result.items).toHaveLength(1);
    expect(result.items[0]).toMatchObject({ match: 'exact', match_reason: null });
  });

  it('skips people the query doesn’t name, and keeps the best three', () => {
    const person = (name: string): PersonCandidate => ({ name, department: 'Acting', known_for: [candidate(`${name}’s film`)] });
    const result = rankTitleSearch('anna', {
      movies: [],
      shows: [],
      people: [person('Bob Hannah'), person('Anna One'), person('Anna Two'), person('Anna Three'), person('Anna Four')],
    });
    expect(result.items.map((item) => item.match_reason)).toEqual(['With Anna One', 'With Anna Two', 'With Anna Three']);
  });
});
