import type { CastMember, DownloadProgress, EpisodeState, EpisodeType, MediaType, TitleTorrent, WatchState } from '@/lib/api/contract';
import { iso } from './catalog';

/**
 * What TMDB and the library hold for each catalog title, with one realistic
 * stage per title (PLAN §6 availability rules; titles.ts builds the answers):
 *
 * | Title | Stage |
 * |---|---|
 * | Dune: Part Two | ready, with a torrent |
 * | Interstellar | downloading |
 * | One Battle After Another | needs a torrent |
 * | Past Lives, The Good Place | not in the library |
 * | Frieren | needs a torrent: S01E29–E38 missing; mapping job 101 has 2 files to review |
 * | One Piece | ready (a gap at the start of Wano doesn't count), absolute numbers, arcs; job 102 has 1 file to review |
 * | Doctor Who | ready (a gap in S3 doesn't count), named specials; job 103 has 3 files to review (a conflict) |
 * | Severance | ready, one upcoming episode |
 * | Bluey | ready, 52-episode seasons |
 * | The Studio | downloading |
 *
 * No episode is seeded `needs_review`: that state comes from the open mapping
 * jobs (fixtures/assignment.ts), for episodes that have no file.
 */

const DAY = 86_400_000;

export interface TitleSeed {
  color: string | null;
  vote_average: number;
  vote_count: number;
  original_language: string;
  cast: CastMember[];
  /** Directors (movies) or creators (shows). */
  crew: string[];
  trailer_youtube_key: string | null;
  recommendation_reason: string | null;
  watch_state: WatchState | null;
  similar: Array<[MediaType, number]>;
  /** In the seeded library. */
  library?: { id: number; added_days_ago: number };
}

export interface MovieSeed extends TitleSeed {
  torrent?: TitleTorrent;
  download?: DownloadProgress;
}

export interface EpisodeSeed {
  n: number;
  name: string;
  air_date: string | null;
  runtime: number;
  /** As seeded; without a file it becomes missing or upcoming (or needs_review, from a mapping job). */
  state: Exclude<EpisodeState, 'needs_review'>;
  pct?: number;
  episode_type?: EpisodeType;
  overview?: string;
}

export interface SeasonSeed {
  n: number;
  name: string;
  /** Where absolute numbers start, for shows whose earlier seasons the mock leaves out. */
  absolute_from?: number;
  /** TMDB story arcs, in in-season episode numbers. */
  arcs?: Array<{ label: string; from: number; to: number }>;
  episodes: EpisodeSeed[];
}

export interface ShowSeed extends TitleSeed {
  /** Keeps episode ids unique across shows: key × 100,000 + season × 1,000 + episode. */
  key: number;
  status: string;
  networks: string[];
  end_year: number | null;
  episode_run_time: number;
  seeders?: number;
  /** The release the episodes' files came from (one per show here; subtitles rank against it). */
  release?: string;
  seasons: SeasonSeed[];
}

const cast = (...pairs: Array<[string, string]>): CastMember[] =>
  pairs.map(([name, character]) => ({ name, character, profile_path: null }));

/** `i` steps of `step` days after `start` (YYYY-MM-DD). */
const after = (start: string, i: number, step = 7) =>
  new Date(Date.parse(`${start}T00:00:00Z`) + i * step * DAY).toISOString().slice(0, 10);

/** Weekly episodes from `start`, numbered from `from`. */
function run(
  names: string[],
  start: string,
  runtime: number,
  state: (n: number) => EpisodeSeed['state'],
  { from = 1, step = 7 }: { from?: number; step?: number } = {}
): EpisodeSeed[] {
  return names.map((name, i) => ({ n: from + i, name, air_date: after(start, i, step), runtime, state: state(from + i) }));
}

const numbered = (count: number, first = 1, label = 'Episode') => Array.from({ length: count }, (_, i) => `${label} ${first + i}`);

function amend(episodes: EpisodeSeed[], n: number, patch: Partial<EpisodeSeed>): EpisodeSeed[] {
  return episodes.map((episode) => (episode.n === n ? { ...episode, ...patch } : episode));
}

const FRIEREN_S1 = [
  'The Journey’s End', 'It Didn’t Have to Be Magic…', 'Killing Magic', 'The Land Where Souls Rest', 'Phantoms of the Dead',
  'The Hero of the Village', 'Like a Fairy Tale', 'Frieren the Slayer', 'Aura the Guillotine', 'A Powerful Mage',
  'Winter in the Northern Lands', 'A Real Hero', 'Aversion to One’s Own Kind', 'Privilege of the Young', 'Smells Like Trouble',
  'Long-Lived Friends', 'Take Care', 'First-Class Mage Exam', 'Well-Laid Plans', 'Necessary Killing',
  'The World of Magic', 'Future Enemies', 'Conquering the Labyrinth', 'Perfect Duplicates', 'A Fatal Vulnerability',
  'The Height of Magic', 'An Ordinary Mage', 'It Would Be Embarrassing When We Met Again', 'Let’s Go Somewhere We Can’t Reach',
  'Mimic', 'Ghosts of the Past', 'The Path Ahead', 'Unforgettable Words', 'Sein’s Choice', 'The Hero of the South',
  'Battle at the Fortress', 'A Mage’s Promise', 'Journey’s Continuation',
];

const DOCTOR_WHO = [
  ['Rose', 'The End of the World', 'The Unquiet Dead', 'Aliens of London (1)', 'World War Three (2)', 'Dalek', 'The Long Game',
    'Father’s Day', 'The Empty Child (1)', 'The Doctor Dances (2)', 'Boom Town', 'Bad Wolf (1)', 'The Parting of the Ways (2)'],
  ['New Earth', 'Tooth and Claw', 'School Reunion', 'The Girl in the Fireplace', 'Rise of the Cybermen (1)', 'The Age of Steel (2)',
    'The Idiot’s Lantern', 'The Impossible Planet (1)', 'The Satan Pit (2)', 'Love & Monsters', 'Fear Her', 'Army of Ghosts (1)', 'Doomsday (2)'],
  ['Smith and Jones', 'The Shakespeare Code', 'Gridlock', 'Daleks in Manhattan (1)', 'Evolution of the Daleks (2)', 'The Lazarus Experiment',
    '42', 'Human Nature (1)', 'The Family of Blood (2)', 'Blink', 'Utopia (1)', 'The Sound of Drums (2)', 'Last of the Time Lords (3)'],
  ['Partners in Crime', 'The Fires of Pompeii', 'Planet of the Ood', 'The Sontaran Stratagem (1)', 'The Poison Sky (2)', 'The Doctor’s Daughter',
    'The Unicorn and the Wasp', 'Silence in the Library (1)', 'Forest of the Dead (2)', 'Midnight', 'Turn Left', 'The Stolen Earth (1)', 'Journey’s End (2)'],
];

export const SHOWS: Record<number, ShowSeed> = {
  209867: {
    key: 0,
    release: '[SubsPlease] Sousou no Frieren (01-28) (1080p) [Batch]',
    color: '#2f8f7f',
    vote_average: 8.9,
    vote_count: 512,
    original_language: 'ja',
    cast: cast(
      ['Atsumi Tanezaki', 'Frieren'],
      ['Kana Ichinose', 'Fern'],
      ['Chiaki Kobayashi', 'Stark'],
      ['Nobuhiko Okamoto', 'Himmel'],
    ),
    crew: ['Keiichirō Saitō'],
    trailer_youtube_key: null,
    recommendation_reason: 'Because you loved Mushishi: quiet, melancholic and kind.',
    watch_state: { watched_at: iso(3), plays: 26, rating: 10, progress: { completed: 26, aired: 38 } },
    similar: [['tv', 37854], ['tv', 82728], ['tv', 66573]],
    library: { id: 1, added_days_ago: 120 },
    status: 'Returning Series',
    networks: ['Nippon TV'],
    end_year: null,
    episode_run_time: 24,
    seasons: [
      {
        n: 0,
        name: 'Specials',
        // TMDB's order (the design's): the recap sits between the mini anime. Job 101 maps "28.5" to S00E03.
        episodes: [
          { n: 1, name: '“Frieren” Mini Anime 1', air_date: '2023-10-06', runtime: 2, state: 'ready' },
          { n: 2, name: '“Frieren” Mini Anime 2', air_date: '2023-10-13', runtime: 2, state: 'ready' },
          { n: 3, name: 'Recap: The Journey So Far', air_date: '2024-03-15', runtime: 24, state: 'missing' },
          { n: 4, name: '“Frieren” Mini Anime 3', air_date: '2024-03-22', runtime: 2, state: 'ready' },
          { n: 5, name: '“Frieren” Mini Anime 4', air_date: '2024-03-29', runtime: 2, state: 'missing' },
          { n: 6, name: '“Frieren” Mini Anime 5', air_date: '2024-04-05', runtime: 2, state: 'missing' },
        ],
      },
      {
        n: 1,
        name: 'Season 1',
        episodes: amend(
          amend(
            [
              ...run(FRIEREN_S1.slice(0, 28), '2023-09-29', 24, (n) => (n === 26 || n === 28 ? 'ready' : 'watched')),
              // The second cour, after a long break: releases call it season 2.
              ...run(FRIEREN_S1.slice(28), '2026-01-16', 24, () => 'missing', { from: 29 }),
            ],
            28,
            { episode_type: 'mid_season', overview: 'The exam ends, and Frieren is reminded of a promise she made to Himmel.' }
          ),
          38,
          { episode_type: 'finale' }
        ),
      },
    ],
  },
  37854: {
    key: 1,
    release: 'One.Piece.S22.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG',
    color: '#b5532a',
    vote_average: 8.7,
    vote_count: 4870,
    original_language: 'ja',
    cast: cast(
      ['Mayumi Tanaka', 'Monkey D. Luffy'],
      ['Kazuya Nakai', 'Roronoa Zoro'],
      ['Akemi Okamura', 'Nami'],
      ['Kappei Yamaguchi', 'Usopp'],
    ),
    crew: ['Eiichiro Oda'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: { watched_at: iso(5), plays: 181, rating: 9, progress: { completed: 181, aired: 192 } },
    similar: [['tv', 209867], ['tv', 82728], ['tv', 57243]],
    library: { id: 2, added_days_ago: 400 },
    status: 'Returning Series',
    networks: ['Fuji TV'],
    end_year: null,
    episode_run_time: 24,
    seasons: [
      {
        n: 0,
        name: 'Specials',
        episodes: amend(
          run(
            ['Episode of Nami', 'Episode of Luffy', 'Episode of Merry', 'Episode of Sabo', '3D2Y', 'Heart of Gold', 'Episode of East Blue',
              'Episode of Sky Island', ...numbered(34, 9, 'Special')],
            '2005-12-18',
            100,
            (n) => (n <= 8 ? 'ready' : 'missing'),
            { step: 120 }
          ),
          41,
          { name: 'One Piece Fan Letter', air_date: '2024-10-20', runtime: 24 }
        ),
      },
      {
        n: 21,
        name: 'Wano Country',
        absolute_from: 892,
        arcs: [
          { label: 'Wano: Act 1', from: 1, to: 25 },
          { label: 'Wano: Act 2', from: 26, to: 66 },
          { label: 'Wano: Act 3', from: 67, to: 156 },
        ],
        // #892–#897 have no file: a gap before the last file (job 102 maps them), so it doesn't count
        episodes: amend(
          run(numbered(156, 892), '2019-07-07', 24, (n) => (n <= 6 ? 'missing' : 'watched')),
          156,
          { episode_type: 'finale' }
        ),
      },
      {
        n: 22,
        name: 'Egghead',
        absolute_from: 1086,
        arcs: [
          { label: 'Egghead', from: 1, to: 30 },
          { label: 'Elbaph', from: 31, to: 38 },
        ],
        // #1086–#1110 watched, up to #1121 ready, #1122–#1123 still to air.
        episodes: run(numbered(38, 1086), '2026-02-08', 24, (n) => (n <= 25 ? 'watched' : n <= 36 ? 'ready' : 'upcoming')),
      },
    ],
  },
  57243: {
    key: 2,
    release: 'Doctor.Who.2005.S01-S04.1080p.BluRay.x264-SHORTBREHD',
    color: '#1f4f8f',
    vote_average: 7.5,
    vote_count: 3120,
    original_language: 'en',
    cast: cast(
      ['David Tennant', 'The Doctor'],
      ['Christopher Eccleston', 'The Doctor'],
      ['Billie Piper', 'Rose Tyler'],
      ['Freema Agyeman', 'Martha Jones'],
      ['Catherine Tate', 'Donna Noble'],
    ),
    crew: ['Russell T Davies'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: { watched_at: iso(40), plays: 29, rating: 8, progress: { completed: 26, aired: 52 } },
    similar: [['tv', 66573], ['tv', 95396], ['tv', 209867]],
    library: { id: 3, added_days_ago: 300 },
    status: 'Ended',
    networks: ['BBC One'],
    end_year: 2022,
    episode_run_time: 45,
    seasons: [
      {
        n: 0,
        name: 'Specials',
        episodes: [
          ['The Christmas Invasion', '2005-12-25'], ['The Runaway Bride', '2006-12-25'], ['Voyage of the Damned', '2007-12-25'],
          ['The Next Doctor', '2008-12-25'], ['Planet of the Dead', '2009-04-11'], ['The Waters of Mars', '2009-11-15'],
          ['The End of Time (1)', '2009-12-25'], ['The End of Time (2)', '2010-01-01'],
        ].map(([name, air_date], i): EpisodeSeed => ({
          n: i + 1,
          name,
          air_date,
          runtime: 60,
          // Planet of the Dead (S00E05) has no file: job 103 maps it, and two of its files want S00E06
          state: i < 3 ? 'watched' : i === 4 ? 'missing' : i < 6 ? 'ready' : 'missing',
        })),
      },
      ...(['2005-03-26', '2006-04-15', '2007-03-31', '2008-04-05'] as const).map((start, i): SeasonSeed => ({
        n: i + 1,
        name: `Series ${i + 1}`,
        episodes: amend(
          run(DOCTOR_WHO[i], start, 45, (n) => {
            if (i < 2) return 'watched';
            // A gap before the last file: it doesn't make the show need a torrent.
            if (i === 2 && n >= 4 && n <= 9) return 'missing';
            return 'ready';
          }),
          13,
          { episode_type: 'finale' }
        ).map((episode) =>
          i === 2 && episode.n === 10
            ? { ...episode, overview: 'Sally Sparrow follows messages hidden under old wallpaper, and learns not to blink.' }
            : episode
        ),
      })),
    ],
  },
  95396: {
    key: 3,
    release: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    color: '#2c5a6b',
    vote_average: 8.4,
    vote_count: 2310,
    original_language: 'en',
    cast: cast(
      ['Adam Scott', 'Mark S.'],
      ['Britt Lower', 'Helly R.'],
      ['Zach Cherry', 'Dylan G.'],
      ['Tramell Tillman', 'Mr. Milchick'],
      ['John Turturro', 'Irving B.'],
      ['Patricia Arquette', 'Harmony Cobel'],
    ),
    crew: ['Dan Erickson'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: { watched_at: iso(6), plays: 13, rating: 10, progress: { completed: 13, aired: 18 } },
    similar: [['tv', 247767], ['tv', 66573], ['tv', 57243]],
    library: { id: 4, added_days_ago: 600 },
    status: 'Returning Series',
    networks: ['Apple TV+'],
    end_year: null,
    episode_run_time: 50,
    seasons: [
      {
        n: 1,
        name: 'Season 1',
        episodes: amend(
          run(['Good News About Hell', 'Half Loop', 'In Perpetuity', 'The You You Are', 'The Grim Barbarity of Optics and Design',
            'Hide and Seek', 'Defiant Jazz', 'What’s for Dinner?', 'The We We Are'], '2022-02-18', 55, () => 'watched'),
          9,
          { episode_type: 'finale' }
        ),
      },
      {
        n: 2,
        name: 'Season 2',
        episodes: ['Hello, Ms. Cobel', 'Goodbye, Mrs. Selvig', 'Who Is Alive?', 'Woe’s Hollow', 'Trojan’s Horse', 'Attila',
          'Chikhai Bardo', 'Sweet Vitriol', 'The After Hours', 'Cold Harbor'].map((name, i): EpisodeSeed => {
          const n = i + 1;
          // Weekly up to last week; the finale airs after a week off.
          const air_date = n < 10 ? iso(70 - 7 * n).slice(0, 10) : iso(-7).slice(0, 10);
          return {
            n,
            name,
            air_date,
            runtime: 52,
            state: n <= 4 ? 'watched' : n <= 9 ? 'ready' : 'upcoming',
            ...(n === 5 ? { overview: 'Mark’s reintegration speeds up while the innies prepare for a visit nobody planned.' } : {}),
            ...(n === 10 ? { episode_type: 'finale' as const } : {}),
          };
        }),
      },
    ],
  },
  82728: {
    key: 4,
    release: 'Bluey.S03.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb',
    color: '#3d7fc4',
    vote_average: 8.7,
    vote_count: 410,
    original_language: 'en',
    cast: cast(['Dave McCormack', 'Bandit (voice)'], ['Melanie Zanetti', 'Chilli (voice)']),
    crew: ['Joe Brumm'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: { watched_at: iso(1), plays: 143, rating: 10, progress: { completed: 143, aired: 153 } },
    similar: [['tv', 209867], ['tv', 37854], ['tv', 66573]],
    library: { id: 5, added_days_ago: 200 },
    status: 'Returning Series',
    networks: ['ABC Kids'],
    end_year: null,
    episode_run_time: 7,
    seasons: [
      { n: 1, name: 'Season 1', episodes: run(numbered(52), '2018-10-01', 7, () => 'watched') },
      { n: 2, name: 'Season 2', episodes: run(numbered(52), '2020-03-17', 7, () => 'watched') },
      { n: 3, name: 'Season 3', episodes: run(numbered(49), '2021-09-05', 7, (n) => (n < 40 ? 'watched' : 'ready')) },
    ],
  },
  247767: {
    key: 5,
    release: 'The.Studio.2025.S01.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265-FLUX',
    color: '#8a5a2b',
    vote_average: 7.9,
    vote_count: 220,
    original_language: 'en',
    cast: cast(
      ['Seth Rogen', 'Matt Remick'],
      ['Catherine O’Hara', 'Patty Leigh'],
      ['Ike Barinholtz', 'Sal Saperstein'],
      ['Kathryn Hahn', 'Maya Mason'],
    ),
    crew: ['Seth Rogen', 'Evan Goldberg'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: { watched_at: iso(2), plays: 6, rating: 8, progress: { completed: 6, aired: 10 } },
    similar: [['tv', 95396], ['tv', 66573]],
    library: { id: 6, added_days_ago: 30 },
    status: 'Returning Series',
    networks: ['Apple TV+'],
    end_year: null,
    episode_run_time: 30,
    seeders: 41,
    seasons: [
      {
        n: 1,
        name: 'Season 1',
        episodes: amend(
          run(['The Promotion', 'The Oner', 'The Note', 'The Missing Reel', 'The War', 'The Pediatric Oncologist', 'The Golden Globes',
            'The Presentation', 'CinemaCon', 'The Pitch'], '2025-03-26', 30, (n) => (n <= 6 ? 'watched' : n <= 9 ? 'ready' : 'downloading')),
          10,
          { pct: 37, episode_type: 'finale' }
        ),
      },
    ],
  },
  66573: {
    key: 6,
    color: '#3a8a6a',
    vote_average: 8.1,
    vote_count: 2950,
    original_language: 'en',
    cast: cast(
      ['Kristen Bell', 'Eleanor Shellstrop'],
      ['Ted Danson', 'Michael'],
      ['William Jackson Harper', 'Chidi Anagonye'],
      ['Jameela Jamil', 'Tahani Al-Jamil'],
      ['D’Arcy Carden', 'Janet'],
      ['Manny Jacinto', 'Jason Mendoza'],
    ),
    crew: ['Michael Schur'],
    trailer_youtube_key: null,
    recommendation_reason: 'The same “what is this place, really?” mystery as Severance, played for laughs.',
    watch_state: null,
    similar: [['tv', 95396], ['tv', 247767], ['tv', 57243]],
    status: 'Ended',
    networks: ['NBC'],
    end_year: 2020,
    episode_run_time: 22,
    seasons: [
      {
        n: 1,
        name: 'Season 1',
        episodes: run(['Everything Is Fine', 'Flying', 'Tahani Al-Jamil', 'Jason Mendoza', 'Category 55 Emergency Doomsday Crisis',
          'What We Owe to Each Other', 'The Eternal Shriek', 'Most Improved Player', '…Someone Like Me as a Member', 'Chidi’s Choice',
          'What’s My Motivation', 'Mindy St. Claire', 'Michael’s Gambit'], '2016-09-19', 22, () => 'missing'),
      },
      { n: 2, name: 'Season 2', episodes: run(numbered(13), '2017-09-20', 22, () => 'missing') },
      { n: 3, name: 'Season 3', episodes: run(numbered(13), '2018-09-27', 22, () => 'missing') },
      { n: 4, name: 'Season 4', episodes: run(numbered(14), '2019-09-26', 22, () => 'missing') },
    ],
  },
};

export const MOVIES: Record<number, MovieSeed> = {
  693134: {
    color: '#a0662a',
    vote_average: 8.2,
    vote_count: 6800,
    original_language: 'en',
    cast: cast(
      ['Timothée Chalamet', 'Paul Atreides'],
      ['Zendaya', 'Chani'],
      ['Rebecca Ferguson', 'Lady Jessica'],
      ['Javier Bardem', 'Stilgar'],
      ['Austin Butler', 'Feyd-Rautha'],
    ),
    crew: ['Denis Villeneuve'],
    trailer_youtube_key: 'Way9Dexny3w',
    recommendation_reason: null,
    watch_state: { watched_at: iso(3), plays: 1, rating: 9 },
    similar: [['movie', 157336], ['movie', 1054867], ['movie', 666277]],
    library: { id: 1, added_days_ago: 12 },
    torrent: {
      info_hash: '4f1a2b3c4d5e6f708192a3b4c5d6e7f8091abe90',
      release_name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
      file_name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv',
      size_bytes: 18_400_000_000,
      resolution: '2160p',
      source: 'WEB-DL',
      added_at: iso(12),
      state: 'not_loaded',
      seeders: null,
    },
  },
  157336: {
    color: '#3c4a5c',
    vote_average: 8.5,
    vote_count: 37_000,
    original_language: 'en',
    cast: cast(
      ['Matthew McConaughey', 'Cooper'],
      ['Anne Hathaway', 'Brand'],
      ['Jessica Chastain', 'Murph'],
      ['Michael Caine', 'Professor Brand'],
    ),
    crew: ['Christopher Nolan'],
    trailer_youtube_key: 'zSWdZVtXT7E',
    recommendation_reason: null,
    watch_state: { watched_at: iso(900), plays: 3, rating: 10 },
    similar: [['movie', 693134], ['movie', 1054867], ['movie', 666277]],
    library: { id: 2, added_days_ago: 1 },
    torrent: {
      info_hash: 'c9d4a7b3e5f60718293a4b5c6d7e8f90a1b2c3d4',
      release_name: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
      file_name: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT.mkv',
      size_bytes: 63_100_000_000,
      resolution: '2160p',
      source: 'BluRay REMUX',
      added_at: iso(1),
      state: 'active',
      seeders: 9,
    },
    download: { pct: 42, speed_bps: 0, eta_seconds: null, seeders: 9 },
  },
  1054867: {
    color: '#7a3a2a',
    vote_average: 7.6,
    vote_count: 1400,
    original_language: 'en',
    cast: cast(
      ['Leonardo DiCaprio', 'Bob Ferguson'],
      ['Sean Penn', 'Col. Steven J. Lockjaw'],
      ['Benicio del Toro', 'Sergio St. Carlos'],
      ['Chase Infiniti', 'Willa'],
    ),
    crew: ['Paul Thomas Anderson'],
    trailer_youtube_key: null,
    recommendation_reason: null,
    watch_state: null,
    similar: [['movie', 693134], ['movie', 157336]],
    library: { id: 3, added_days_ago: 2 },
  },
  666277: {
    color: null,
    vote_average: 7.8,
    vote_count: 2100,
    original_language: 'en',
    cast: cast(['Greta Lee', 'Nora'], ['Teo Yoo', 'Hae Sung'], ['John Magaro', 'Arthur']),
    crew: ['Celine Song'],
    trailer_youtube_key: null,
    recommendation_reason: 'Quiet, warm and under two hours.',
    watch_state: null,
    similar: [['movie', 1054867], ['movie', 157336]],
  },
};

// ---- Episode ids and absolute numbers (shared by titles.ts and assignment.ts) ----

/** Library id of a seeded episode: key × 100,000 + season × 1,000 + episode. */
export const episodeIdFor = (key: number, season: number, episode: number) => key * 100_000 + season * 1000 + episode;

/** Absolute numbers by episode id: regular seasons count on (from `absolute_from` where set); specials have none. */
export function absoluteNumbers(show: ShowSeed): Map<number, number> {
  const numbers = new Map<number, number>();
  let absolute = 1;
  for (const season of show.seasons) {
    if (season.n === 0) continue;
    absolute = season.absolute_from ?? absolute;
    for (const episode of season.episodes) numbers.set(episodeIdFor(show.key, season.n, episode.n), absolute++);
  }
  return numbers;
}
