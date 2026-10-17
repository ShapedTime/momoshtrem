import type { Download, DownloadTarget, EpisodeSummary, Keep, SeasonDetail, Title } from '@/lib/api/contract';
import { episodeCode, parseEpisodeCode } from './episodes';
import { keepExpiryLabel, keepOf } from './keep';

/**
 * The Keep sheet (`?sheet=keep`, #206): what it's on, from its URL params, and
 * the library target and download that go with it. Pure. The copy for a
 * download's state lives in `keep.ts`, shared with Activity.
 *
 * - `?sheet=keep&movie=<tmdb>`: a movie;
 * - `?sheet=keep&show=<tmdb>&scope=show`: the whole show (also without `scope`);
 * - `?sheet=keep&show=<tmdb>&scope=season&season=<n>`: a season (`season` is also the page's tab, so it stays);
 * - `?sheet=keep&show=<tmdb>&scope=episode&se=S02E03`: an episode.
 *
 * `scope` decides, because the episode sheet's `se` stays in the URL under a sheet it opens.
 */
export type KeepContext =
  | { kind: 'movie'; tmdbId: number }
  | { kind: 'show'; tmdbId: number }
  | { kind: 'season'; tmdbId: number; season: number }
  | { kind: 'episode'; tmdbId: number; season: number; episode: number };

/** The params the sheet removes when it closes from a deep link (`season` is the show page's tab). */
export const KEEP_PARAMS = ['movie', 'show', 'scope', 'se'];

const tmdbParam = (value: string | null) => {
  const n = Number(value);
  return value && Number.isInteger(n) && n > 0 ? n : null;
};

export function keepContext(params: URLSearchParams): KeepContext | null {
  const movie = tmdbParam(params.get('movie'));
  if (movie !== null) return { kind: 'movie', tmdbId: movie };
  const show = tmdbParam(params.get('show'));
  if (show === null) return null;
  const scope = params.get('scope');
  if (scope === 'episode') {
    const code = parseEpisodeCode(params.get('se'));
    return code ? { kind: 'episode', tmdbId: show, season: code.season, episode: code.episode } : null;
  }
  if (scope !== 'season') return { kind: 'show', tmdbId: show };
  const season = params.get('season');
  return season !== null && /^\d+$/.test(season) ? { kind: 'season', tmdbId: show, season: Number(season) } : null;
}

/** `?sheet=keep&…` params that open the sheet on `context`. */
export function keepSheetParams(context: KeepContext): Record<string, string> {
  switch (context.kind) {
    case 'movie':
      return { movie: String(context.tmdbId) };
    case 'show':
      return { show: String(context.tmdbId), scope: 'show' };
    case 'season':
      return { show: String(context.tmdbId), scope: 'season', season: String(context.season) };
    case 'episode':
      return { show: String(context.tmdbId), scope: 'episode', se: episodeCode(context.season, context.episode) };
  }
}

/** The Keep sheet for what holds an episode: its own download (or a new one), else its season's or its show's. */
export function episodeKeepContext(tmdbId: number, episode: Pick<EpisodeSummary, 'season_number' | 'episode_number' | 'keep'>): KeepContext {
  const keep = keepOf(episode.keep);
  if (keep && keep.scope !== 'episode') return heldByContext(tmdbId, keep, episode.season_number);
  return { kind: 'episode', tmdbId, season: episode.season_number, episode: episode.episode_number };
}

/** The season the sheet reads its episodes or season keep from. */
export const keepSeasonNumber = (context: KeepContext): number | null =>
  context.kind === 'season' || context.kind === 'episode' ? context.season : null;

/** "Interstellar", "Severance", "Severance · Season 2" ("Specials"), "Severance · S02E03". */
export function keepTargetName(context: KeepContext, titleName: string): string {
  if (context.kind === 'season') return `${titleName} · ${context.season === 0 ? 'Specials' : `Season ${context.season}`}`;
  if (context.kind === 'episode') return `${titleName} · ${episodeCode(context.season, context.episode)}`;
  return titleName;
}

/** What the sheet keeps, and how it stands now. */
export interface KeepSubject {
  target: DownloadTarget;
  /** "Severance · S02E03": toasts and confirmations. */
  name: string;
  /** Its own download (same scope): a Keep, or for an episode a smart pick. Null: nothing yet. */
  own: Keep | null;
  /** An episode that a season or show keep holds already (it isn't its own). */
  heldBy: Keep | null;
}

export type KeepSubjectResult =
  | { status: 'ready'; subject: KeepSubject }
  /** Not in the library, or an episode the season doesn't list. */
  | { status: 'missing' }
  /** The backend doesn't send `keep` (before #202 and #204): it can't keep anything yet. */
  | { status: 'unsupported' };

/**
 * Whether the backend does downloads (#77), read from where a page gets its keep: a title's
 * library state, a season or an episode. `keep: null` is supported and not kept; a backend from
 * before #202 and #204 leaves the field out, and then every Keep entry point hides (its POST
 * would 404).
 */
export const downloadsAvailable = (holder: { keep?: Keep | null } | null | undefined): boolean => holder?.keep !== undefined;

/** The library target for `context`, and its download, from the title and (season and episode scopes) the season. */
export function keepSubject(context: KeepContext, title: Title, season: SeasonDetail | undefined): KeepSubjectResult {
  const libraryId = title.library.library_id;
  if (!title.library.in_library || libraryId === null) return { status: 'missing' };
  const name = keepTargetName(context, title.title);
  switch (context.kind) {
    case 'movie':
    case 'show':
      if (!downloadsAvailable(title.library)) return { status: 'unsupported' };
      return { status: 'ready', subject: { target: { scope: context.kind, id: libraryId }, name, own: keepOf(title.library.keep), heldBy: null } };
    case 'season':
      if (season && !downloadsAvailable(season)) return { status: 'unsupported' };
      return {
        status: 'ready',
        subject: { target: { scope: 'season', id: libraryId, season_number: context.season }, name, own: keepOf(season?.keep), heldBy: null },
      };
    case 'episode': {
      const episode: EpisodeSummary | undefined = season?.episodes.find((e) => e.episode_number === context.episode);
      if (!episode) return { status: 'missing' };
      if (!downloadsAvailable(episode)) return { status: 'unsupported' };
      const keep = keepOf(episode.keep);
      const own = keep?.scope === 'episode' ? keep : null;
      return { status: 'ready', subject: { target: { scope: 'episode', id: episode.id }, name, own, heldBy: own ? null : keep } };
    }
  }
}

/**
 * What the sheet does: a new Keep, keeping a smart pick, changing a kept one, or (an episode a
 * season or show keep holds) pointing to that keep.
 */
export type KeepSheetMode = 'new' | 'smart' | 'manage' | 'held';

export function keepSheetMode(subject: Pick<KeepSubject, 'own' | 'heldBy'> | null): KeepSheetMode {
  const own = subject?.own ?? null;
  if (own) return own.reason === 'smart' ? 'smart' : 'manage';
  return subject?.heldBy ? 'held' : 'new';
}

/** The Keep sheet for the season or show keep that holds an episode. */
export function heldByContext(tmdbId: number, heldBy: Pick<Keep, 'scope'>, seasonNumber: number): KeepContext {
  return heldBy.scope === 'show' ? { kind: 'show', tmdbId } : { kind: 'season', tmdbId, season: seasonNumber };
}

/** "Change season 3’s keep", "Change the Specials keep", "Change the show’s keep": what opens the keep that holds an episode. */
export function heldByAction(heldBy: Pick<Keep, 'scope'>, seasonNumber: number): string {
  if (heldBy.scope === 'show') return 'Change the show’s keep';
  return seasonNumber === 0 ? 'Change the Specials keep' : `Change season ${seasonNumber}’s keep`;
}

/**
 * "Season 2 is kept until watched · by Nov 16, so this episode is too." for an episode a season or
 * show keep holds (smart picks are episodes). Null while the episode plays from HDRezka: the keep
 * can't hold it (`unavailable`), and NOT_DOWNLOADABLE_COPY says so instead.
 */
export function heldByCopy(heldBy: Keep, seasonNumber: number): string | null {
  if (heldBy.state === 'unavailable') return null;
  const what = heldBy.scope === 'show' ? 'The whole show is' : seasonNumber === 0 ? 'Specials are' : `Season ${seasonNumber} is`;
  // "Kept until watched · by Nov 16" → "kept until watched · by Nov 16"
  const how = keepExpiryLabel(heldBy).replace(/^Kept/, 'kept');
  return `${what} ${how}, so this episode is too.`;
}

// ---- Toasts (Stop keeping's are `stopKeepingCopy` in keep.ts, shared with Activity) ---

/** After a Keep: what it is now ("Kept until watched · by Nov 16"), and whether it downloads. */
export function keptToast(download: Pick<Download, 'reason' | 'state' | 'until_watched' | 'expires_at'>, name: string, now: Date = new Date()) {
  const next =
    download.state === 'done'
      ? 'It’s already on disk.'
      : download.state === 'no_file'
        ? 'It downloads once there’s a file.'
        : download.state === 'unavailable'
          ? 'It plays from HDRezka, so nothing downloads.'
          : 'It downloads in the background.';
  return { title: `Keeping ${name} on disk`, body: `${keepExpiryLabel(download, now)}. ${next}` };
}

export const changedToast = (download: Pick<Download, 'reason' | 'until_watched' | 'expires_at'>, name: string, now: Date = new Date()) => ({
  title: `Changed how long ${name} is kept`,
  body: keepExpiryLabel(download, now),
});
