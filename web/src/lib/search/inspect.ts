import { isApiError } from '@/lib/api/client';
import { type AssignmentJob, type InspectRequest, isJobWorking, type JobStatus, type Proposal, type Release, type Title } from '@/lib/api/contract';
import { groupProposals } from '@/lib/mapping/review';
import { contextMediaType, type SourceContext } from '@/lib/source/context';
import { formatBytes, plural } from '@/lib/title/format';
import type { SearchProblem } from './copy';

/**
 * Inspect (#93, PLAN §6 "Search"): a dry-run mapping job for a release, shown
 * in a nested sheet before anything downloads. Dry runs are read-only and,
 * for a title outside the library, their targets carry TMDB episode ids, so
 * everything here reads the proposals' own `targets` and never the library.
 */

/** POST /search/torrents/inspect's body: the release, and the episode (or season) the search is for as the hint. */
export function inspectRequest(context: SourceContext, release: Pick<Release, 'id'>): InspectRequest {
  const request: InspectRequest = { media_type: contextMediaType(context), tmdb_id: context.tmdbId, release_id: release.id };
  if (context.kind === 'movie' || context.season === null) return request;
  return { ...request, hint: context.episode === null ? { season: context.season } : { season: context.season, episode: context.episode } };
}

/** "16 files · 13.2 GB" once the file list is in. */
export function inspectTitle(job: Pick<AssignmentJob, 'torrent'> | undefined): string {
  const files = job?.torrent.file_count ?? null;
  const size = job?.torrent.size_bytes ?? null;
  return files !== null && size !== null ? `${plural(files, 'file')} · ${formatBytes(size)}` : 'Reading the torrent…';
}

/** The primary action: what the card's Add does, said for the title. */
export function inspectAddLabel(title: Pick<Title, 'media_type' | 'library'>): string {
  if (title.media_type === 'tv') return 'Add & review mapping';
  return title.library.in_library ? 'Add & download' : 'Add to library & download';
}

// ---- Steps ------------------------------------------------------------------------------------

export interface InspectStep {
  label: string;
  state: 'done' | 'active' | 'pending';
}

const WORKING_STEP: Partial<Record<JobStatus, number>> = { queued: 0, fetching_metadata: 0, classifying: 1, resolving: 2 };
/** Past the last step: the job stopped working. */
const DONE = 3;

/**
 * "Fetched file list → Matched files → Asked AI about 3 tricky ones", each
 * said as it happens while it runs. The AI step shows only when the AI ran.
 */
export function inspectSteps(job: Pick<AssignmentJob, 'status' | 'progress'> | undefined): InspectStep[] {
  const active = !job ? 0 : isJobWorking(job.status) ? (WORKING_STEP[job.status] ?? 0) : DONE;
  const state = (step: number): InspectStep['state'] => (step < active ? 'done' : step === active ? 'active' : 'pending');
  const steps: InspectStep[] = [
    { label: active > 0 ? 'Fetched file list' : 'Fetching the file list', state: state(0) },
    { label: active > 1 ? 'Matched files' : 'Matching files', state: state(1) },
  ];
  const asked = job?.progress.ai_files ?? null;
  if (job?.status === 'resolving' || (asked !== null && asked > 0)) {
    const which = asked !== null ? plural(asked, 'tricky one') : 'the tricky ones';
    steps.push({ label: `${active > 2 ? 'Asked' : 'Asking'} AI about ${which}`, state: state(2) });
  }
  return steps;
}

// ---- Groups -----------------------------------------------------------------------------------

export type InspectSectionKind = 'map' | 'pick' | 'review' | 'specials' | 'ignored';

export interface InspectSection {
  kind: InspectSectionKind;
  /** "Will map · 9 episodes", "You'll review · 2 files". */
  title: string;
  /** The rows to show. */
  rows: Proposal[];
  /** "+ 5 more, all high confidence" for the rows left out, or null. */
  more: string | null;
}

/** Will map shows four files, then a count; extras six. */
const MAP_ROWS = 4;
const IGNORED_ROWS = 6;

function capped(kind: InspectSectionKind, title: string, proposals: Proposal[], limit: number): InspectSection {
  const rest = proposals.slice(limit);
  const allHigh = kind === 'map' && rest.every((p) => p.confidence === 'high');
  return {
    kind,
    title,
    rows: proposals.slice(0, limit),
    more: rest.length === 0 ? null : `+ ${rest.length} more${allHigh ? ', all high confidence' : ''}`,
  };
}

/**
 * The file list as Inspect groups it (HANDOFF §5.2): what will map (episodes,
 * or the movie file), what you'll review (unsure and unmatched files; a
 * movie's competing files), specials, then what's ignored.
 */
export function inspectSections(proposals: Proposal[]): InspectSection[] {
  const groups = new Map(groupProposals(proposals).map((entry) => [entry.group, entry.proposals]));
  const movie = proposals.some((p) => p.kind === 'movie');
  const sections: InspectSection[] = [];

  const matched = groups.get('matched') ?? [];
  if (matched.length > 0) {
    const episodes = new Set(matched.flatMap((p) => p.targets.map((t) => t.episode_id))).size;
    const count = movie || episodes === 0 ? plural(matched.length, 'file') : plural(episodes, 'episode');
    sections.push(capped('map', `Will map · ${count}`, matched, MAP_ROWS));
  }
  const review = [...(groups.get('needs_review') ?? []), ...(groups.get('unmatched') ?? [])];
  if (review.length > 0) {
    const count = plural(review.length, 'file');
    sections.push({ kind: movie ? 'pick' : 'review', title: movie ? `You’ll pick one · ${count}` : `You’ll review · ${count}`, rows: review, more: null });
  }
  const specials = groups.get('specials') ?? [];
  if (specials.length > 0) sections.push({ kind: 'specials', title: `Specials · ${specials.length}`, rows: specials, more: null });
  const extras = groups.get('extras') ?? [];
  if (extras.length > 0) sections.push(capped('ignored', `Ignored · ${extras.length}`, extras, IGNORED_ROWS));
  return sections;
}

// ---- Problems ---------------------------------------------------------------------------------

/** Starting Inspect failed (PLAN §6 inspect errors; resolve's 502 and 504 for a release without a magnet). */
export function inspectProblem(error: unknown): SearchProblem {
  const message = error instanceof Error ? error.message : '';
  switch (isApiError(error) ? error.code : null) {
    case 'not_found':
      return { title: 'This result expired', body: 'Search again to inspect it.', retry: false };
    case 'torrent_unavailable':
      return { title: 'Couldn’t fetch the torrent', body: message || 'The indexer didn’t hand over its .torrent file.', retry: true };
    case 'timeout':
      return { title: 'The indexer didn’t answer in time', body: message || 'Fetching the .torrent file took too long.', retry: true };
    default:
      return { title: 'Couldn’t inspect this release', body: message || 'Something went wrong on the way. Try again.', retry: true };
  }
}

/** The dry run itself failed (no peers sent the file list, no videos): the job's `error`. */
export function inspectJobProblem(job: Pick<AssignmentJob, 'status' | 'error'>): SearchProblem {
  if (job.status === 'cancelled') return { title: 'This preview was discarded', body: 'Inspect it again to look inside.', retry: true };
  return { title: 'Couldn’t look inside this torrent', body: job.error ?? 'Its file list never arrived.', retry: true };
}
