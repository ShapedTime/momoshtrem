import type { MediaType } from '@/lib/api/contract';
import type { ShowStatus } from '@/lib/discover/filters';
import type { GenreSlug } from '@/lib/discover/genres';

/**
 * A small TMDB stand-in for Discover (#100): enough titles for three pages
 * of 20, with the facts the filters read. The library's titles keep their
 * ids, so their cards get library pills. Posters are null: mocks never hit
 * TMDB.
 */
export interface DiscoverFixture {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number;
  genres: GenreSlug[];
  rating: number;
  /** Minutes; per episode for shows. */
  runtime: number;
  /** Original language (ISO 639-1) and country of origin. */
  lang: string;
  country: string;
  /** Shows only. */
  status: ShowStatus | null;
  popularity: number;
}

type Facts = Omit<DiscoverFixture, 'media_type' | 'tmdb_id' | 'title' | 'status'>;

const movie = (tmdb_id: number, title: string, facts: Facts): DiscoverFixture => ({ media_type: 'movie', tmdb_id, title, status: null, ...facts });
const show = (tmdb_id: number, title: string, status: ShowStatus, facts: Facts): DiscoverFixture => ({ media_type: 'tv', tmdb_id, title, status, ...facts });

export const DISCOVER_FIXTURES: DiscoverFixture[] = [
  // In the mock library (fixtures/catalog.ts)
  show(209867, 'Frieren: Beyond Journey’s End', 'returning', { year: 2023, genres: ['animation', 'adventure', 'fantasy'], rating: 8.8, runtime: 24, lang: 'ja', country: 'JP', popularity: 120 }),
  show(95396, 'Severance', 'returning', { year: 2022, genres: ['drama', 'mystery', 'sci-fi'], rating: 8.4, runtime: 50, lang: 'en', country: 'US', popularity: 300 }),
  show(82728, 'Bluey', 'returning', { year: 2018, genres: ['animation', 'comedy', 'family'], rating: 8.7, runtime: 7, lang: 'en', country: 'AU', popularity: 90 }),
  show(247767, 'The Studio', 'returning', { year: 2025, genres: ['comedy'], rating: 7.9, runtime: 30, lang: 'en', country: 'US', popularity: 350 }),
  movie(1054867, 'One Battle After Another', { year: 2025, genres: ['action', 'thriller'], rating: 7.8, runtime: 162, lang: 'en', country: 'US', popularity: 280 }),
  movie(666277, 'Past Lives', { year: 2023, genres: ['drama', 'romance'], rating: 7.8, runtime: 106, lang: 'en', country: 'US', popularity: 60 }),
  movie(693134, 'Dune: Part Two', { year: 2024, genres: ['adventure', 'sci-fi'], rating: 8.2, runtime: 167, lang: 'en', country: 'US', popularity: 330 }),
  show(37854, 'One Piece', 'returning', { year: 1999, genres: ['action', 'adventure', 'animation', 'comedy'], rating: 8.7, runtime: 24, lang: 'ja', country: 'JP', popularity: 200 }),
  show(57243, 'Doctor Who', 'ended', { year: 2005, genres: ['adventure', 'drama', 'sci-fi'], rating: 7.5, runtime: 45, lang: 'en', country: 'GB', popularity: 70 }),
  movie(157336, 'Interstellar', { year: 2014, genres: ['adventure', 'drama', 'sci-fi'], rating: 8.5, runtime: 169, lang: 'en', country: 'US', popularity: 250 }),
  show(66573, 'The Good Place', 'ended', { year: 2016, genres: ['comedy', 'fantasy'], rating: 8.1, runtime: 22, lang: 'en', country: 'US', popularity: 80 }),

  // Shows
  show(222766, 'Pluribus', 'returning', { year: 2025, genres: ['drama', 'mystery', 'sci-fi'], rating: 8.0, runtime: 50, lang: 'en', country: 'US', popularity: 360 }),
  show(240411, 'Dan Da Dan', 'returning', { year: 2024, genres: ['action', 'animation', 'comedy', 'sci-fi'], rating: 8.6, runtime: 24, lang: 'ja', country: 'JP', popularity: 150 }),
  show(120089, 'SPY x FAMILY', 'returning', { year: 2022, genres: ['action', 'animation', 'comedy'], rating: 8.5, runtime: 24, lang: 'ja', country: 'JP', popularity: 110 }),
  show(105248, 'Cyberpunk: Edgerunners', 'ended', { year: 2022, genres: ['action', 'animation', 'sci-fi'], rating: 8.5, runtime: 25, lang: 'ja', country: 'JP', popularity: 60 }),
  show(1429, 'Attack on Titan', 'ended', { year: 2013, genres: ['action', 'animation', 'sci-fi'], rating: 8.7, runtime: 24, lang: 'ja', country: 'JP', popularity: 130 }),
  show(4613, 'Mushi-Shi', 'ended', { year: 2005, genres: ['animation', 'drama', 'fantasy', 'mystery'], rating: 8.5, runtime: 24, lang: 'ja', country: 'JP', popularity: 20 }),
  show(136315, 'The Bear', 'returning', { year: 2022, genres: ['comedy', 'drama'], rating: 8.2, runtime: 30, lang: 'en', country: 'US', popularity: 140 }),
  show(125988, 'Silo', 'returning', { year: 2023, genres: ['drama', 'mystery', 'sci-fi'], rating: 8.1, runtime: 50, lang: 'en', country: 'US', popularity: 160 }),
  show(83867, 'Andor', 'ended', { year: 2022, genres: ['action', 'adventure', 'drama', 'sci-fi'], rating: 8.4, runtime: 45, lang: 'en', country: 'US', popularity: 170 }),
  show(126308, 'Shōgun', 'returning', { year: 2024, genres: ['adventure', 'drama', 'war'], rating: 8.6, runtime: 60, lang: 'en', country: 'US', popularity: 190 }),
  show(95480, 'Slow Horses', 'returning', { year: 2022, genres: ['comedy', 'drama', 'mystery'], rating: 8.2, runtime: 45, lang: 'en', country: 'GB', popularity: 100 }),
  show(107113, 'Only Murders in the Building', 'returning', { year: 2021, genres: ['comedy', 'drama', 'mystery'], rating: 8.0, runtime: 30, lang: 'en', country: 'US', popularity: 85 }),
  show(1399, 'Game of Thrones', 'ended', { year: 2011, genres: ['adventure', 'drama', 'fantasy'], rating: 8.5, runtime: 60, lang: 'en', country: 'US', popularity: 260 }),
  show(1396, 'Breaking Bad', 'ended', { year: 2008, genres: ['drama'], rating: 8.9, runtime: 47, lang: 'en', country: 'US', popularity: 220 }),
  show(66732, 'Stranger Things', 'ended', { year: 2016, genres: ['drama', 'mystery', 'sci-fi'], rating: 8.6, runtime: 50, lang: 'en', country: 'US', popularity: 240 }),
  show(94997, 'House of the Dragon', 'returning', { year: 2022, genres: ['action', 'drama', 'fantasy'], rating: 8.4, runtime: 60, lang: 'en', country: 'US', popularity: 230 }),
  show(100088, 'The Last of Us', 'returning', { year: 2023, genres: ['adventure', 'drama'], rating: 8.6, runtime: 55, lang: 'en', country: 'US', popularity: 210 }),
  show(93405, 'Squid Game', 'ended', { year: 2021, genres: ['action', 'drama', 'mystery'], rating: 7.9, runtime: 55, lang: 'ko', country: 'KR', popularity: 180 }),
  show(85552, 'Euphoria', 'returning', { year: 2019, genres: ['drama'], rating: 8.3, runtime: 55, lang: 'en', country: 'US', popularity: 75 }),
  show(290001, 'Neuromancer', 'upcoming', { year: 2026, genres: ['drama', 'sci-fi'], rating: 0, runtime: 55, lang: 'en', country: 'US', popularity: 40 }),
  show(290002, 'Avatar: Seven Havens', 'upcoming', { year: 2026, genres: ['adventure', 'animation', 'fantasy'], rating: 0, runtime: 24, lang: 'en', country: 'US', popularity: 35 }),

  // Movies
  movie(872585, 'Oppenheimer', { year: 2023, genres: ['drama', 'history'], rating: 8.1, runtime: 181, lang: 'en', country: 'US', popularity: 200 }),
  movie(27205, 'Inception', { year: 2010, genres: ['action', 'adventure', 'sci-fi'], rating: 8.4, runtime: 148, lang: 'en', country: 'US', popularity: 190 }),
  movie(438631, 'Dune', { year: 2021, genres: ['adventure', 'sci-fi'], rating: 7.8, runtime: 155, lang: 'en', country: 'US', popularity: 170 }),
  movie(129, 'Spirited Away', { year: 2001, genres: ['animation', 'family', 'fantasy'], rating: 8.5, runtime: 125, lang: 'ja', country: 'JP', popularity: 120 }),
  movie(8392, 'My Neighbor Totoro', { year: 1988, genres: ['animation', 'family', 'fantasy'], rating: 8.1, runtime: 86, lang: 'ja', country: 'JP', popularity: 70 }),
  movie(4977, 'Paprika', { year: 2006, genres: ['animation', 'sci-fi', 'thriller'], rating: 7.8, runtime: 90, lang: 'ja', country: 'JP', popularity: 40 }),
  movie(496243, 'Parasite', { year: 2019, genres: ['comedy', 'drama', 'thriller'], rating: 8.5, runtime: 133, lang: 'ko', country: 'KR', popularity: 150 }),
  movie(329865, 'Arrival', { year: 2016, genres: ['drama', 'mystery', 'sci-fi'], rating: 7.6, runtime: 116, lang: 'en', country: 'US', popularity: 90 }),
  movie(976893, 'Perfect Days', { year: 2023, genres: ['drama'], rating: 7.8, runtime: 124, lang: 'ja', country: 'JP', popularity: 35 }),
  movie(965150, 'Aftersun', { year: 2022, genres: ['drama'], rating: 7.7, runtime: 101, lang: 'en', country: 'GB', popularity: 45 }),
  movie(414419, 'Columbus', { year: 2017, genres: ['drama', 'romance'], rating: 7.2, runtime: 104, lang: 'en', country: 'US', popularity: 15 }),
  movie(545611, 'Everything Everywhere All at Once', { year: 2022, genres: ['action', 'adventure', 'comedy', 'sci-fi'], rating: 7.8, runtime: 139, lang: 'en', country: 'US', popularity: 130 }),
  movie(120, 'The Lord of the Rings: The Fellowship of the Ring', { year: 2001, genres: ['action', 'adventure', 'fantasy'], rating: 8.4, runtime: 179, lang: 'en', country: 'US', popularity: 160 }),
  movie(238, 'The Godfather', { year: 1972, genres: ['drama'], rating: 8.7, runtime: 175, lang: 'en', country: 'US', popularity: 140 }),
  movie(155, 'The Dark Knight', { year: 2008, genres: ['action', 'drama', 'thriller'], rating: 8.5, runtime: 152, lang: 'en', country: 'US', popularity: 180 }),
  movie(194, 'Amélie', { year: 2001, genres: ['comedy', 'romance'], rating: 7.9, runtime: 122, lang: 'fr', country: 'FR', popularity: 60 }),
  movie(1124, 'The Prestige', { year: 2006, genres: ['drama', 'mystery', 'sci-fi'], rating: 8.2, runtime: 130, lang: 'en', country: 'US', popularity: 95 }),
  movie(13, 'Forrest Gump', { year: 1994, genres: ['comedy', 'drama', 'romance'], rating: 8.5, runtime: 142, lang: 'en', country: 'US', popularity: 170 }),
  movie(508442, 'Soul', { year: 2020, genres: ['animation', 'comedy', 'family', 'fantasy'], rating: 8.0, runtime: 100, lang: 'en', country: 'US', popularity: 100 }),
  movie(1022789, 'Inside Out 2', { year: 2024, genres: ['adventure', 'animation', 'comedy', 'family'], rating: 7.6, runtime: 97, lang: 'en', country: 'US', popularity: 210 }),
  movie(324857, 'Spider-Man: Into the Spider-Verse', { year: 2018, genres: ['action', 'adventure', 'animation', 'sci-fi'], rating: 8.4, runtime: 117, lang: 'en', country: 'US', popularity: 150 }),
  movie(335984, 'Blade Runner 2049', { year: 2017, genres: ['drama', 'sci-fi'], rating: 7.6, runtime: 164, lang: 'en', country: 'US', popularity: 110 }),
  movie(76341, 'Mad Max: Fury Road', { year: 2015, genres: ['action', 'adventure', 'sci-fi'], rating: 7.6, runtime: 121, lang: 'en', country: 'AU', popularity: 120 }),
  movie(530915, '1917', { year: 2019, genres: ['action', 'drama', 'war'], rating: 8.0, runtime: 119, lang: 'en', country: 'GB', popularity: 90 }),
  movie(497, 'The Green Mile', { year: 1999, genres: ['drama', 'fantasy'], rating: 8.5, runtime: 189, lang: 'en', country: 'US', popularity: 100 }),
  movie(11216, 'Cinema Paradiso', { year: 1988, genres: ['drama', 'romance'], rating: 8.4, runtime: 124, lang: 'it', country: 'IT', popularity: 30 }),
  movie(372058, 'Your Name.', { year: 2016, genres: ['animation', 'drama', 'romance'], rating: 8.5, runtime: 106, lang: 'ja', country: 'JP', popularity: 110 }),
  movie(1011985, 'Kung Fu Panda 4', { year: 2024, genres: ['action', 'animation', 'comedy', 'family'], rating: 7.1, runtime: 94, lang: 'en', country: 'US', popularity: 130 }),
];
