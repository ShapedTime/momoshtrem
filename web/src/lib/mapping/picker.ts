import type { Proposal, ProposalTarget, ShowEpisode, ShowEpisodesSeason } from '@/lib/api/contract';
import { shortName } from './review';

/**
 * The episode picker's catalog (GET /shows/{id}/episodes) as rows: sticky
 * season headers and episodes with their tags, filtered by the search and
 * "Hide taken". Pure, so the pane and the phone sheet share it.
 */

export interface PickerEpisode {
  id: number;
  season: number;
  episode: number;
  absolute: number | null;
  name: string;
  /** Already has a file ("In library"). */
  hasFile: boolean;
}

export type PickerTag = { kind: 'current' } | { kind: 'taken'; by: string } | { kind: 'library' } | null;

export type PickerRow =
  | { type: 'header'; season: number; label: string; count: number }
  | { type: 'episode'; episode: PickerEpisode; tag: PickerTag };

const pad = (n: number) => String(n).padStart(2, '0');

/** Above this many episodes the list is virtualised (as on the show page). */
export const PICKER_VIRTUALIZE_ABOVE = 60;

const GENERIC = /^(season|series)\s+\d+$/i;

/** The season jump's label: "Specials", "Season 1", "S21 Wano". */
export function seasonShortLabel(season: Pick<ShowEpisodesSeason, 'season_number' | 'name'>): string {
  if (season.season_number === 0) return 'Specials';
  if (!season.name || GENERIC.test(season.name)) return season.name || `Season ${season.season_number}`;
  return `S${season.season_number} ${season.name.split(/\s+/)[0]}`;
}

/** The sticky header: "Specials · Season 0", "Season 1", "Season 21 · Wano Country". */
export function seasonHeader(season: Pick<ShowEpisodesSeason, 'season_number' | 'name'>): string {
  if (season.season_number === 0) return 'Specials · Season 0';
  if (!season.name || GENERIC.test(season.name)) return season.name || `Season ${season.season_number}`;
  return `Season ${season.season_number} · ${season.name}`;
}

/** The number column: "E35", or "SP03" for specials. */
export const episodeNumberLabel = (ep: Pick<PickerEpisode, 'season' | 'episode'>): string =>
  `${ep.season === 0 ? 'SP' : 'E'}${pad(ep.episode)}`;

export function toPickerEpisode(season: number, ep: ShowEpisode): PickerEpisode {
  return { id: ep.id, season, episode: ep.episode_number, absolute: ep.absolute_number, name: ep.name, hasFile: ep.has_file };
}

/** A picked episode as a proposal target (optimistic display). */
export const toTarget = (ep: PickerEpisode): ProposalTarget => ({
  episode_id: ep.id,
  season: ep.season,
  episode: ep.episode,
  absolute_number: ep.absolute,
  name: ep.name,
});

/**
 * Search: `35` or `#35` (episode or absolute number; absolute numbers by
 * prefix too, so `107` finds #1070–#1079), `e35`, `s00e3` / `S01E35`, `s2`
 * (a whole season), `sp`/`special…` (Season 0, `sp3` one special), or a
 * case-insensitive part of the title.
 */
export function matchesQuery(ep: PickerEpisode, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  const compact = q.replace(/\s+/g, '');
  const number = /^#?(\d+)$/.exec(compact);
  if (number) {
    const value = Number(number[1]);
    return ep.episode === value || ep.absolute === value || (ep.absolute !== null && String(ep.absolute).startsWith(number[1]));
  }
  const inSeason = /^e(\d+)$/.exec(compact);
  if (inSeason) return ep.episode === Number(inSeason[1]);
  const code = /^s(\d+)(?:e(\d*))?$/.exec(compact);
  if (code) return ep.season === Number(code[1]) && (!code[2] || ep.episode === Number(code[2]));
  const special = /^sp(?:ecials?)?#?(\d+)?$/.exec(compact);
  if (special && ep.season === 0) return !special[1] || ep.episode === Number(special[1]);
  return ep.name.toLowerCase().includes(q);
}

/** Episodes other non-ignored files in this job already use, with that file's short name. */
export function takenBy(proposals: Proposal[], selectedId: number | null, showTitle: string): Map<number, string> {
  const taken = new Map<number, string>();
  for (const p of proposals) {
    if (p.id === selectedId || p.decision === 'ignored') continue;
    for (const t of p.targets) if (!taken.has(t.episode_id)) taken.set(t.episode_id, shortName(p.file_path, showTitle));
  }
  return taken;
}

export interface PickerFilter {
  query: string;
  /** Hide episodes another file takes or that already have one; Current always stays. */
  hideTaken: boolean;
  current: ReadonlySet<number>;
  taken: ReadonlyMap<number, string>;
}

export function tagFor(ep: PickerEpisode, filter: Pick<PickerFilter, 'current' | 'taken'>): PickerTag {
  if (filter.current.has(ep.id)) return { kind: 'current' };
  const by = filter.taken.get(ep.id);
  if (by !== undefined) return { kind: 'taken', by };
  return ep.hasFile ? { kind: 'library' } : null;
}

/** Headers and episodes in catalog order (Specials first, as the API lists them); empty seasons drop out. */
export function pickerRows(seasons: ShowEpisodesSeason[], filter: PickerFilter): PickerRow[] {
  const rows: PickerRow[] = [];
  for (const season of seasons) {
    const episodes = season.episodes
      .map((ep) => toPickerEpisode(season.season_number, ep))
      .map((episode) => ({ type: 'episode' as const, episode, tag: tagFor(episode, filter) }))
      .filter(({ episode, tag }) => matchesQuery(episode, filter.query) && (!filter.hideTaken || tag === null || tag.kind === 'current'));
    if (episodes.length === 0) continue;
    rows.push({ type: 'header', season: season.season_number, label: seasonHeader(season), count: season.episodes.length }, ...episodes);
  }
  return rows;
}

/** The season the picker opens on: the file's current one, else the first regular season. */
export function initialSeason(seasons: ShowEpisodesSeason[], proposal: Pick<Proposal, 'targets'> | null): number | null {
  const current = proposal?.targets[0]?.season;
  if (current !== undefined && seasons.some((s) => s.season_number === current)) return current;
  return (seasons.find((s) => s.season_number > 0) ?? seasons[0])?.season_number ?? null;
}

export const episodeCount = (seasons: ShowEpisodesSeason[]): number => seasons.reduce((sum, s) => sum + s.episodes.length, 0);
