import type { MediaType, TitleMatch, TitleSearchItem, TitleSearchResult } from '@/lib/api/contract';
import { normalize, queryWords } from './text';

/**
 * Ranking for GET /api/title-search (js/screens/search.js `search`): TMDB's
 * movie, TV and person answers merged into one list. Pure, so the route,
 * the mocks and Jest share it.
 */

/** A title as TMDB's search (or a person's `known_for`) lists it. */
export interface TitleCandidate {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  original_title: string | null;
  year: number | null;
  poster_path: string | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
}

/** A person from TMDB's person search. */
export interface PersonCandidate {
  name: string;
  /** TMDB's `known_for_department`: "Acting", "Directing", "Writing", … */
  department: string | null;
  known_for: TitleCandidate[];
}

export interface TitleSearchAnswers {
  movies: TitleCandidate[];
  shows: TitleCandidate[];
  people: PersonCandidate[];
}

/** Each kind of match's weight; the rating adds at most 1 and only breaks ties within a kind. */
export const MATCH_SCORE: Record<TitleMatch, number> = { exact: 100, prefix: 80, words: 60, original: 55, person: 40 };

/** The people whose titles join the results: the best few that the query names. */
const MAX_PEOPLE = 3;

/**
 * A rating on fewer votes than this says little: "dune" finds a dozen
 * films called Dune, and a short rated 7.4 by 20 people mustn't beat
 * Lynch's. Below it, TMDB's popularity orders them.
 */
const MIN_VOTES = 100;

export const ORIGINAL_TITLE_MATCH = 'Original title match';

/**
 * How a title's own names answer the query: equal, starts with it, holds
 * every word, or its original title holds every word. Null when none does:
 * TMDB matched an alternative title or a translation.
 */
export function titleMatch(query: string, title: string, originalTitle: string | null): Exclude<TitleMatch, 'person'> | null {
  const words = queryWords(query);
  if (!words.length) return null;
  const wanted = words.join(' ');
  const name = normalize(title);
  if (name === wanted) return 'exact';
  if (name.startsWith(wanted)) return 'prefix';
  if (words.every((word) => name.includes(word))) return 'words';
  if (originalTitle) {
    const original = normalize(originalTitle);
    if (words.every((word) => original.includes(word))) return 'original';
  }
  return null;
}

/** The query names this person: each word starts one of the name's words ("vince gil" → Vince Gilligan). */
export function personMatches(query: string, name: string): boolean {
  const words = queryWords(query);
  const parts = normalize(name).split(' ');
  return words.length > 0 && words.every((word) => parts.some((part) => part.startsWith(word)));
}

/**
 * Why a person's title matched. TMDB's `known_for` has no job per title, so
 * the person's department stands in: actors are "With", and the people
 * behind a show "Created by"; a film is "Directed by" or "Written by" its
 * director or writer, and "From" anyone else behind it.
 */
export function personReason(name: string, department: string | null, mediaType: MediaType): string {
  if (!department || department === 'Acting') return `With ${name}`;
  if (mediaType === 'tv') return `Created by ${name}`;
  if (department === 'Directing') return `Directed by ${name}`;
  if (department === 'Writing') return `Written by ${name}`;
  return `From ${name}`;
}

const keyOf = (candidate: Pick<TitleCandidate, 'media_type' | 'tmdb_id'>) => `${candidate.media_type}:${candidate.tmdb_id}`;

const ratingBonus = (candidate: TitleCandidate) => (candidate.vote_count >= MIN_VOTES ? candidate.vote_average / 10 : 0);

interface Ranked {
  item: TitleSearchItem;
  score: number;
  popularity: number;
}

/**
 * One ranked list from TMDB's three answers: each title once, with its best
 * match. Ties go to the more popular title.
 */
export function rankTitleSearch(query: string, answers: TitleSearchAnswers): TitleSearchResult {
  const best = new Map<string, Ranked>();
  const offer = (candidate: TitleCandidate, match: TitleMatch, reason: string | null) => {
    const score = MATCH_SCORE[match] + ratingBonus(candidate);
    const current = best.get(keyOf(candidate));
    if (current && current.score >= score) return;
    best.set(keyOf(candidate), {
      score,
      popularity: candidate.popularity,
      item: {
        media_type: candidate.media_type,
        tmdb_id: candidate.tmdb_id,
        title: candidate.title,
        original_title: candidate.original_title,
        year: candidate.year,
        poster_path: candidate.poster_path,
        match,
        match_reason: reason,
      },
    });
  };

  for (const candidate of [...answers.movies, ...answers.shows]) {
    const match = titleMatch(query, candidate.title, candidate.original_title) ?? 'original';
    offer(candidate, match, match === 'original' ? ORIGINAL_TITLE_MATCH : null);
  }
  const people = answers.people.filter((person) => personMatches(query, person.name)).slice(0, MAX_PEOPLE);
  for (const person of people) {
    for (const candidate of person.known_for) offer(candidate, 'person', personReason(person.name, person.department, candidate.media_type));
  }

  const items = [...best.values()].sort((a, b) => b.score - a.score || b.popularity - a.popularity).map((ranked) => ranked.item);
  return {
    query,
    items,
    counts: {
      all: items.length,
      movie: items.filter((item) => item.media_type === 'movie').length,
      tv: items.filter((item) => item.media_type === 'tv').length,
    },
  };
}
