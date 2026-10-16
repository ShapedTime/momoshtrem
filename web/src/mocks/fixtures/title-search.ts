import type { MediaType, TitleSearchResult } from '@/lib/api/contract';
import { type PersonCandidate, rankTitleSearch, type TitleCandidate } from '@/lib/title-search/rank';
import { normalize, queryWords } from '@/lib/title-search/text';

/**
 * A small TMDB stand-in for title search (#101): movies, shows and people
 * with what they're known for, enough for "dune" (six titles, one in the
 * library), "sev", "sousou no frieren" (an alternative title) and "vince
 * gilligan" (a person). The library's titles keep their ids, so the search
 * page and the palette find them in the library. Posters are null: mocks
 * never hit TMDB.
 */

interface TitleFixture extends TitleCandidate {
  /** Alternative titles and translations TMDB also matches (the route never sees them). */
  aliases: string[];
}

type Facts = Partial<Pick<TitleFixture, 'original_title' | 'aliases'>> & { rating: number; popularity: number };

const title = (media_type: MediaType, tmdb_id: number, name: string, year: number, facts: Facts): TitleFixture => ({
  media_type,
  tmdb_id,
  title: name,
  original_title: facts.original_title ?? null,
  year,
  poster_path: null,
  vote_average: facts.rating,
  vote_count: 1000,
  popularity: facts.popularity,
  aliases: facts.aliases ?? [],
});
const movie = (tmdb_id: number, name: string, year: number, facts: Facts) => title('movie', tmdb_id, name, year, facts);
const show = (tmdb_id: number, name: string, year: number, facts: Facts) => title('tv', tmdb_id, name, year, facts);

export const TITLE_FIXTURES: TitleFixture[] = [
  movie(438631, 'Dune', 2021, { rating: 7.8, popularity: 120 }),
  movie(841, 'Dune', 1984, { rating: 6.3, popularity: 40 }),
  movie(693134, 'Dune: Part Two', 2024, { rating: 8.2, popularity: 200 }),
  movie(251516, 'Jodorowsky’s Dune', 2013, { rating: 7.9, popularity: 10 }),
  movie(157336, 'Interstellar', 2014, { rating: 8.5, popularity: 150 }),
  movie(666277, 'Past Lives', 2023, { rating: 7.8, popularity: 60 }),
  movie(1054867, 'One Battle After Another', 2025, { rating: 7.8, popularity: 180 }),
  movie(329865, 'Arrival', 2016, { rating: 7.6, popularity: 70 }),
  movie(559969, 'El Camino: A Breaking Bad Movie', 2019, { rating: 6.9, popularity: 30 }),
  movie(655, 'Paris, Texas', 1984, { rating: 8.0, popularity: 20 }),
  movie(144, 'Wings of Desire', 1987, { original_title: 'Der Himmel über Berlin', rating: 7.9, popularity: 15 }),
  movie(976893, 'Perfect Days', 2023, { rating: 7.8, popularity: 35 }),
  movie(346, 'Seven Samurai', 1954, { original_title: '七人の侍', rating: 8.5, popularity: 25 }),
  show(90228, 'Dune: Prophecy', 2024, { rating: 7.6, popularity: 110 }),
  show(10342, 'Frank Herbert’s Dune', 2000, { rating: 6.9, popularity: 8 }),
  show(209867, 'Frieren: Beyond Journey’s End', 2023, { original_title: '葬送のフリーレン', aliases: ['Sousou no Frieren'], rating: 8.8, popularity: 120 }),
  show(95396, 'Severance', 2022, { rating: 8.4, popularity: 300 }),
  show(37854, 'One Piece', 1999, { original_title: 'ワンピース', aliases: ['Wan Pīsu'], rating: 8.7, popularity: 200 }),
  show(82728, 'Bluey', 2018, { rating: 8.7, popularity: 90 }),
  show(247767, 'The Studio', 2025, { rating: 7.9, popularity: 350 }),
  show(57243, 'Doctor Who', 2005, { rating: 7.5, popularity: 70 }),
  show(66573, 'The Good Place', 2016, { rating: 8.1, popularity: 80 }),
  show(1396, 'Breaking Bad', 2008, { rating: 8.9, popularity: 400 }),
  show(60059, 'Better Call Saul', 2015, { rating: 8.7, popularity: 160 }),
  show(126308, 'Shōgun', 2024, { rating: 8.6, popularity: 190 }),
];

const byId = (mediaType: MediaType, tmdbId: number): TitleFixture => {
  const found = TITLE_FIXTURES.find((entry) => entry.media_type === mediaType && entry.tmdb_id === tmdbId);
  if (!found) throw new Error(`No title fixture ${mediaType}:${tmdbId}`);
  return found;
};

export const PEOPLE_FIXTURES: PersonCandidate[] = [
  { name: 'Vince Gilligan', department: 'Writing', known_for: [byId('tv', 1396), byId('tv', 60059), byId('movie', 559969)] },
  { name: 'Wim Wenders', department: 'Directing', known_for: [byId('movie', 655), byId('movie', 144), byId('movie', 976893)] },
  { name: 'Denis Villeneuve', department: 'Directing', known_for: [byId('movie', 438631), byId('movie', 693134), byId('movie', 329865)] },
  { name: 'Zendaya', department: 'Acting', known_for: [byId('movie', 693134), byId('movie', 438631)] },
];

/** TMDB-ish matching: each query word starts a word of one of the names. */
function finds(query: string, names: Array<string | null>): boolean {
  const words = queryWords(query);
  return names.some((name) => {
    if (!name) return false;
    const parts = normalize(name).split(' ');
    return words.every((word) => parts.some((part) => part.startsWith(word)));
  });
}

/** GET /api/title-search over the fixtures, ranked by the route's own code. */
export function mockTitleSearch(query: string): TitleSearchResult {
  const found = TITLE_FIXTURES.filter((entry) => finds(query, [entry.title, entry.original_title, ...entry.aliases]));
  return rankTitleSearch(query, {
    movies: found.filter((entry) => entry.media_type === 'movie'),
    shows: found.filter((entry) => entry.media_type === 'tv'),
    people: PEOPLE_FIXTURES.filter((person) => finds(query, [person.name])),
  });
}
