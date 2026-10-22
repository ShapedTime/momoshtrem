import type {
  DownloadProgress,
  EpisodeRange,
  EpisodeStream,
  EpisodeState,
  EpisodeSummary,
  MediaType,
  MissingRun,
  MovieTitle,
  NextEpisode,
  SeasonDetail,
  SeasonSummary,
  ShowEpisodesSeason,
  ShowSettingsPatch,
  ShowTitle,
  SimilarTitle,
  SubtitleItemType,
  Timestamp,
  Title,
  TitleLibraryState,
  TitleStream,
  TitleTorrent,
  TraktSyncStep,
  WatchState,
} from '@/lib/api/contract';
import { type EpisodeFile, type MockReview, NO_REVIEW, seededReview } from './assignment';
import { type CatalogEntry, findTitle, iso } from './catalog';
import {
  EPISODE_BYTES_PER_MINUTE,
  type EpisodeFileKind,
  episodeHold,
  episodeKeep,
  episodeWanted,
  keepOf,
  type LiveHold,
  mockHolds,
  onEpisodeFile,
  onMovieFile,
  scopeHold,
  wantsBytes,
} from './downloads';
import {
  absoluteNumbers,
  type EpisodeKey,
  episodeIdFor,
  hasPlay,
  MOVIES,
  type MovieSeed,
  type SeasonSeed,
  seedEpisode,
  SHOWS,
  type ShowSeed,
  type TitleSeed,
} from './title-seeds';
// recommendations.ts builds titles for its cards: functions only, so the cycle is safe
import { recommendationReason } from './recommendations';
import { traktPlayed, traktRated } from './trakt';

/**
 * Title and season answers built from the seeds (title-seeds.ts) and the
 * page's library: what's added, unassigned or changed lives in a `MockLibrary`
 * that handlers/titles.ts keeps per page. Watch state is what the mock Trakt
 * account has imported so far (fixtures/trakt.ts): nothing while it's off.
 * Keeps and what's downloading come from the page's downloads
 * (fixtures/downloads.ts, #77): a file is downloading only while a download
 * wants it, so a file nothing keeps is ready as soon as it's assigned.
 */

/** The mock WEBDAV_PUBLIC_URL. */
const WEBDAV_PUBLIC_URL = 'https://momoshtrem.local:36911';
const TODAY = iso(0).slice(0, 10);
const DAY = 86_400_000;

type ShowSettings = Required<ShowSettingsPatch>;

/** One title in the mock library. */
export interface MockLibraryEntry {
  media_type: MediaType;
  tmdb_id: number;
  library_id: number;
  added_at: Timestamp;
  /** False for a title added this session, or a movie whose torrent was unassigned: nothing has a file. */
  files: boolean;
  /** Shows: episodes whose file was unassigned. */
  unassigned: Set<number>;
  /** Shows: episodes given a file this session (an applied mapping job). */
  filed: Map<number, EpisodeFile>;
  /** What the open mapping jobs say about this title (kept in sync by handlers/assignment.ts). */
  review: MockReview;
  /** Movies: a torrent assigned this session (an applied mapping job); ready, unless a Keep wants it (then it downloads). */
  assigned?: TitleTorrent;
  /** Movies: an HDRezka stream assigned this session (#63); it replaces any torrent. */
  stream?: TitleStream;
  /** Shows: episodes playing from HDRezka (#63). */
  streamed: Map<number, EpisodeStream>;
  /** Shows. */
  settings: ShowSettings;
}

/** Keyed by `libraryKey(mediaType, tmdbId)`. */
export type MockLibrary = Map<string, MockLibraryEntry>;

export const libraryKey = (mediaType: MediaType, tmdbId: number) => `${mediaType}:${tmdbId}`;

// ---- Paths (the backend's VFS layout) -----------------------------------------

/** library.SanitizeFilename */
const sanitize = (name: string) => name.replace(/[/\\:|]/g, '-').replace(/[*?<>]/g, '').replace(/"/g, "'");
const pad = (n: number) => String(n).padStart(2, '0');
const folder = (entry: CatalogEntry) => `${sanitize(entry.title)} (${entry.year})`;

function moviePath(entry: CatalogEntry, fileName: string): string {
  const ext = /\.[^./]+$/.exec(fileName)?.[0] ?? '.mkv';
  return `/Movies/${folder(entry)}/${folder(entry)}${ext}`;
}

const showPath = (entry: CatalogEntry) => `/TV Shows/${folder(entry)}/`;

/** HDRezka files are always MP4 (#63). */
const episodePath = (entry: CatalogEntry, season: number, episode: number, name: string, ext = '.mkv') =>
  `/TV Shows/${folder(entry)}/Season ${pad(season)}/${sanitize(entry.title)} - S${pad(season)}E${pad(episode)} - ${sanitize(name)}${ext}`;

export const webdavUrl = (path: string | null) => (path === null ? null : WEBDAV_PUBLIC_URL + path.split('/').map(encodeURIComponent).join('/'));

// ---- Library ----------------------------------------------------------------

const defaultSettings = (entry: CatalogEntry): ShowSettings => ({
  episode_ordering: entry.is_anime ? 'absolute' : 'aired',
  episode_group_id: null,
  include_specials: true,
  classifier_mode: 'auto',
});

/** The library a page starts with. */
export function seedLibrary(): MockLibrary {
  const library: MockLibrary = new Map();
  const seeds: Array<[MediaType, Record<number, TitleSeed>]> = [['movie', MOVIES], ['tv', SHOWS]];
  for (const [mediaType, records] of seeds) {
    for (const [tmdbId, seed] of Object.entries(records)) {
      const entry = findTitle(mediaType, Number(tmdbId));
      if (!seed.library || !entry) continue;
      library.set(libraryKey(mediaType, entry.tmdb_id), {
        media_type: mediaType,
        tmdb_id: entry.tmdb_id,
        library_id: seed.library.id,
        added_at: iso(seed.library.added_days_ago),
        files: true,
        unassigned: new Set(),
        filed: new Map(),
        streamed: new Map(),
        review: seededReview(mediaType, seed.library.id),
        settings: defaultSettings(entry),
      });
    }
  }
  return library;
}

/** A title just added: in the library, nothing assigned. */
export function addToLibrary(library: MockLibrary, entry: CatalogEntry): MockLibraryEntry {
  const ids = [...library.values()].filter((row) => row.media_type === entry.media_type).map((row) => row.library_id);
  const row: MockLibraryEntry = {
    media_type: entry.media_type,
    tmdb_id: entry.tmdb_id,
    library_id: Math.max(0, ...ids) + 1,
    added_at: new Date().toISOString(),
    files: false,
    unassigned: new Set(),
    filed: new Map(),
    streamed: new Map(),
    review: NO_REVIEW,
    settings: defaultSettings(entry),
  };
  library.set(libraryKey(entry.media_type, entry.tmdb_id), row);
  return row;
}

/** The catalog entry and library row for a library id (`/movies/{id}`, `/shows/{id}`). */
export function findByLibraryId(library: MockLibrary, mediaType: MediaType, id: number) {
  const row = [...library.values()].find((r) => r.media_type === mediaType && r.library_id === id);
  const entry = row && findTitle(row.media_type, row.tmdb_id);
  return row && entry ? { key: libraryKey(row.media_type, row.tmdb_id), row, entry } : undefined;
}

/** Library id of a seeded episode. */
export function episodeId(tmdbId: number, season: number, episode: number): number {
  return episodeIdFor(SHOWS[tmdbId].key, season, episode);
}

/** The show an episode id belongs to, when it exists. */
export function findEpisode(id: number): { tmdbId: number; season: number; episode: number } | undefined {
  const key = Math.floor(id / 100_000);
  const season = Math.floor((id % 100_000) / 1000);
  const episode = id % 1000;
  const found = Object.entries(SHOWS).find(([, show]) => show.key === key);
  if (!found) return undefined;
  const exists = found[1].seasons.some((s) => s.n === season && s.episodes.some((e) => e.n === episode));
  return exists ? { tmdbId: Number(found[0]), season, episode } : undefined;
}

// ---- Builders ---------------------------------------------------------------

/** A seed with a file: `watched` ones have one too. */
const seededFile = (state: EpisodeState) => state === 'watched' || state === 'ready' || state === 'downloading';
/** A built episode with a file, watched or not (PLAN §6: `pct`, `webdav_path` and `source` describe it). */
const hasFile = (episode: Pick<EpisodeSummary, 'webdav_path'>) => episode.webdav_path !== null;
const aired = (airDate: string | null) => airDate !== null && airDate <= TODAY;

/** What the library holds for an episode, plays aside. */
type FileState = Exclude<EpisodeState, 'watched'>;

/**
 * A file wins (seeded, or from an applied job); without one, an episode an open
 * mapping job wants to review is needs_review, otherwise missing or upcoming.
 * A file is `ready` here: buildSeasons makes it downloading while a download wants it.
 */
function fileState(seed: SeasonSeed['episodes'][number], id: number, row: MockLibraryEntry | undefined): FileState {
  const kept = row !== undefined && !row.unassigned.has(id);
  if (kept && row.files && seededFile(seed.state)) return 'ready';
  if (kept && (row.filed.has(id) || row.streamed.has(id))) return 'ready';
  if (row?.review.episode_ids.has(id)) return 'needs_review';
  if (seed.state === 'missing' || seed.state === 'upcoming') return seed.state;
  return aired(seed.air_date) ? 'missing' : 'upcoming';
}

/**
 * A seed's watch state, as far as Trakt's import has come (PLAN §6 "Watch
 * state rules"): null before its history; the rating with the ratings; a
 * show's progress and next episode with the progress step (next_episode is
 * then always there, maybe null). Plays and progress are the household's
 * (any member who watched it), the rating the viewer's (#239).
 */
function watchState(entry: CatalogEntry, seed: TitleSeed | ShowSeed): WatchState | null {
  const state = seed.watch_state;
  const imported = (step: TraktSyncStep) => traktPlayed(entry.media_type, entry.tmdb_id, step);
  if (!state || !imported('history')) return null;
  const { progress, ...plays } = state;
  const base: WatchState = { ...plays, rating: traktRated(entry.media_type, entry.tmdb_id) ? plays.rating : null };
  if (!('seasons' in seed)) return base;
  if (!imported('progress')) return { ...base, next_episode: null };
  return { ...base, ...(progress ? { progress } : {}), next_episode: seed.next_up ? nextEpisodeOf(seed, seed.next_up) : null };
}

function notInLibrary(): TitleLibraryState {
  return {
    in_library: false,
    library_id: null,
    added_at: null,
    availability_stage: 'none',
    webdav_path: null,
    webdav_url: null,
    download: null,
    needs_review: 0,
    review_job_id: null,
    episodes_total: null,
    episodes_ready: null,
    missing: null,
    torrent: null,
    stream: null,
    keep: null,
  };
}

interface BuiltSeason {
  seed: SeasonSeed;
  episodes: EpisodeSummary[];
}

/** Seasons as the backend sees them, with the library applied; season 0 only while specials are on. */
function buildSeasons(entry: CatalogEntry, show: ShowSeed, row: MockLibraryEntry | undefined): BuiltSeason[] {
  const includeSpecials = row?.settings.include_specials ?? true;
  const absolute = absoluteNumbers(show);
  const plays = traktPlayed('tv', entry.tmdb_id, 'history');
  const holds = row ? mockHolds() : [];
  return show.seasons
    .map((season) => {
      const episodes = season.episodes.map((seed): EpisodeSummary => {
        const id = episodeIdFor(show.key, season.n, seed.n);
        const found = fileState(seed, id, row);
        // A play wins the state (PLAN §6); pct, the path, source and stream still describe the file
        const stream = (found === 'ready' && row?.streamed.get(id)) || null;
        // #77: a file is downloading only while a download wants it (an HDRezka one never is)
        const hold = episodeHold(holds, entry.tmdb_id, season.n, seed.n);
        const facts = { season: season.n, episode: seed.n, has_file: found === 'ready', hdrezka: stream !== null, pct: seed.pct ?? null };
        const file: FileState = found === 'ready' && episodeWanted(hold, facts) ? 'downloading' : found;
        const filed = file === 'ready' || file === 'downloading';
        const keep = episodeKeep(hold, facts);
        return {
          id,
          season_number: season.n,
          episode_number: seed.n,
          absolute_number: absolute.get(id) ?? null,
          name: seed.name,
          overview: seed.overview ?? '',
          air_date: seed.air_date,
          still_path: null,
          runtime: seed.runtime,
          episode_type: seed.episode_type ?? 'standard',
          state: plays && hasPlay(show, season.n, seed) ? 'watched' : file,
          pct: file === 'downloading' ? (seed.pct ?? keep?.pct ?? 0) : null,
          webdav_path: filed ? episodePath(entry, season.n, seed.n, seed.name, stream ? '.mp4' : '.mkv') : null,
          source: stream ? 'hdrezka' : filed ? 'torrent' : null,
          stream,
          keep,
        };
      });
      return { seed: season, episodes };
    })
    .filter((season) => includeSpecials || season.seed.n !== 0);
}

/** Watched episodes count as ready when they have a file, and never as missing, under review or upcoming. */
function seasonSummary({ seed, episodes }: BuiltSeason, inLibrary: boolean): SeasonSummary {
  const count = (match: (episode: EpisodeSummary) => boolean) => (inLibrary ? episodes.filter(match).length : 0);
  const absolute = episodes.map((e) => e.absolute_number).filter((n): n is number => n !== null);
  return {
    season_number: seed.n,
    name: seed.name,
    episodes_total: episodes.length,
    ready: count(hasFile),
    missing: count((e) => e.state === 'missing'),
    needs_review: count((e) => e.state === 'needs_review'),
    upcoming: count((e) => e.state === 'upcoming'),
    absolute_from: inLibrary ? (absolute[0] ?? null) : null,
    absolute_to: inLibrary ? (absolute.at(-1) ?? null) : null,
  };
}

/** Story arcs when TMDB has them; otherwise split at breaks of more than 45 days (not specials), then into blocks of up to 10. */
function ranges({ seed, episodes }: BuiltSeason): EpisodeRange[] {
  if (episodes.length <= 20) return [];
  const spans: Array<Pick<EpisodeRange, 'label' | 'kind' | 'from' | 'to'>> =
    seed.arcs?.map((arc) => ({ ...arc, kind: 'arc' as const })) ??
    episodes
      .reduce<EpisodeSummary[][]>((runs, episode, i) => {
        const previous = episodes[i - 1]?.air_date;
        // Specials air years apart: number blocks only.
        const gap = seed.n > 0 && previous && episode.air_date && Date.parse(episode.air_date) - Date.parse(previous) > 45 * DAY;
        if (i === 0 || gap) runs.push([]);
        runs[runs.length - 1].push(episode);
        return runs;
      }, [])
      .flatMap((block) =>
        Array.from({ length: Math.ceil(block.length / 10) }, (_, i) => {
          const chunk = block.slice(i * 10, i * 10 + 10);
          const from = chunk[0].episode_number;
          const to = chunk[chunk.length - 1].episode_number;
          return { label: `${from}–${to}`, kind: 'block' as const, from, to };
        })
      );
  return spans.map((span) => {
    const inside = episodes.filter((e) => e.episode_number >= span.from && e.episode_number <= span.to);
    return {
      ...span,
      absolute_from: inside[0]?.absolute_number ?? null,
      absolute_to: inside.at(-1)?.absolute_number ?? null,
      missing: inside.filter((e) => e.state === 'missing').length,
    };
  });
}

/**
 * The first run of the frontier: aired regular episodes after the last one
 * with a file or watched. An episode under review has no file yet, so it
 * belongs to it.
 */
function frontierRun(seasons: BuiltSeason[]): MissingRun | null {
  const regular = seasons.filter((s) => s.seed.n > 0).flatMap((s) => s.episodes);
  const rest = regular.slice(regular.findLastIndex((e) => hasFile(e) || e.state === 'watched') + 1);
  const unfiled = (e: EpisodeSummary | undefined) => e !== undefined && (e.state === 'missing' || (e.state === 'needs_review' && aired(e.air_date)));
  const start = rest.findIndex(unfiled);
  if (start < 0) return null;
  let end = start;
  while (unfiled(rest[end + 1]) && rest[end + 1].season_number === rest[start].season_number) end++;
  return {
    season_number: rest[start].season_number,
    from: rest[start].episode_number,
    to: rest[end].episode_number,
    absolute_from: rest[start].absolute_number,
    absolute_to: rest[end].absolute_number,
    count: end - start + 1,
  };
}

function showLibrary(entry: CatalogEntry, show: ShowSeed, row: MockLibraryEntry | undefined, seasons: BuiltSeason[]): TitleLibraryState {
  if (!row) return notInLibrary();
  const all = seasons.flatMap((s) => s.episodes);
  const regular = all.filter((e) => e.season_number > 0);
  const withFile = regular.filter(hasFile);
  // pct is the file's: a watched episode can still be downloading
  const downloading = all.filter((e) => e.pct !== null);
  const missing = frontierRun(seasons);
  const path = all.some(hasFile) ? showPath(entry) : null;
  return {
    in_library: true,
    library_id: row.library_id,
    added_at: row.added_at,
    availability_stage: withFile.length === 0 || missing ? 'needs_torrent' : downloading.length ? 'downloading' : 'ready',
    webdav_path: path,
    webdav_url: webdavUrl(path),
    download: downloading.length
      ? {
          pct: Math.round(downloading.reduce((sum, e) => sum + (e.pct ?? 0), 0) / downloading.length),
          speed_bps: 0,
          eta_seconds: null,
          seeders: show.seeders ?? 0,
        }
      : null,
    needs_review: row.review.files,
    review_job_id: row.review.job_id,
    episodes_total: regular.filter((e) => e.state !== 'upcoming').length,
    episodes_ready: withFile.length,
    missing,
    torrent: null,
    stream: null,
    keep: keepFor(scopeHold('tv', entry.tmdb_id)),
  };
}

const keepFor = (hold: LiveHold | undefined) => (hold ? keepOf(hold.download) : null);

function movieLibrary(entry: CatalogEntry, movie: MovieSeed, row: MockLibraryEntry | undefined): TitleLibraryState {
  if (!row) return notInLibrary();
  // A stream replaces the torrent, and is never downloading: it plays from the CDN
  const stream = row.stream ?? null;
  const torrent = stream ? null : (row.assigned ?? (row.files ? (movie.torrent ?? null) : null));
  // #77: downloading only while a download wants the file; a file nothing keeps streams on demand, so it's ready
  // as soon as it's assigned (one assigned this session too). The seed's progress is its own torrent's.
  const hold = scopeHold('movie', entry.tmdb_id);
  const wanted = torrent !== null && wantsBytes(hold?.admitted);
  const seeded: DownloadProgress | undefined = row.assigned ? undefined : movie.download;
  const download: DownloadProgress | null = wanted
    ? (seeded ?? { pct: keepFor(hold)?.pct ?? 0, speed_bps: 0, eta_seconds: null, seeders: torrent.seeders ?? 0 })
    : null;
  const path = stream ? moviePath(entry, 'stream.mp4') : torrent ? moviePath(entry, torrent.file_name) : null;
  return {
    ...notInLibrary(),
    in_library: true,
    library_id: row.library_id,
    added_at: row.added_at,
    needs_review: row.review.files,
    review_job_id: row.review.job_id,
    availability_stage: stream ? 'ready' : !torrent ? 'needs_torrent' : download ? 'downloading' : 'ready',
    webdav_path: path,
    webdav_url: webdavUrl(path),
    download,
    torrent,
    stream,
    keep: keepFor(hold),
  };
}

/** A movie gets a file (an applied mapping job): loaded, nothing fetched yet. */
export function assignMovieRelease(
  row: MockLibraryEntry,
  release: Pick<TitleTorrent, 'info_hash' | 'release_name' | 'file_name' | 'size_bytes'>
): TitleTorrent {
  const name = release.release_name;
  const torrent: TitleTorrent = {
    ...release,
    resolution: /2160p/i.test(name) ? '2160p' : /1080p/i.test(name) ? '1080p' : null,
    source: /bluray/i.test(name) ? 'BluRay' : /web/i.test(name) ? 'WEB-DL' : null,
    added_at: new Date().toISOString(),
    state: 'active',
    seeders: 12,
  };
  row.assigned = torrent;
  row.stream = undefined;
  row.files = true;
  onMovieFile(row.tmdb_id, torrent);
  return torrent;
}

/** A movie plays from HDRezka (#63): the stream replaces its torrent. */
export function assignMovieStream(row: MockLibraryEntry, stream: Omit<TitleStream, 'added_at' | 'provider'>): TitleStream {
  row.stream = { provider: 'hdrezka', ...stream, added_at: new Date().toISOString() };
  row.assigned = undefined;
  row.files = true;
  onMovieFile(row.tmdb_id, 'stream');
  return row.stream;
}

/** Episodes play from HDRezka (#63): each replaces the episode's torrent file. */
export function assignEpisodeStream(row: MockLibraryEntry, episodeId: number, stream: EpisodeStream): void {
  const previous = episodeFileKind(row, episodeId);
  row.streamed.set(episodeId, stream);
  row.filed.delete(episodeId);
  row.unassigned.delete(episodeId);
  onEpisodeFile(episodeId, 'stream', previous);
}

/** The file an episode has now, for the downloads hooks: HDRezka, a torrent's (estimated by runtime), or none. */
export function episodeFileKind(row: MockLibraryEntry, episodeId: number): EpisodeFileKind {
  if (row.streamed.has(episodeId)) return 'stream';
  const episode = findEpisode(episodeId);
  const seed = episode && seedEpisode(SHOWS[episode.tmdbId], [episode.season, episode.episode]);
  if (!seed || fileState(seed, episodeId, row) !== 'ready') return null;
  const filed = row.filed.get(episodeId);
  return { bytes: seed.runtime * EPISODE_BYTES_PER_MINUTE, file: filed && `${filed.info_hash}/${filed.file_path}` };
}

function libraryState(entry: CatalogEntry, library: MockLibrary): TitleLibraryState {
  const row = library.get(libraryKey(entry.media_type, entry.tmdb_id));
  if (entry.media_type === 'movie') return movieLibrary(entry, MOVIES[entry.tmdb_id], row);
  const show = SHOWS[entry.tmdb_id];
  return showLibrary(entry, show, row, buildSeasons(entry, show, row));
}

function similarTitles(seed: TitleSeed, library: MockLibrary): SimilarTitle[] {
  return seed.similar.flatMap(([mediaType, tmdbId]) => {
    const entry = findTitle(mediaType, tmdbId);
    if (!entry) return [];
    const state = libraryState(entry, library);
    return [
      {
        media_type: mediaType,
        tmdb_id: tmdbId,
        title: entry.title,
        year: entry.year,
        poster_path: null,
        library:
          state.library_id === null
            ? null
            : {
                library_id: state.library_id,
                availability_stage: state.availability_stage,
                episodes_total: state.episodes_total,
                episodes_ready: state.episodes_ready,
              },
      },
    ];
  });
}

function base(entry: CatalogEntry, seed: TitleSeed, library: MockLibrary) {
  return {
    tmdb_id: entry.tmdb_id,
    title: entry.title,
    original_title: entry.original_title,
    year: entry.year,
    overview: entry.overview,
    poster_path: null,
    backdrop_path: null,
    backdrop_dominant_color: seed.color,
    genres: entry.genres,
    vote_average: seed.vote_average,
    vote_count: seed.vote_count,
    original_language: seed.original_language,
    cast: seed.cast,
    trailer_youtube_key: seed.trailer_youtube_key,
    similar: similarTitles(seed, library),
    library: libraryState(entry, library),
    watch_state: watchState(entry, seed),
    recommendation_reason: recommendationReason(entry.media_type, entry.tmdb_id),
  };
}

/** A seeded episode as `watch_state.next_episode` names it; null when the show doesn't list it. */
export function nextEpisodeOf(show: ShowSeed, key: EpisodeKey): NextEpisode | null {
  const seed = seedEpisode(show, key);
  if (!seed) return null;
  return {
    season: key[0],
    episode: key[1],
    absolute_number: absoluteNumbers(show).get(episodeIdFor(show.key, ...key)) ?? null,
    name: seed.name,
    air_date: seed.air_date,
  };
}

/** GET /titles/{movie|tv}/{tmdb_id} for a catalog title, against the page's library. */
export function buildTitle(mediaType: MediaType, tmdbId: number, library: MockLibrary = seedLibrary()): Title | undefined {
  const entry = findTitle(mediaType, tmdbId);
  if (!entry) return undefined;

  if (entry.media_type === 'movie') {
    const movie = MOVIES[tmdbId];
    if (!movie) return undefined;
    const title: MovieTitle = { ...base(entry, movie, library), media_type: 'movie', runtime: entry.runtime ?? null, directors: movie.crew };
    return title;
  }

  const show = SHOWS[tmdbId];
  if (!show) return undefined;
  const row = library.get(libraryKey('tv', tmdbId));
  const seasons = buildSeasons(entry, show, row);
  const settings = row?.settings ?? defaultSettings(entry);
  const regular = seasons.filter((s) => s.seed.n > 0);
  const specials = seasons.filter((s) => s.seed.n === 0);
  const withFile = regular.filter((s) => s.episodes.some(hasFile));
  const titleBase = base(entry, show, library);
  // PLAN §6: Trakt's next episode's season when the title lists it, else the frontier's, …
  const next = titleBase.watch_state?.next_episode;
  const nextSeason = next && seasons.some((s) => s.seed.n === next.season) ? next.season : null;
  const title: ShowTitle = {
    ...titleBase,
    media_type: 'tv',
    status: show.status,
    is_anime: Boolean(entry.is_anime),
    ...settings,
    creators: show.crew,
    networks: show.networks,
    end_year: show.end_year,
    episode_run_time: show.episode_run_time,
    seasons: [...regular, ...specials].map((season) => seasonSummary(season, row !== undefined)),
    default_season:
      nextSeason ?? titleBase.library.missing?.season_number ?? withFile.at(-1)?.seed.n ?? regular[0]?.seed.n ?? specials[0]?.seed.n ?? null,
  };
  return title;
}

/** GET /titles/tv/{tmdb_id}/seasons/{n}. Undefined for an unknown (or hidden) season. */
export function buildSeason(tmdbId: number, seasonNumber: number, library: MockLibrary = seedLibrary()): SeasonDetail | undefined {
  const entry = findTitle('tv', tmdbId);
  const show = SHOWS[tmdbId];
  if (!entry || !show) return undefined;
  const row = library.get(libraryKey('tv', tmdbId));
  const season = buildSeasons(entry, show, row).find((s) => s.seed.n === seasonNumber);
  return (
    season && {
      season_number: season.seed.n,
      name: season.seed.name,
      episodes: season.episodes,
      ranges: ranges(season),
      keep: row ? keepFor(scopeHold('tv', tmdbId, seasonNumber)) : null,
    }
  );
}

/**
 * The release a movie's or episode's file came from, for ranking subtitles:
 * null while it has no file, undefined when the library doesn't have the item.
 */
export function itemRelease(library: MockLibrary, type: SubtitleItemType, id: number): string | null | undefined {
  if (type === 'movie') {
    const found = findByLibraryId(library, 'movie', id);
    if (!found) return undefined;
    // An HDRezka item has no release name until #68
    if (found.row.stream) return null;
    return found.row.files ? (found.row.assigned?.release_name ?? MOVIES[found.entry.tmdb_id]?.torrent?.release_name ?? null) : null;
  }
  const episode = findEpisode(id);
  const row = episode && library.get(libraryKey('tv', episode.tmdbId));
  if (!row) return undefined;
  if (row.streamed.has(id)) return null;
  return episodeFile(library, id)?.release_name ?? null;
}

/** An episode's current file (seeded, or from an applied job); null without one. */
export function episodeFile(library: MockLibrary, id: number): EpisodeFile | null {
  const episode = findEpisode(id);
  const row = episode && library.get(libraryKey('tv', episode.tmdbId));
  if (!episode || !row) return null;
  const built = buildSeason(episode.tmdbId, episode.season, library)?.episodes.find((e) => e.id === id);
  if (!built || !hasFile(built)) return null;
  // PLAN §6 "HDRezka": what a mapping proposal would replace
  const stream = row.streamed.get(id);
  if (stream) return { info_hash: '', release_name: `HDRezka · ${stream.translator} · ${stream.quality}`, file_path: '' };
  const filed = row.filed.get(id);
  if (filed) return filed;
  // A seeded file: named after the show's release, e.g. Doctor.Who.2005.S00E06.1080p.BluRay.x264-SHORTBREHD.mkv
  const release = SHOWS[episode.tmdbId].release ?? 'Unknown.Release';
  const code = `S${pad(episode.season)}E${pad(episode.episode)}`;
  const name = /S\d\d(-S\d\d)?/.test(release) ? release.replace(/S\d\d(-S\d\d)?/, code) : `${release} - ${code}`;
  return { info_hash: `seed-${SHOWS[episode.tmdbId].key}`, release_name: release, file_path: `${release}/${name}.mkv` };
}

/** GET /shows/{id}/episodes: every season in number order, Specials whenever the show has season 0 rows (whatever include_specials says). */
export function showEpisodes(tmdbId: number, library: MockLibrary): ShowEpisodesSeason[] {
  const entry = findTitle('tv', tmdbId);
  const show = SHOWS[tmdbId];
  const row = library.get(libraryKey('tv', tmdbId));
  if (!entry || !show) return [];
  const withSpecials = row && { ...row, settings: { ...row.settings, include_specials: true } };
  return buildSeasons(entry, show, withSpecials)
    .sort((a, b) => a.seed.n - b.seed.n)
    .map(({ seed, episodes }) => ({
      season_number: seed.n,
      name: seed.name,
      episodes: episodes.map((e) => ({
        id: e.id,
        episode_number: e.episode_number,
        absolute_number: e.absolute_number,
        name: e.name,
        air_date: e.air_date,
        has_file: hasFile(e),
      })),
    }));
}

/** `LibraryItem.episodes_watched`: the show's regular episodes with a play (0 without Trakt). */
export function episodesWatched(tmdbId: number, library: MockLibrary): number {
  const entry = findTitle('tv', tmdbId);
  const show = SHOWS[tmdbId];
  if (!entry || !show) return 0;
  return buildSeasons(entry, show, library.get(libraryKey('tv', tmdbId)))
    .filter((season) => season.seed.n > 0)
    .reduce((sum, season) => sum + season.episodes.filter((e) => e.state === 'watched').length, 0);
}
