import type {
  HDRezkaCandidate,
  HDRezkaMatch,
  HDRezkaQuality,
  HDRezkaSeason,
  HDRezkaStreams,
  HDRezkaSubtitle,
  HDRezkaTitle,
  HDRezkaTranslator,
  MediaType,
} from '@/lib/api/contract';
import { buildSeason, findByLibraryId, type MockLibrary } from './titles';

/**
 * HDRezka as the backend sees it (#63): a few titles with their translators,
 * qualities and subtitles, and which ones a TMDB title's search finds.
 * Names are the site's own (Russian), as the backend passes them through.
 *
 * | Title | Search finds |
 * |---|---|
 * | One Battle After Another | an exact match (six voice-overs: one premium, ads, CAMRip, director's cut) and a weak one |
 * | Interstellar | an exact match and a likely one (a documentary) |
 * | Dune: Part Two | an exact match, Dune (2021) as likely and Dune (1984) as weak |
 * | Severance, Frieren | an exact series match each, with seasons and episodes |
 *
 * A query with "zzz" in it finds nothing.
 */

export const HDREZKA_URL = 'https://rezka.ag';

export interface TitleSeed {
  path: string;
  title: string;
  original_title: string | null;
  year: number | null;
  /** What search says; the title page always knows. */
  search_kind: HDRezkaCandidate['kind'];
  kind: HDRezkaTitle['kind'];
  rating: number | null;
  translators: HDRezkaTranslator[];
  /** Minutes: a movie's, or an episode's. */
  runtime: number;
  /** Series: episodes per season, in HDRezka's numbers. */
  seasons?: Array<[season: number, episodes: number]>;
  /** The library title it is, for episode annotations. */
  tmdb?: [MediaType, number];
}

const translator = (id: number, name: string, flags: Partial<Omit<HDRezkaTranslator, 'id' | 'name'>> = {}): HDRezkaTranslator => ({
  id,
  name,
  default: false,
  premium: false,
  camrip: false,
  ads: false,
  director: false,
  ...flags,
});

/** The original audio: the same id on every title. */
const ORIGINAL = translator(238, 'Оригинал (+субтитры)');

const movie = (seed: Omit<TitleSeed, 'search_kind' | 'kind'>): TitleSeed => ({ ...seed, search_kind: 'movie', kind: 'movie' });

const TITLES: TitleSeed[] = [
  movie({
    path: '/films/action/78655-bitva-za-bitvoy-2025.html',
    title: 'Битва за битвой',
    original_title: 'One Battle After Another',
    year: 2025,
    rating: 7.9,
    runtime: 162,
    tmdb: ['movie', 1054867],
    translators: [
      translator(111, 'HDrezka Studio', { default: true }),
      ORIGINAL,
      translator(56, 'Дубляж', { premium: true }),
      translator(35, 'TVShows', { ads: true }),
      translator(405, 'Red Head Sound', { camrip: true }),
      translator(376, 'Jaskier', { director: true }),
    ],
  }),
  movie({
    path: '/films/comedy/25403-bitva-polov-2017.html',
    title: 'Битва полов',
    original_title: 'Battle of the Sexes',
    year: 2017,
    rating: 6.9,
    runtime: 121,
    translators: [translator(56, 'Дубляж', { default: true }), ORIGINAL],
  }),
  movie({
    path: '/films/fiction/2259-interstellar-2014.html',
    title: 'Интерстеллар',
    original_title: 'Interstellar',
    year: 2014,
    rating: 8.6,
    runtime: 169,
    tmdb: ['movie', 157336],
    translators: [translator(56, 'Дубляж', { default: true }), ORIGINAL, translator(13, 'Гоблин'), translator(1, 'Дубляж (Ultra HD)', { premium: true })],
  }),
  movie({
    path: '/films/documentary/10432-nauka-interstellara-2015.html',
    title: 'Наука «Интерстеллара»',
    original_title: 'The Science of Interstellar',
    year: 2015,
    rating: 7.4,
    runtime: 52,
    translators: [translator(35, 'TVShows', { default: true })],
  }),
  movie({
    path: '/films/fiction/64829-dyuna-chast-vtoraya-2024.html',
    title: 'Дюна: Часть вторая',
    original_title: 'Dune: Part Two',
    year: 2024,
    rating: 8.3,
    runtime: 167,
    tmdb: ['movie', 693134],
    translators: [translator(56, 'Дубляж', { default: true }), ORIGINAL, translator(111, 'HDrezka Studio')],
  }),
  movie({
    path: '/films/fiction/40683-dyuna-2021.html',
    title: 'Дюна',
    original_title: 'Dune',
    year: 2021,
    rating: 7.8,
    runtime: 155,
    translators: [translator(56, 'Дубляж', { default: true }), ORIGINAL],
  }),
  movie({
    path: '/films/fiction/9031-dyuna-1984.html',
    title: 'Дюна',
    original_title: 'Dune',
    year: 1984,
    rating: 6.6,
    runtime: 137,
    translators: [translator(7, 'Многоголосый', { default: true })],
  }),
  {
    path: '/series/thriller/47283-razdelenie-2022.html',
    title: 'Разделение',
    original_title: 'Severance',
    year: 2022,
    search_kind: 'series',
    kind: 'series',
    rating: 8.4,
    runtime: 52,
    seasons: [
      [1, 9],
      [2, 9],
    ],
    tmdb: ['tv', 95396],
    translators: [translator(111, 'HDrezka Studio', { default: true }), ORIGINAL, translator(10, 'LostFilm')],
  },
  {
    path: '/animation/adventures/61478-provozhayuschaya-v-posledniy-put-friren-2023.html',
    title: 'Провожающая в последний путь Фрирен',
    original_title: 'Sousou no Frieren',
    year: 2023,
    // Anime: the category doesn't say movie or series
    search_kind: null,
    kind: 'series',
    rating: 8.9,
    runtime: 24,
    seasons: [[1, 38]],
    tmdb: ['tv', 209867],
    translators: [translator(82, 'AniLibria', { default: true }), ORIGINAL],
  },
];

/** What a search without `q` finds for a TMDB title, best first. */
const SEARCHES: Record<string, Array<[path: string, match: HDRezkaMatch]>> = {
  'movie:1054867': [
    ['/films/action/78655-bitva-za-bitvoy-2025.html', 'exact'],
    ['/films/comedy/25403-bitva-polov-2017.html', 'weak'],
  ],
  'movie:157336': [
    ['/films/fiction/2259-interstellar-2014.html', 'exact'],
    ['/films/documentary/10432-nauka-interstellara-2015.html', 'likely'],
  ],
  'movie:693134': [
    ['/films/fiction/64829-dyuna-chast-vtoraya-2024.html', 'exact'],
    ['/films/fiction/40683-dyuna-2021.html', 'likely'],
    ['/films/fiction/9031-dyuna-1984.html', 'weak'],
  ],
  'tv:95396': [['/series/thriller/47283-razdelenie-2022.html', 'exact']],
  'tv:209867': [['/animation/adventures/61478-provozhayuschaya-v-posledniy-put-friren-2023.html', 'exact']],
};

const category = (path: string) => path.split('/')[1] as HDRezkaCandidate['category'];

function candidate(seed: TitleSeed, match: HDRezkaMatch): HDRezkaCandidate {
  return {
    url: HDREZKA_URL + seed.path,
    path: seed.path,
    title: seed.title,
    original_title: seed.original_title,
    year: seed.year,
    kind: seed.search_kind,
    category: category(seed.path),
    rating: seed.rating,
    match,
  };
}

const normalize = (text: string) =>
  text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();

/** GET /hdrezka/search: the TMDB title's hits, or the titles whose names contain `q` (its trailing year aside). */
export function searchHDRezka(mediaType: MediaType, tmdbId: number, q: string | null): HDRezkaCandidate[] {
  if (q === null) {
    return (SEARCHES[`${mediaType}:${tmdbId}`] ?? []).flatMap(([path, match]) => {
      const seed = findSeed(path);
      return seed ? [candidate(seed, match)] : [];
    });
  }
  const wanted = normalize(q.replace(/\b(19|20)\d\d\s*$/, ''));
  if (!wanted || wanted.includes('zzz')) return [];
  const graded = TITLES.flatMap((seed) => {
    const names = [seed.title, seed.original_title].filter((name): name is string => name !== null).map(normalize);
    if (names.includes(wanted)) return [candidate(seed, 'exact')];
    if (names.some((name) => name.includes(wanted) || wanted.includes(name))) return [candidate(seed, 'likely')];
    return [];
  });
  const order: HDRezkaMatch[] = ['exact', 'likely', 'weak'];
  return graded.sort((a, b) => order.indexOf(a.match) - order.indexOf(b.match));
}

/** A URL on any mirror, or a bare path → the path; null when it isn't a title page. */
export function titlePath(url: string): string | null {
  const path = url.trim().replace(/^https?:\/\/[^/]+/i, '').replace(/[?#].*$/, '');
  return /^\/(films|series|cartoons|animation)\/[a-z0-9_-]+\/\d+-[a-z0-9_-]+\.html$/.test(path) ? path : null;
}

export const findSeed = (path: string) => TITLES.find((seed) => seed.path === path);

/** The scenario where every voice-over needs Premium. */
const premiumOnly = (translators: HDRezkaTranslator[]) => translators.map((t) => ({ ...t, premium: true }));

export function seedTranslators(seed: TitleSeed, everyPremium: boolean): HDRezkaTranslator[] {
  return everyPremium ? premiumOnly(seed.translators) : seed.translators;
}

const postId = (path: string) => Number(/\/(\d+)-/.exec(path)?.[1] ?? 0);

/** HDRezka's episodes, each with the library episode of the same number when the show is the library's. */
function seasons(seed: TitleSeed, library: MockLibrary, showId: number | null): HDRezkaSeason[] {
  const found = showId !== null ? findByLibraryId(library, 'tv', showId) : undefined;
  const mapped = found && seed.tmdb?.[0] === 'tv' && seed.tmdb[1] === found.entry.tmdb_id ? found.entry.tmdb_id : null;
  return (seed.seasons ?? []).map(([number, count]) => {
    const episodes = mapped !== null ? (buildSeason(mapped, number, library)?.episodes ?? []) : [];
    return {
      number,
      episodes: Array.from({ length: count }, (_, i) => {
        const episode = episodes.find((e) => e.episode_number === i + 1);
        return {
          number: i + 1,
          library: episode
            ? {
                episode_id: episode.id,
                season: episode.season_number,
                episode: episode.episode_number,
                name: episode.name,
                air_date: episode.air_date,
                has_file: episode.webdav_path !== null,
                source: episode.source,
                stream: episode.stream,
              }
            : null,
        };
      }),
    };
  });
}

/**
 * GET /hdrezka/titles: the requested translator, else the one a movie
 * already streams from this title, else the page's default.
 */
export function buildHDRezkaTitle(
  seed: TitleSeed,
  library: MockLibrary,
  options: { translatorId: number | null; showId: number | null; movieId: number | null; everyPremium: boolean }
): HDRezkaTitle {
  const translators = seedTranslators(seed, options.everyPremium);
  const movieRow = options.movieId !== null ? findByLibraryId(library, 'movie', options.movieId)?.row : undefined;
  const current =
    movieRow?.stream && titlePath(movieRow.stream.url) === seed.path ? translators.find((t) => t.name === movieRow.stream?.translator) : undefined;
  const chosen =
    translators.find((t) => t.id === options.translatorId) ?? current ?? translators.find((t) => t.default) ?? translators[0];
  return {
    url: HDREZKA_URL + seed.path,
    path: seed.path,
    post_id: postId(seed.path),
    kind: seed.kind,
    title: seed.title,
    original_title: seed.original_title,
    year: seed.year,
    translators,
    translator_id: chosen.id,
    seasons: seed.kind === 'series' ? seasons(seed, library, options.showId) : [],
  };
}

/** Decimal GB an hour of each quality, as HDRezka's MP4s run. */
const GB_PER_HOUR: Array<[label: string, resolution: number, rate: number]> = [
  ['360p', 360, 0.35],
  ['480p', 480, 0.6],
  ['720p', 720, 1.0],
  ['1080p', 1080, 1.6],
];

/**
 * GET /hdrezka/streams: 360p–1080p with sizes, plus a premium "1080p Ultra".
 * A CAMRip tops out at 720p. The original audio has English and Russian
 * subtitles; the dubs have none.
 */
export function buildStreams(seed: TitleSeed, translator: HDRezkaTranslator): HDRezkaStreams {
  const hours = seed.runtime / 60;
  const qualities: HDRezkaQuality[] = GB_PER_HOUR.map(([label, resolution, rate]) => {
    const available = !(translator.camrip && resolution > 720);
    return { label, resolution, premium: false, available, size_bytes: available ? Math.round(rate * hours * 1e9) : null };
  });
  qualities.push({ label: '1080p Ultra', resolution: 1080, premium: true, available: true, size_bytes: null });
  const subtitles: HDRezkaSubtitle[] =
    translator.id === ORIGINAL.id
      ? [
          { language_code: 'en', language_name: 'English' },
          { language_code: 'ru', language_name: 'Русский' },
        ]
      : [];
  return { qualities, subtitles };
}
