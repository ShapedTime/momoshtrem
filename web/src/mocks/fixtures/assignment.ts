import type {
  AssignHint,
  AssignmentJobCounts,
  AssignmentJobDetail,
  ExtraType,
  JobStatus,
  MatchConfidence,
  MatchSource,
  MediaType,
  Proposal,
  ProposalChange,
  ProposalDecision,
  ProposalGroup,
  ProposalKind,
  ProposalTarget,
  Timestamp,
} from '@/lib/api/contract';
import { canHaveEpisodes } from '@/lib/mapping/files';
import { type CatalogEntry, findTitle, iso } from './catalog';
import { absoluteNumbers, episodeIdFor, SHOWS, type ShowSeed } from './title-seeds';

/**
 * Mapping jobs (PLAN §6 "Assignment"; #5): templates, the seeded jobs and the
 * review rules, as plain functions. handlers/assignment.ts keeps the page's
 * jobs, walks them through their statuses and tells the library what they mean.
 *
 * | Job | Torrent | Review |
 * |---|---|---|
 * | 101 Frieren | the design's 16 files, #29–#38 | 2 files: 35v2 (S01E35, AI medium) and 28.5 (S00E03, AI low) |
 * | 102 One Piece | #892–#897 + Fan Letter, 8 files | 1 file: #892 (S21E01, AI medium) |
 * | 103 Doctor Who | the 2009 specials, 7 files | 3 files: an Easter special guessed by number (S00E01, AI low; a re-check moves it to S00E05) and two files that want S00E06 |
 *
 * New jobs (assign or Inspect) are built from the same templates: Frieren,
 * One Piece and Doctor Who get theirs, other shows a season pack that maps
 * cleanly (it applies on its own), movies one file (it applies on its own)
 * or, for a Blu-ray release, two cuts to choose from.
 */

const GB = 1e9;
const MB = 1e6;
const KB = 1e3;
const TODAY = iso(0).slice(0, 10);

type Ref = [season: number, episode: number];

/** What a re-check with AI answers for one proposal (scripted per fixture, as in the prototype); `next` is the following re-check's answer. */
export interface RecheckScript {
  confidence: MatchConfidence | null;
  reason: string;
  at?: Ref[];
  next?: RecheckScript;
}

interface FileDraft {
  /** Inside the torrent's folder. */
  file: string;
  size: number;
  group: ProposalGroup;
  reason: string;
  kind?: ProposalKind;
  extra_type?: ExtraType;
  at?: Ref[];
  /** Defaults: high in matched and specials, otherwise null. */
  confidence?: MatchConfidence | null;
  source?: MatchSource;
  recheck?: RecheckScript;
}

interface Template {
  name: string;
  release_group: string | null;
  indexer: string | null;
  suggested_hints: string[];
  /** Files the first AI run looks at; 0 when nothing needs asking. */
  ai_files: number;
  files: FileDraft[];
}

/** A proposal as the mock keeps it: the API's fields plus the suggestion Undo restores and its re-check script. */
export interface MockProposal extends Proposal {
  suggestion: { confidence: MatchConfidence | null; source: MatchSource; reason: string };
  recheck: RecheckScript | null;
  /** The rules' own `high` answer, which no AI run overrides (the backend runs the rules again to know it). */
  rules: { targets: ProposalTarget[]; reason: string } | null;
}

/** The statuses a job still has to pass through (mock-only). */
export interface Walk {
  kind: 'classify' | 'recheck';
  steps: JobStatus[];
  started: number;
  polls: number;
  scope?: 'unresolved' | 'all';
}

export interface MockJob {
  id: number;
  status: JobStatus;
  dry_run: boolean;
  media_type: MediaType;
  tmdb_id: number;
  library_id: number | null;
  magnet_uri: string;
  info_hash: string;
  name: string;
  release_group: string | null;
  indexer: string | null;
  hint: AssignHint | null;
  error: string | null;
  ai_error: string | null;
  suggested_hints: string[];
  /** Files sent to the AI in the current or latest run. */
  ai_files: number | null;
  /** Files the first AI run asks about (0: it doesn't run). */
  first_run_files: number;
  /** Steps done: 0 queued … 4 classified. */
  reached: number;
  /** Movies: the file the assign request picked up front. */
  pick?: string;
  /** The full list, also while the API still hides it. */
  proposals: MockProposal[];
  created_at: Timestamp;
  updated_at: Timestamp;
  applied_at: Timestamp | null;
  walk: Walk | null;
}

/** An episode's current file, for `replaces`. */
export interface EpisodeFile {
  info_hash: string;
  release_name: string;
  file_path: string;
}

/** What open mapping jobs tell the catalog about one title (PLAN §6 "Review"). */
export interface MockReview {
  /** Newest open job past classification (resolving, needs_review or ready). */
  job_id: number | null;
  /** Files pending with a target or in a conflict. */
  files: number;
  /** Episodes those files target. */
  episode_ids: Set<number>;
}

export const NO_REVIEW: MockReview = { job_id: null, files: 0, episode_ids: new Set() };

// ---- Rules ------------------------------------------------------------------

export const WORKING: readonly JobStatus[] = ['queued', 'fetching_metadata', 'classifying', 'resolving'];
export const CLOSED: readonly JobStatus[] = ['applied', 'failed', 'cancelled'];
const GROUP_ORDER: ProposalGroup[] = ['needs_review', 'unmatched', 'specials', 'matched', 'extras'];

export const defaultDecision = (group: ProposalGroup): ProposalDecision =>
  group === 'matched' || group === 'specials' ? 'accepted' : group === 'extras' ? 'ignored' : 'pending';

/** It has targets, or it's a movie candidate (it maps to the job's movie). */
export const maps = (p: Proposal) => p.kind === 'movie' || p.targets.length > 0;

const ids = (targets: ProposalTarget[]) => targets.map((t) => t.episode_id).join(',');

/** Non-ignored proposals whose targets share an episode. Movie candidates and subtitles never conflict. */
export function conflictIds(proposals: Proposal[]): Set<number> {
  const byEpisode = new Map<number, number[]>();
  for (const p of proposals) {
    if (p.decision === 'ignored' || p.kind === 'movie' || p.kind === 'subtitle') continue;
    for (const t of new Set(p.targets.map((target) => target.episode_id))) byEpisode.set(t, [...(byEpisode.get(t) ?? []), p.id]);
  }
  return new Set([...byEpisode.values()].filter((list) => list.length > 1).flat());
}

export function jobCounts(proposals: Proposal[]): AssignmentJobCounts {
  const conflicts = conflictIds(proposals);
  const pending = proposals.filter((p) => maps(p) && p.decision === 'pending');
  const accepted = proposals.filter((p) => p.decision === 'accepted');
  const special = (p: Proposal) => p.targets[0]?.season === 0;
  return {
    files: proposals.length,
    to_apply: proposals.filter((p) => p.decision !== 'ignored' && maps(p)).length,
    pending: pending.length,
    conflicts: conflicts.size,
    needs_review: new Set([...pending.map((p) => p.id), ...conflicts]).size,
    matched: accepted.filter((p) => !special(p)).length,
    specials: accepted.filter(special).length,
    ignored: proposals.filter((p) => p.decision === 'ignored').length,
    unmatched: proposals.filter((p) => p.decision !== 'ignored' && !maps(p)).length,
  };
}

/** After classification, every PATCH and every AI run. */
export function settledStatus(proposals: Proposal[]): 'needs_review' | 'ready' {
  const counts = jobCounts(proposals);
  return counts.pending > 0 || counts.conflicts > 0 ? 'needs_review' : 'ready';
}

/** Auto-apply: something to apply, no conflict, nothing pending or unmatched, everything that maps is high. */
export function autoApplies(job: MockJob): boolean {
  const counts = jobCounts(job.proposals);
  return (
    !job.dry_run &&
    counts.to_apply > 0 &&
    counts.conflicts === 0 &&
    counts.pending === 0 &&
    counts.unmatched === 0 &&
    job.proposals.every((p) => p.decision === 'ignored' || !maps(p) || p.confidence === 'high')
  );
}

/** Open jobs whose proposals exist: a re-check (`resolving`) doesn't make the title's banner flicker. */
const REVIEWABLE: readonly JobStatus[] = ['resolving', 'needs_review', 'ready'];

export function reviewOf(jobs: Iterable<MockJob>, mediaType: MediaType, libraryId: number): MockReview {
  const open = [...jobs].filter(
    (job) => !job.dry_run && job.media_type === mediaType && job.library_id === libraryId && REVIEWABLE.includes(job.status)
  );
  if (open.length === 0) return NO_REVIEW;
  const episodeIds = new Set<number>();
  let files = 0;
  for (const job of open) {
    const conflicts = conflictIds(job.proposals);
    for (const p of job.proposals) {
      if (!conflicts.has(p.id) && !(maps(p) && p.decision === 'pending')) continue;
      files++;
      for (const target of p.targets) episodeIds.add(target.episode_id);
    }
  }
  return { job_id: Math.max(...open.map((job) => job.id)), files, episode_ids: episodeIds };
}

// ---- Episodes -----------------------------------------------------------------

function target(show: ShowSeed, [season, episode]: Ref): ProposalTarget {
  const seed = show.seasons.find((s) => s.n === season)?.episodes.find((e) => e.n === episode);
  if (!seed) throw new Error(`Fixture bug: no S${season}E${episode}`);
  const id = episodeIdFor(show.key, season, episode);
  return { episode_id: id, season, episode, absolute_number: absoluteNumbers(show).get(id) ?? null, name: seed.name };
}

/** A show's episode by library id, as a target; undefined when it isn't that show's. */
export function targetById(tmdbId: number, episodeId: number): ProposalTarget | undefined {
  const show = SHOWS[tmdbId];
  if (!show || Math.floor(episodeId / 100_000) !== show.key) return undefined;
  const ref: Ref = [Math.floor((episodeId % 100_000) / 1000), episodeId % 1000];
  const exists = show.seasons.some((s) => s.n === ref[0] && s.episodes.some((e) => e.n === ref[1]));
  return exists ? target(show, ref) : undefined;
}

// ---- Templates --------------------------------------------------------------------

const FRIEREN = 209867;
const ONE_PIECE = 37854;
const DOCTOR_WHO = 57243;

const pad = (n: number) => String(n).padStart(2, '0');
const dotted = (title: string) =>
  title
    .replace(/&/g, 'and')
    .replace(/[’'":!?,()]/g, '')
    .trim()
    .replace(/\s+/g, '.');

function frieren(): Template {
  const crc = (n: number) => (0x8e3d5c21 + n * 977).toString(16).toUpperCase().slice(0, 8);
  const file = (label: string | number, hash?: string) => `[SubsPlease] Sousou no Frieren - ${label} (1080p)${hash ? ` [${hash}]` : ''}.mkv`;
  return {
    name: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]',
    release_group: 'SubsPlease',
    indexer: 'Nyaa',
    suggested_hints: ['these are absolute numbers', '28.5 is a recap', 'v2 replaces v1'],
    ai_files: 3,
    files: [
      ...[29, 30, 31, 32, 33, 34, 36, 37, 38].map(
        (n): FileDraft => ({
          file: file(n, crc(n)),
          size: (1.28 + (n % 3) * 0.04) * GB,
          group: 'matched',
          at: [[1, n]],
          reason: `Absolute #${n} → S01E${pad(n)} (single-season show)`,
        })
      ),
      {
        file: file('35v2', '5A1B3C9D'),
        size: 1.31 * GB,
        group: 'needs_review',
        at: [[1, 35]],
        confidence: 'medium',
        source: 'ai',
        reason: '"v2" is a re-release of #35, and this torrent has no v1 → S01E35',
        recheck: { confidence: 'high', reason: 'v2 fixes the subtitle timing in #35, so it’s the one to keep → S01E35' },
      },
      {
        file: file('28.5', '0C4E9A12'),
        size: 640 * MB,
        group: 'needs_review',
        at: [[0, 3]],
        confidence: 'low',
        source: 'ai',
        reason: '".5" usually marks a recap special; matched S00E03 by air date (Mar 15)',
        recheck: { confidence: 'high', reason: '28.5 is the recap “The Journey So Far” that aired Mar 15 → S00E03' },
      },
      {
        file: file('Mini Anime 05', 'AB12CD34'),
        size: 58 * MB,
        group: 'specials',
        at: [[0, 6]],
        source: 'ai',
        reason: '"Mini Anime 05" → S00E06 by title',
      },
      { file: file('NCOP2'), size: 142 * MB, group: 'extras', kind: 'extra', extra_type: 'opening', reason: 'NCOP = creditless opening; ignored by default' },
      { file: file('NCED2'), size: 139 * MB, group: 'extras', kind: 'extra', extra_type: 'ending', reason: 'NCED = creditless ending; ignored by default' },
      { file: 'Fonts/OpenSans-Semibold.ttf', size: 220 * KB, group: 'extras', kind: 'extra', extra_type: 'font', reason: 'Not a video' },
      {
        file: file('Special Extra', 'F00DBABE'),
        size: 210 * MB,
        group: 'unmatched',
        reason: 'No number and no special title matched. Pick an episode or ignore it.',
        recheck: { confidence: null, reason: 'Looks like a Blu-ray bonus that TMDB doesn’t list. Ignoring it is safe.' },
      },
    ],
  };
}

/** One Piece #897 (rules: S21E06, high) over four re-checks: a dispute, another, a low answer that changes nothing, then agreement. */
const DISPUTED_897: RecheckScript = {
  confidence: 'medium',
  at: [[21, 7]],
  reason: 'TMDB’s air order puts #897 a week later',
  next: {
    confidence: 'high',
    at: [[21, 8]],
    reason: 'the runtime fits S21E08 better',
    next: { confidence: 'low', at: [[21, 9]], reason: 'maybe S21E09', next: { confidence: 'high', at: [[21, 6]], reason: 'the air date matches S21E06' } },
  },
};

function onePiece(): Template {
  const file = (abs: number) => `One.Piece.${abs}.1080p.WEB.x264-GROUP.mkv`;
  return {
    name: 'One.Piece.892-897.+.Fan.Letter.1080p.WEB.x264-GROUP',
    release_group: 'GROUP',
    indexer: 'TorrentLeech',
    suggested_hints: ['these are absolute numbers', 'use TMDB air order', 'Fan Letter is a special'],
    ai_files: 2,
    files: [
      {
        file: file(892),
        size: 1.21 * GB,
        group: 'needs_review',
        at: [[21, 1]],
        confidence: 'medium',
        source: 'ai',
        reason: 'Absolute #892 → S21E01 by air order. TMDB starts Wano here, unlike the broadcast arcs, so check this one.',
        recheck: { confidence: 'high', reason: 'Absolute #892 → S21E01 by air date (Jul 7 2019); the title matches TMDB too' },
      },
      ...[893, 894, 895, 896, 897].map(
        (abs, i): FileDraft => ({
          file: file(abs),
          size: (1.18 + (abs % 4) * 0.03) * GB,
          group: 'matched',
          at: [[21, 2 + i]],
          reason: `Absolute #${abs} → S21E${pad(2 + i)} (same offset as #892)`,
          // Re-checks of all files dispute this one (it goes back to review, keeping its episode), then agree
          ...(abs === 897 && { recheck: DISPUTED_897 }),
        })
      ),
      {
        file: 'One.Piece.Fan.Letter.1080p.WEB.x264-GROUP.mkv',
        size: 1.02 * GB,
        group: 'specials',
        at: [[0, 41]],
        source: 'ai',
        reason: '"Fan Letter" is the Oct 2024 TV special → S00E41 by title',
      },
      { file: 'Sample/One.Piece.892.sample.mkv', size: 48 * MB, group: 'extras', kind: 'extra', extra_type: 'sample', reason: 'Sample clip; ignored by default' },
    ],
  };
}

function doctorWho(): Template {
  const file = (label: string) => `Doctor.Who.2005.${label}.720p.BluRay.x264-SHORTBREHD.mkv`;
  return {
    name: 'Doctor.Who.2005.The.2009.Specials.720p.BluRay.x264-SHORTBREHD',
    release_group: 'SHORTBREHD',
    indexer: 'TorrentLeech',
    suggested_hints: ['match specials by title', 'Confidential is a different show'],
    ai_files: 1,
    files: [
      // A re-check moves this one to another episode (Undo then goes back to the AI's new suggestion)
      {
        file: file('S00E01.Easter.Special'),
        size: 1.42 * GB,
        group: 'needs_review',
        at: [[0, 1]],
        confidence: 'low',
        source: 'ai',
        reason: 'No title in the name, so S00E01 by number, but this pack’s numbers don’t match TMDB’s specials',
        recheck: { confidence: 'high', at: [[0, 5]], reason: 'The 2009 Easter special is "Planet of the Dead" (Apr 11 2009): TMDB’s S00E05' },
      },
      // Two files want S00E06, which already has a file: a conflict, and a `replaces` note on both
      {
        file: file('S00E02.The.Waters.of.Mars'),
        size: 1.71 * GB,
        group: 'needs_review',
        at: [[0, 6]],
        confidence: 'high',
        reason: 'Title "The Waters of Mars" → S00E06 (numbers differ from the file)',
      },
      {
        file: file('The.Waters.of.Mars.Extended.Edition'),
        size: 1.88 * GB,
        group: 'needs_review',
        at: [[0, 6]],
        confidence: 'medium',
        reason: 'Title "The Waters of Mars" → S00E06; "Extended Edition" is probably a second cut',
      },
      {
        file: file('S00E03.The.End.of.Time.Part.1'),
        size: 1.69 * GB,
        group: 'specials',
        at: [[0, 7]],
        reason: 'Title "The End of Time (1)" → S00E07 (numbers differ from the file)',
      },
      {
        file: file('S00E04.The.End.of.Time.Part.2'),
        size: 1.74 * GB,
        group: 'specials',
        at: [[0, 8]],
        reason: 'Title "The End of Time (2)" → S00E08 (numbers differ from the file)',
      },
      {
        file: 'Doctor.Who.2005.The.2009.Specials.720p.BluRay.x264-SHORTBREHD.nfo',
        size: 6 * KB,
        group: 'extras',
        kind: 'extra',
        extra_type: 'info',
        reason: 'Not a video',
      },
      {
        file: 'Doctor.Who.Confidential.The.End.of.Time.720p.mkv',
        size: 640 * MB,
        group: 'unmatched',
        reason: 'Doctor Who Confidential is a separate show on TMDB. Pick an episode or ignore it.',
        recheck: { confidence: null, reason: 'Confidential is a behind-the-scenes show, not an episode. Ignoring it is safe.' },
      },
    ],
  };
}

/** Any other show: a pack of its latest season that maps cleanly, so the job applies on its own. */
function seasonPack(entry: CatalogEntry, name: string | null): Template {
  const show = SHOWS[entry.tmdb_id];
  const season = show?.seasons.filter((s) => s.n > 0).at(-1);
  const title = dotted(entry.title);
  const pack = name ?? `${title}.S${pad(season?.n ?? 1)}.1080p.WEB-DL.x264-GROUP`;
  const aired = season?.episodes.filter((e) => e.air_date !== null && e.air_date <= TODAY) ?? [];
  return {
    name: pack,
    release_group: 'GROUP',
    indexer: null,
    suggested_hints: [],
    ai_files: 0,
    files: [
      ...aired.map((e): FileDraft => {
        const code = `S${pad(season?.n ?? 1)}E${pad(e.n)}`;
        return { file: `${title}.${code}.1080p.WEB-DL.x264-GROUP.mkv`, size: 1.4 * GB, group: 'matched', at: [[season?.n ?? 1, e.n]], reason: `"${code}" in the name` };
      }),
      ...(aired.length === 0
        ? [{ file: `${title}.mkv`, size: 1.4 * GB, group: 'unmatched' as const, reason: 'No episode number in the name. Pick an episode or ignore it.' }]
        : []),
      { file: `Sample/${title}.sample.mkv`, size: 40 * MB, group: 'extras', kind: 'extra', extra_type: 'sample', reason: 'Sample clip; ignored by default' },
    ],
  };
}

/**
 * Movies: a Blu-ray release has two feature-length cuts to choose from (the
 * job waits for review); any other release has one plausible file, so the
 * job applies on its own. Both come with a sample. The AI never runs for movies.
 */
function movieCuts(entry: CatalogEntry, name: string | null): Template {
  const release = name ?? `${dotted(entry.title)}.${entry.year}.2160p.WEB-DL.DDP5.1.H.265-GROUP`;
  const sample: FileDraft = {
    file: `Sample/${release}.sample.mkv`,
    size: 60 * MB,
    group: 'extras',
    kind: 'extra',
    extra_type: 'sample',
    reason: 'Sample clip; ignored by default',
  };
  if (!/blu-?ray/i.test(release)) {
    return {
      name: release,
      release_group: 'GROUP',
      indexer: null,
      suggested_hints: [],
      ai_files: 0,
      files: [{ file: `${release}.mkv`, size: 14.2 * GB, group: 'matched', kind: 'movie', reason: 'The only feature-length video; title and year match' }, sample],
    };
  }
  return {
    name: release,
    release_group: 'GROUP',
    indexer: null,
    suggested_hints: [],
    ai_files: 0,
    files: [
      {
        file: `${release}.mkv`,
        size: 14.2 * GB,
        group: 'needs_review',
        kind: 'movie',
        confidence: 'medium',
        reason: 'Title and year match; the runtime fits the theatrical cut',
      },
      {
        file: `${release}.Extended.Cut.mkv`,
        size: 16.8 * GB,
        group: 'needs_review',
        kind: 'movie',
        confidence: 'low',
        reason: 'Also feature-length: probably the extended cut',
      },
      sample,
    ],
  };
}

const NAMED: Record<number, () => Template> = { [FRIEREN]: frieren, [ONE_PIECE]: onePiece, [DOCTOR_WHO]: doctorWho };

function templateFor(entry: CatalogEntry, name: string | null): Template {
  if (entry.media_type === 'movie') return movieCuts(entry, name);
  const named = NAMED[entry.tmdb_id];
  if (!named) return seasonPack(entry, name);
  const template = named();
  return name ? { ...template, name } : template;
}

// ---- Building jobs ------------------------------------------------------------------

function proposal(id: number, root: string, draft: FileDraft, show: ShowSeed | undefined): MockProposal {
  const targets = show ? (draft.at ?? []).map((ref) => target(show, ref)) : [];
  const confidence = draft.confidence !== undefined ? draft.confidence : draft.group === 'matched' || draft.group === 'specials' ? 'high' : null;
  const source = draft.source ?? 'rules';
  return {
    id,
    file_path: `${root}/${draft.file}`,
    file_size: Math.round(draft.size),
    kind: draft.kind ?? 'episode',
    group: draft.group,
    extra_type: draft.extra_type ?? null,
    targets,
    suggested_targets: targets,
    confidence,
    source,
    reason: draft.reason,
    decision: defaultDecision(draft.group),
    user_decided: false,
    replaces: null,
    updated: false,
    hinted: false,
    subtitle_count: 0,
    superseded_by: null,
    suggestion: { confidence, source, reason: draft.reason },
    recheck: draft.recheck ?? null,
    rules: source === 'rules' && confidence === 'high' && targets.length > 0 ? { targets, reason: draft.reason } : null,
  };
}

export interface NewJob {
  id: number;
  media_type: MediaType;
  tmdb_id: number;
  library_id: number | null;
  magnet_uri: string;
  info_hash: string;
  hint?: AssignHint | null;
  indexer?: string | null;
  dry_run?: boolean;
  pick?: string;
  /** The AI resolver runs (when the template has anything to ask). */
  ai: boolean;
  now: number;
}

/** A job just created: queued, with the steps it will walk. */
export function newJob(input: NewJob): MockJob {
  const entry = findTitle(input.media_type, input.tmdb_id);
  if (!entry) throw new Error(`No catalog entry for ${input.media_type} ${input.tmdb_id}`);
  const dn = new URLSearchParams(input.magnet_uri.replace(/^magnet:\?/, '')).get('dn');
  const template = templateFor(entry, dn);
  const show = input.media_type === 'tv' ? SHOWS[input.tmdb_id] : undefined;
  const asks = input.ai && template.ai_files > 0;
  const created = new Date(input.now).toISOString();
  return {
    id: input.id,
    status: 'queued',
    dry_run: input.dry_run ?? false,
    media_type: input.media_type,
    tmdb_id: input.tmdb_id,
    library_id: input.library_id,
    magnet_uri: input.magnet_uri,
    info_hash: input.info_hash,
    name: template.name,
    release_group: template.release_group,
    indexer: input.indexer ?? template.indexer,
    hint: input.hint ?? null,
    error: null,
    ai_error: null,
    suggested_hints: template.suggested_hints,
    ai_files: null,
    first_run_files: asks ? template.ai_files : 0,
    reached: 0,
    pick: input.pick,
    proposals: template.files.map((draft, i) => proposal(input.id * 1000 + i + 1, template.name, draft, show)),
    created_at: created,
    updated_at: created,
    applied_at: null,
    walk: {
      kind: 'classify',
      steps: ['queued', 'fetching_metadata', 'classifying', ...(asks ? (['resolving'] as const) : [])],
      started: input.now,
      polls: 0,
    },
  };
}

const SEEDS: Array<Omit<NewJob, 'ai' | 'now'> & { days_ago: number }> = [
  {
    id: 101,
    media_type: 'tv',
    tmdb_id: FRIEREN,
    library_id: 1,
    info_hash: 'b7a3f1e9c2d4058a6e1f3c5b7d9a2e4f6081c3d5',
    magnet_uri: 'magnet:?xt=urn:btih:b7a3f1e9c2d4058a6e1f3c5b7d9a2e4f6081c3d5',
    days_ago: 2,
  },
  {
    id: 102,
    media_type: 'tv',
    tmdb_id: ONE_PIECE,
    library_id: 2,
    info_hash: '3c9e04a1b2d6f8e0a4c7d1b3e5f709a2c4e6b8d0',
    magnet_uri: 'magnet:?xt=urn:btih:3c9e04a1b2d6f8e0a4c7d1b3e5f709a2c4e6b8d0',
    days_ago: 1,
  },
  {
    id: 103,
    media_type: 'tv',
    tmdb_id: DOCTOR_WHO,
    library_id: 3,
    info_hash: 'e1d2c3b4a5968778695a4b3c2d1e0f1a2b3c4d5e',
    magnet_uri: 'magnet:?xt=urn:btih:e1d2c3b4a5968778695a4b3c2d1e0f1a2b3c4d5e',
    days_ago: 0,
  },
];

/** The jobs a page starts with: classified, waiting for review. */
export function seedJobs(): Map<number, MockJob> {
  return new Map(
    SEEDS.map(({ days_ago, ...seed }) => {
      const job = newJob({ ...seed, ai: true, now: Date.parse(iso(days_ago)) });
      job.walk = null;
      job.reached = 4;
      job.ai_files = job.first_run_files;
      job.status = settledStatus(job.proposals);
      return [job.id, job];
    })
  );
}

let seeded: MockJob[] | undefined;

/** What the seeded jobs tell the catalog about a title, for the seeded library. */
export function seededReview(mediaType: MediaType, libraryId: number): MockReview {
  seeded ??= [...seedJobs().values()];
  return reviewOf(seeded, mediaType, libraryId);
}

// ---- Changes ----------------------------------------------------------------------------

/** PATCH: validate every change, then make them all (all or nothing). Returns the 422 message, or null. */
export function patchProposals(job: MockJob, items: ProposalChange[]): string | null {
  const next = new Map<number, MockProposal>();
  for (const item of items) {
    const current = next.get(item.id) ?? job.proposals.find((p) => p.id === item.id);
    if (!current) return `This job has no proposal ${item.id}.`;
    const p: MockProposal = { ...current };
    if (item.episode_ids !== undefined) {
      if (job.media_type === 'movie') return 'Movie files have no episodes: accept the file instead.';
      const targets = item.episode_ids.map((id) => targetById(job.tmdb_id, id));
      const missing = item.episode_ids.find((_, i) => !targets[i]);
      if (missing !== undefined) return `Episode ${missing} isn’t one of this show’s.`;
      p.targets = targets.filter((t): t is ProposalTarget => t !== undefined);
      // Picking the suggestion again restores it; anything else is the user's pick
      Object.assign(p, ids(p.targets) === ids(p.suggested_targets) ? p.suggestion : { confidence: null, source: 'manual', reason: 'You picked this episode' });
    }
    if (item.decision !== undefined) p.decision = item.decision;
    if (p.decision === 'accepted' && !maps(p)) return `Proposal ${item.id}: pick an episode before accepting it.`;
    // As the backend's takesEpisodes: fonts, info files and images can't play an episode
    if (p.targets.length > 0 && (item.episode_ids !== undefined || p.decision === 'accepted') && !canHaveEpisodes(p)) {
      return `"${p.file_path.split('/').pop()}" isn't a video, so it can't play an episode.`;
    }
    next.set(p.id, p);
  }
  // A movie job maps one file: the first candidate accepted here wins, and the others are ignored
  const acceptedMovie = [...next.values()].find((p) => p.kind === 'movie' && p.decision === 'accepted');
  job.proposals = job.proposals.map((p) => {
    const sent = next.get(p.id);
    const stepsAside = acceptedMovie && p.kind === 'movie' && p.id !== acceptedMovie.id && (sent || p.decision !== 'ignored');
    const changed = stepsAside ? { ...(sent ?? p), decision: 'ignored' as const } : (sent ?? p);
    const back = ids(changed.targets) === ids(changed.suggested_targets) && changed.decision === defaultDecision(changed.group);
    return changed === p ? p : { ...changed, user_decided: !back };
  });
  return null;
}

/**
 * Proposals a re-check asks about: episode files the user hasn't decided; for `unresolved`, only those pending
 * (unmatched included) or in a conflict. Extras and subtitles keep the rules' reading.
 */
export const inScope = (p: Proposal, scope: 'unresolved' | 'all', conflicts: Set<number>) =>
  !p.user_decided && p.kind === 'episode' && (scope === 'all' || p.decision === 'pending' || conflicts.has(p.id));

const codes = (targets: ProposalTarget[]) => targets.map((t) => `S${pad(t.season)}E${pad(t.episode)}`).join('+');
const clip = (reason: string) => (reason.length > 120 ? `${reason.slice(0, 119)}…` : reason);

/**
 * One proposal after the AI answered `script`, as the backend works it out (PLAN §6 "Re-check"), or
 * itself when nothing changes.
 * - A `high` rules answer on its own targets is protected, also once an earlier re-check flagged it:
 *   a medium or high AI answer for other episodes keeps the targets, drops it to medium and names the
 *   AI's pick; a dispute still on its group's default acceptance goes back to pending. A low answer
 *   changes nothing. When the AI agrees with a flagged answer, the flag lifts: the rules' high and
 *   reason come back, and the decision stays.
 * - Any other answer takes the AI's, except that an accepted proposal never loses its target.
 */
function recheckAnswer(p: MockProposal, script: RecheckScript, aiTargets: ProposalTarget[], hinted: boolean): MockProposal {
  const changed = (fields: Partial<MockProposal>): MockProposal => ({ ...p, ...fields, updated: true, hinted });
  if (p.rules && ids(p.rules.targets) === ids(p.targets)) {
    if (ids(aiTargets) === ids(p.targets)) {
      const flagged = p.confidence !== 'high' || p.reason !== p.rules.reason;
      if (!flagged) return p;
      const suggestion = { confidence: 'high' as const, source: 'rules' as const, reason: p.rules.reason };
      return changed({ ...suggestion, suggestion });
    }
    if (script.confidence !== 'high' && script.confidence !== 'medium') return p;
    const reason = clip(`AI suggests ${codes(aiTargets)}: ${script.reason}`);
    if (p.confidence === 'medium' && p.reason === reason) return p;
    const suggestion = { confidence: 'medium' as const, source: 'rules' as const, reason };
    const decision = p.decision === 'accepted' && p.decision === defaultDecision(p.group) ? 'pending' : p.decision;
    return changed({ ...suggestion, suggestion, decision });
  }
  if (aiTargets.length === 0 && p.decision === 'accepted') return p;
  const suggestion = { confidence: script.confidence, source: 'ai' as const, reason: script.reason };
  return changed({ ...suggestion, suggestion, targets: aiTargets, suggested_targets: aiTargets });
}

/** A re-check's AI writes: each proposal still in scope gets its scripted answer, and the script moves on to the next one. */
export function finishRecheck(job: MockJob, scope: 'unresolved' | 'all'): void {
  const show = SHOWS[job.tmdb_id];
  const hinted = Boolean(job.hint?.text);
  const conflicts = conflictIds(job.proposals);
  job.proposals = job.proposals.map((p) => {
    const script = p.recheck;
    if (!script || !inScope(p, scope, conflicts)) return p;
    const aiTargets = script.at && show ? script.at.map((ref) => target(show, ref)) : p.targets;
    return { ...recheckAnswer(p, script, aiTargets, hinted), recheck: script.next ?? script };
  });
}

// ---- The API's answer ---------------------------------------------------------------------

export interface DetailContext {
  entry: CatalogEntry;
  ai_available: boolean;
  /** The file an episode has now, if any. */
  currentFile: (episodeId: number) => EpisodeFile | null;
}

/** Video files that aren't extras (`torrent.video_count`, `progress.videos`). */
const isVideo = (p: Proposal) => p.kind !== 'extra' && /\.(mkv|mp4|avi|m4v|ts)$/i.test(p.file_path);

const CONFIDENCE_ORDER: Array<MatchConfidence | null> = ['high', 'medium', 'low', null];

const sortKey = (p: Proposal) => {
  const t = p.targets[0];
  // Regular seasons first, Specials after them, then files without a target (movie candidates best first)
  const season = t ? (t.season === 0 ? 10_000 : t.season) : 20_000;
  return [GROUP_ORDER.indexOf(p.group), season, t?.episode ?? 0, CONFIDENCE_ORDER.indexOf(p.confidence)] as const;
};

function compareProposals(a: Proposal, b: Proposal): number {
  const [ka, kb] = [sortKey(a), sortKey(b)];
  for (let i = 0; i < ka.length; i++) if (ka[i] !== kb[i]) return ka[i] - kb[i];
  return a.file_path < b.file_path ? -1 : a.file_path > b.file_path ? 1 : 0;
}

/** The API's fields, without the mock's bookkeeping. */
const apiFields = ({
  id,
  file_path,
  file_size,
  kind,
  group,
  extra_type,
  targets,
  suggested_targets,
  confidence,
  source,
  reason,
  decision,
  user_decided,
  replaces,
  updated,
  hinted,
  subtitle_count,
  superseded_by,
}: MockProposal): Proposal => ({
  id,
  file_path,
  file_size,
  kind,
  group,
  extra_type,
  targets,
  suggested_targets,
  confidence,
  source,
  reason,
  decision,
  user_decided,
  replaces,
  updated,
  hinted,
  subtitle_count,
  superseded_by,
});

function seasonsAndEpisodes(job: MockJob): { seasons: number | null; episodes: number | null } {
  const show = job.media_type === 'tv' ? SHOWS[job.tmdb_id] : undefined;
  if (!show || job.reached < 2) return { seasons: null, episodes: null };
  return { seasons: show.seasons.length, episodes: show.seasons.reduce((sum, s) => sum + s.episodes.length, 0) };
}

/** GET /assignment-jobs/{id}, as the backend answers it at this moment. */
export function jobDetail(job: MockJob, ctx: DetailContext): AssignmentJobDetail {
  // Proposals exist once classification is done (from `resolving` on), also for a job cancelled before that
  const shown = job.reached >= 3 ? job.proposals : [];
  const listed = job.reached >= 2 && job.proposals.length > 0;
  const proposals = shown
    .map(apiFields)
    .map((p): Proposal => {
      const file = p.targets[0] ? ctx.currentFile(p.targets[0].episode_id) : null;
      const own = file?.info_hash === job.info_hash && file.file_path === p.file_path;
      return { ...p, replaces: file && !own ? { release_name: file.release_name, file_path: file.file_path } : null };
    })
    .sort(compareProposals);
  return {
    job: {
      id: job.id,
      status: job.status,
      dry_run: job.dry_run,
      item: {
        id: job.library_id,
        media_type: job.media_type,
        tmdb_id: job.tmdb_id,
        title: ctx.entry.title,
        year: ctx.entry.year,
        poster_path: null,
      },
      torrent: {
        info_hash: job.info_hash,
        name: job.name,
        size_bytes: listed ? job.proposals.reduce((sum, p) => sum + p.file_size, 0) : null,
        file_count: listed ? job.proposals.length : null,
        video_count: listed ? job.proposals.filter(isVideo).length : null,
        release_group: job.release_group,
        indexer: job.indexer,
      },
      progress: {
        ...seasonsAndEpisodes(job),
        files: listed ? job.proposals.length : null,
        videos: listed ? job.proposals.filter(isVideo).length : null,
        ai_files: job.ai_files,
      },
      hint: job.hint,
      ai_available: ctx.ai_available,
      ai_error: job.ai_error,
      suggested_hints: job.suggested_hints,
      counts: jobCounts(proposals),
      error: job.error,
      created_at: job.created_at,
      updated_at: job.updated_at,
      applied_at: job.applied_at,
    },
    proposals,
  };
}
