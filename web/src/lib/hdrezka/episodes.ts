import type {
  HDRezkaEpisode,
  HDRezkaLibraryEpisode,
  HDRezkaSeason,
  StreamEpisodeFailure,
  StreamEpisodeTarget,
} from '@/lib/api/contract';
import { episodeCode } from '@/lib/title/episodes';

/**
 * A show's episodes on HDRezka (#67): which library episode each one fills,
 * what to select first, and assigning them in chunks. HDRezka numbers its
 * own seasons and episodes; `library` is the episode it maps to, or null.
 */

/** The backend takes at most 30 episodes a request (PLAN §6 "HDRezka"). */
export const EPISODES_PER_REQUEST = 30;

/** An HDRezka episode in selection and session state: "2:5". */
export const episodeKey = (season: number, episode: number) => `${season}:${episode}`;

const today = () => new Date().toISOString().slice(0, 10);

export type EpisodeRowState = 'ready' | 'missing' | 'upcoming';

/** The row's pill, from the library episode: it has a file, it aired without one, or it hasn't aired. */
export function episodeRowState(library: HDRezkaLibraryEpisode, now: string = today()): EpisodeRowState {
  if (library.has_file) return 'ready';
  return library.air_date !== null && library.air_date <= now ? 'missing' : 'upcoming';
}

/** Where the episode's file comes from now: "Torrent", "HDRezka · LostFilm", or null without one. */
export function episodeSourceLabel(library: HDRezkaLibraryEpisode): string | null {
  if (!library.has_file) return null;
  if (library.source === 'hdrezka') return library.stream ? `HDRezka · ${library.stream.translator}` : 'HDRezka';
  return 'Torrent';
}

/** "S01E05", the library episode an HDRezka one fills. */
export const libraryCode = (library: HDRezkaLibraryEpisode) => episodeCode(library.season, library.episode);

const mapped = (season: HDRezkaSeason) =>
  season.episodes.filter((e): e is HDRezkaEpisode & { library: HDRezkaLibraryEpisode } => e.library !== null);

/** "Select missing": aired episodes of the season without a file. */
export function missingKeys(season: HDRezkaSeason, now: string = today()): string[] {
  return mapped(season)
    .filter((e) => episodeRowState(e.library, now) === 'missing')
    .map((e) => episodeKey(season.number, e.number));
}

/** "Select all": every episode of the season that maps to the library. */
export const allKeys = (season: HDRezkaSeason): string[] => mapped(season).map((e) => episodeKey(season.number, e.number));

/** What the sheet was opened for: a library season, and maybe one of its episodes. */
export interface EpisodeScope {
  season: number | null;
  episode: number | null;
}

/**
 * The season tab to open: the HDRezka season holding the scope's library
 * season (or `fallback`, the show's default season), else the same number,
 * else the first.
 */
export function defaultSeasonNumber(seasons: HDRezkaSeason[], scope: EpisodeScope, fallback: number | null = null): number | null {
  const wanted = scope.season ?? fallback;
  if (wanted !== null) {
    const holding = seasons.find((s) => s.episodes.some((e) => e.library?.season === wanted));
    if (holding) return holding.number;
    if (seasons.some((s) => s.number === wanted)) return wanted;
  }
  return seasons[0]?.number ?? null;
}

/** What starts selected: the scope's episode when there is one, else "Select missing" in the open season. */
export function defaultSelection(seasons: HDRezkaSeason[], seasonNumber: number | null, scope: EpisodeScope, now: string = today()): string[] {
  if (scope.season !== null && scope.episode !== null) {
    for (const season of seasons) {
      const hit = season.episodes.find((e) => e.library?.season === scope.season && e.library.episode === scope.episode);
      if (hit) return [episodeKey(season.number, hit.number)];
    }
  }
  const season = seasons.find((s) => s.number === seasonNumber);
  return season ? missingKeys(season, now) : [];
}

/** The selected episodes that map to the library, in season order, as the assign request wants them. */
export function episodeTargets(seasons: HDRezkaSeason[], keys: string[]): Array<StreamEpisodeTarget & { library: HDRezkaLibraryEpisode }> {
  const wanted = new Set(keys);
  return seasons.flatMap((season) =>
    mapped(season)
      .filter((e) => wanted.has(episodeKey(season.number, e.number)))
      .map((e) => ({ episode_id: e.library.episode_id, season: season.number, episode: e.number, library: e.library }))
  );
}

/** The episode whose qualities stand for the season's: the scope's, else the first selected, else the open season's first. */
export function sampleEpisode(seasons: HDRezkaSeason[], seasonNumber: number | null, keys: string[]): { season: number; episode: number } | null {
  const [first] = episodeTargets(seasons, keys);
  if (first) return { season: first.season, episode: first.episode };
  const season = seasons.find((s) => s.number === seasonNumber) ?? seasons[0];
  const episode = season?.episodes[0];
  return season && episode ? { season: season.number, episode: episode.number } : null;
}

export function chunk<T>(items: T[], size: number = EPISODES_PER_REQUEST): T[][] {
  return Array.from({ length: Math.ceil(items.length / size) }, (_, i) => items.slice(i * size, i * size + size));
}

export interface ChunkedResult {
  assigned: number;
  failed: StreamEpisodeFailure[];
}

/**
 * Assign episodes a chunk at a time, one request after another (each within
 * the backend's 25 s budget). A chunk that fails as a whole fails each of
 * its episodes with that error; the next chunk still goes.
 */
export async function assignInChunks(
  targets: StreamEpisodeTarget[],
  assign: (items: StreamEpisodeTarget[]) => Promise<{ assigned: unknown[]; failed: StreamEpisodeFailure[] }>,
  onProgress: (done: number, total: number) => void = () => {}
): Promise<ChunkedResult> {
  const result: ChunkedResult = { assigned: 0, failed: [] };
  let done = 0;
  onProgress(0, targets.length);
  for (const items of chunk(targets)) {
    try {
      const answer = await assign(items);
      result.assigned += answer.assigned.length;
      result.failed.push(...answer.failed);
    } catch (error) {
      const code = typeof error === 'object' && error !== null && 'code' in error && typeof error.code === 'string' ? error.code : 'internal_error';
      const message = error instanceof Error ? error.message : 'The request failed.';
      result.failed.push(...items.map((item) => ({ episode_id: item.episode_id, code, message })));
    }
    done += items.length;
    onProgress(done, targets.length);
  }
  return result;
}
