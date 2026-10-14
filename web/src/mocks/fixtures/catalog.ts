import type { MediaType } from '@/lib/api/contract';

/** The titles every fixture draws from (artwork paths are null: mocks never hit TMDB). */
export interface CatalogEntry {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  original_title: string | null;
  year: number;
  genres: string[];
  overview: string;
  runtime?: number;
  is_anime?: boolean;
}

export const CATALOG: CatalogEntry[] = [
  {
    media_type: 'tv',
    tmdb_id: 209867,
    title: 'Frieren: Beyond Journey’s End',
    original_title: '葬送のフリーレン',
    year: 2023,
    genres: ['Animation', 'Fantasy'],
    overview: 'After the party defeats the Demon King, the elf mage Frieren sets out to understand the humans she outlived.',
    is_anime: true,
  },
  {
    media_type: 'tv',
    tmdb_id: 95396,
    title: 'Severance',
    original_title: null,
    year: 2022,
    genres: ['Drama', 'Mystery', 'Sci-Fi'],
    overview: 'Mark leads a team of office workers whose memories have been surgically divided between their work and personal lives.',
  },
  {
    media_type: 'tv',
    tmdb_id: 82728,
    title: 'Bluey',
    original_title: null,
    year: 2018,
    genres: ['Animation', 'Family'],
    overview: 'The adventures of Bluey, an inexhaustible six-year-old Blue Heeler, and her family.',
  },
  {
    media_type: 'tv',
    tmdb_id: 247767,
    title: 'The Studio',
    original_title: null,
    year: 2025,
    genres: ['Comedy'],
    overview: 'The new head of a struggling Hollywood studio tries to save it without losing his soul.',
  },
  {
    media_type: 'movie',
    tmdb_id: 1054867,
    title: 'One Battle After Another',
    original_title: null,
    year: 2025,
    genres: ['Action', 'Thriller'],
    overview: 'A washed-up revolutionary reunites with old comrades when his daughter goes missing.',
    runtime: 162,
  },
  {
    media_type: 'movie',
    tmdb_id: 666277,
    title: 'Past Lives',
    original_title: null,
    year: 2023,
    genres: ['Drama', 'Romance'],
    overview: 'Two childhood friends are reunited in New York for one fateful week.',
    runtime: 106,
  },
  {
    media_type: 'movie',
    tmdb_id: 693134,
    title: 'Dune: Part Two',
    original_title: null,
    year: 2024,
    genres: ['Sci-Fi', 'Adventure'],
    overview: 'Paul Atreides unites with the Fremen while seeking revenge against those who destroyed his family.',
    runtime: 167,
  },
];

export function findTitle(mediaType: MediaType, tmdbId: number): CatalogEntry | undefined {
  return CATALOG.find((entry) => entry.media_type === mediaType && entry.tmdb_id === tmdbId);
}

export const iso = (daysAgo: number) => new Date(Date.UTC(2026, 9, 14) - daysAgo * 86_400_000).toISOString();
