import {
  type AssignmentJob,
  type ExtraType,
  isJobWorking,
  type JobStatus,
  type Proposal,
  type ProposalChange,
  type ProposalDecision,
  type ProposalGroup,
  type ProposalTarget,
} from '@/lib/api/contract';
import { STATUS, type StatusTone } from '@/lib/status';
import { episodeCode } from '@/lib/title/episodes';
import { plural } from '@/lib/title/format';

/**
 * The mapping review's rules and words (PLAN §6 "Assignment", F4; #5), as
 * pure functions over a job's proposals: groups, counts, conflicts, the
 * summary line, the sticky foot and the Applied summary. The UI keeps no
 * other state about a proposal than what the server answered.
 */

const pad = (n: number) => String(n).padStart(2, '0');

// ---- Groups --------------------------------------------------------------------------

/** Urgency order: decisions first, the long tail last. */
export const GROUP_ORDER: readonly ProposalGroup[] = ['needs_review', 'unmatched', 'specials', 'matched', 'extras'];

export type GroupTone = Extract<StatusTone, 'review' | 'warning' | 'upcoming' | 'ready' | 'missing'>;

export interface GroupCopy {
  title: string;
  tone: GroupTone;
  description: string;
}

export const GROUPS: Record<ProposalGroup, GroupCopy> = {
  needs_review: {
    title: STATUS.needs_review.label,
    tone: 'review',
    description: 'Noir wasn’t sure about these. Accept the suggestion or pick another episode.',
  },
  unmatched: {
    title: STATUS.unmatched.label,
    tone: 'warning',
    description: 'No episode found. Pick one, or ignore the file. Unmapped files stay in the torrent but not in Infuse.',
  },
  specials: { title: 'Specials', tone: 'upcoming', description: 'Mapped to Season 0. Infuse lists them under Specials.' },
  matched: { title: STATUS.matched.label, tone: 'ready', description: 'Confident matches. Open to spot-check.' },
  extras: {
    title: 'Extras & ignored',
    tone: 'missing',
    description: 'Openings, endings, samples and non-video files stay out of Infuse.',
  },
};

/** Movie candidates have their own words: there's one movie, and the files compete for it. */
export const MOVIE_GROUPS: Partial<Record<ProposalGroup, Pick<GroupCopy, 'description'>>> = {
  needs_review: { description: 'More than one file could be the movie. Use the right one; the others stay out of Infuse.' },
  matched: { description: 'The movie file.' },
};

/** Groups with rows, in urgency order. */
export function groupProposals(proposals: Proposal[]): Array<{ group: ProposalGroup; proposals: Proposal[] }> {
  return GROUP_ORDER.map((group) => ({ group, proposals: proposals.filter((p) => p.group === group) })).filter(
    (entry) => entry.proposals.length > 0
  );
}

/** Everything is open on desktop; below 1024 px the long, settled groups start collapsed. */
export const defaultGroupOpen = (group: ProposalGroup, desktop: boolean): boolean =>
  desktop || (group !== 'matched' && group !== 'extras');

/** Full rows (decide here) vs compact ones (spot-check). */
export const isFullRow = (p: Proposal): boolean =>
  (p.group === 'needs_review' || p.group === 'unmatched') && p.decision !== 'ignored';

// ---- Mapping, conflicts and counts --------------------------------------------------------

/** It has targets, or it's a movie candidate (it maps to the job's movie). */
export const maps = (p: Proposal): boolean => p.kind === 'movie' || p.targets.length > 0;

const isSpecial = (p: Proposal) => p.targets[0]?.season === 0;

/**
 * Two or more non-ignored proposals whose targets share an episode: for each
 * proposal in a conflict, the others it collides with. Movie candidates never conflict.
 */
export function conflictMap(proposals: Proposal[]): Map<number, Proposal[]> {
  const byEpisode = new Map<number, Proposal[]>();
  for (const p of proposals) {
    if (p.decision === 'ignored' || p.kind === 'movie') continue;
    for (const id of new Set(p.targets.map((t) => t.episode_id))) byEpisode.set(id, [...(byEpisode.get(id) ?? []), p]);
  }
  const out = new Map<number, Proposal[]>();
  for (const sharing of byEpisode.values()) {
    if (sharing.length < 2) continue;
    for (const p of sharing) {
      const others = out.get(p.id) ?? [];
      for (const other of sharing) if (other.id !== p.id && !others.includes(other)) others.push(other);
      out.set(p.id, others);
    }
  }
  return out;
}

export interface ReviewCounts {
  /** Accepted regular episodes (or the accepted movie file), not in a conflict. */
  matched: number;
  /** Maps and pending, or in a conflict. */
  review: number;
  /** Accepted Season 0, not in a conflict. */
  specials: number;
  ignored: number;
  /** Not ignored and doesn't map. Never blocks. */
  unmatched: number;
  /** Not ignored and maps: what Apply writes. */
  toApply: number;
  /** Maps and pending: they block Apply. */
  pending: number;
  /** Proposals in a conflict: they block Apply. */
  conflicts: number;
}

/** One bucket per proposal (they add up to the file count), plus what blocks Apply. */
export function reviewCounts(proposals: Proposal[]): ReviewCounts {
  const conflicts = conflictMap(proposals);
  const counts: ReviewCounts = { matched: 0, review: 0, specials: 0, ignored: 0, unmatched: 0, toApply: 0, pending: 0, conflicts: conflicts.size };
  for (const p of proposals) {
    if (p.decision === 'ignored') counts.ignored++;
    else if (!maps(p)) counts.unmatched++;
    else {
      counts.toApply++;
      if (p.decision === 'pending') counts.pending++;
      if (p.decision === 'pending' || conflicts.has(p.id)) counts.review++;
      else if (isSpecial(p)) counts.specials++;
      else counts.matched++;
    }
  }
  return counts;
}

/** Proposals in display order (group urgency, then the server's order). */
export function displayOrder(proposals: Proposal[]): Proposal[] {
  return groupProposals(proposals).flatMap((entry) => entry.proposals);
}

/** The first pending proposal that maps, in display order (selected on load). */
export function firstPending(proposals: Proposal[], after?: number): Proposal | undefined {
  return displayOrder(proposals).find((p) => p.decision === 'pending' && maps(p) && p.id !== after);
}

/** What a disabled Apply jumps to: the first conflict, else the first pending file. */
export function firstBlocker(proposals: Proposal[]): { proposal: Proposal; reason: 'conflict' | 'pending' } | undefined {
  const ordered = displayOrder(proposals);
  const conflicts = conflictMap(proposals);
  const conflict = ordered.find((p) => conflicts.has(p.id));
  if (conflict) return { proposal: conflict, reason: 'conflict' };
  const pending = ordered.find((p) => p.decision === 'pending' && maps(p));
  return pending ? { proposal: pending, reason: 'pending' } : undefined;
}

// ---- Summary line ------------------------------------------------------------------------

export interface SummaryPart {
  key: 'matched' | 'review' | 'specials' | 'ignored' | 'unmatched';
  count: number;
  label: string;
  tone: GroupTone;
}

/** "● 9 matched ● 2 need review ● 1 special ● 3 ignored ● 1 unmatched": zero counts hide, except review. */
export function summaryParts(counts: ReviewCounts): SummaryPart[] {
  const word = (key: 'matched' | 'ignored' | 'unmatched') => STATUS[key].label.toLowerCase();
  const parts: SummaryPart[] = [
    { key: 'matched', count: counts.matched, label: word('matched'), tone: 'ready' },
    {
      key: 'review',
      count: counts.review,
      label: counts.review === 1 ? 'needs review' : counts.review > 0 ? 'need review' : 'left to review',
      tone: 'review',
    },
    { key: 'specials', count: counts.specials, label: counts.specials === 1 ? 'special' : 'specials', tone: 'upcoming' },
    { key: 'ignored', count: counts.ignored, label: word('ignored'), tone: 'missing' },
    { key: 'unmatched', count: counts.unmatched, label: word('unmatched'), tone: 'warning' },
  ];
  return parts.filter((part) => part.count > 0 || part.key === 'review');
}

// ---- Episode codes and ranges ---------------------------------------------------------------

/** One target "S01E05"; several in a season "S01E01–E02"; across seasons "S01E12 + S02E01". */
export function targetCode(targets: Pick<ProposalTarget, 'season' | 'episode'>[]): string {
  if (targets.length === 0) return '';
  const [first] = targets;
  const last = targets[targets.length - 1];
  if (targets.length === 1) return episodeCode(first.season, first.episode);
  if (targets.every((t) => t.season === first.season)) return `${episodeCode(first.season, first.episode)}–E${pad(last.episode)}`;
  return targets.map((t) => episodeCode(t.season, t.episode)).join(' + ');
}

/** The episode names of a target list, "Rose / The End of the World". */
export const targetName = (targets: Pick<ProposalTarget, 'name'>[]): string => targets.map((t) => t.name).join(' / ');

/** [29…34, 36, 37] → "E29–E34, E36, E37": three or more in a row become a range. */
export function episodeRanges(episodes: number[]): string {
  const sorted = [...new Set(episodes)].sort((a, b) => a - b);
  const out: string[] = [];
  for (let i = 0; i < sorted.length; i++) {
    let j = i;
    while (j + 1 < sorted.length && sorted[j + 1] === sorted[j] + 1) j++;
    if (j > i + 1) out.push(`E${pad(sorted[i])}–E${pad(sorted[j])}`);
    else if (j === i + 1) out.push(`E${pad(sorted[i])}, E${pad(sorted[j])}`);
    else out.push(`E${pad(sorted[i])}`);
    i = j;
  }
  return out.join(', ');
}

/** Episodes per season, numbered seasons in order and Specials (0) last. */
function bySeason(proposals: Proposal[]): Array<{ season: number; episodes: number[] }> {
  const seasons = new Map<number, number[]>();
  for (const p of proposals) for (const t of p.targets) seasons.set(t.season, [...(seasons.get(t.season) ?? []), t.episode]);
  return [...seasons.entries()]
    .sort(([a], [b]) => Number(a === 0) - Number(b === 0) || a - b)
    .map(([season, episodes]) => ({ season, episodes: [...new Set(episodes)].sort((x, y) => x - y) }));
}

/** A collapsed group's peek: "S01 E29–E34, E36–E38 · S02 E01". */
export function seasonRangeSummary(proposals: Proposal[]): string {
  return bySeason(proposals.filter((p) => p.decision !== 'ignored'))
    .map(({ season, episodes }) => `S${pad(season)} ${episodeRanges(episodes)}`)
    .join(' · ');
}

// ---- Files ---------------------------------------------------------------------------------

/** A multi-file torrent's paths start with its name: the UI shows them without it. */
export function displayPath(path: string, torrentName: string): string {
  const prefix = `${torrentName}/`;
  return path.startsWith(prefix) ? path.slice(prefix.length) : path;
}

const QUALITY = /[. _-](?:\d{3,4}p|web(?:-?dl|rip)?|blu-?ray|bdrip|hdtv|x26[45]|h\.?26[45]|hevc|dvdrip|remux)\b.*$/i;

/**
 * A file's short name for notes and tags: "#35v2" (fansub numbering), "S00E02",
 * "#892" (absolute numbers), else its title without the show's name and the
 * quality tags ("The Waters of Mars Extended Edition").
 */
export function shortName(path: string, showTitle = ''): string {
  const base = (path.split('/').pop() ?? path).replace(/\.[a-z0-9]{2,4}$/i, '');
  const fansub = / - (.+?) [([]/.exec(base);
  if (fansub) return /^[\d.]+v?\d*$/.test(fansub[1]) ? `#${fansub[1]}` : fansub[1];
  const code = /S\d{2}E\d{2,4}/i.exec(base);
  if (code) return code[0].toUpperCase();
  // A bare number between separators that isn't a year: "One.Piece.892.1080p" → #892
  const absolute = [...base.matchAll(/[. _-](\d{2,4}(?:v\d)?)(?=[. _-])/g)].find((match) => !/^(19|20)\d\d$/.test(match[1]));
  if (absolute) return `#${absolute[1]}`;
  const words = base.replace(QUALITY, '').replace(/[._]+/g, ' ').trim();
  const titleWords = showTitle
    .split(':')[0]
    .replace(/[^\p{L}\p{N}\s]/gu, ' ')
    .split(/\s+/)
    .filter(Boolean);
  const showPrefix = titleWords.length ? new RegExp(`^${titleWords.join('[\\s.-]+')}(?:\\s+(?:19|20)\\d\\d)?\\s+`, 'i') : null;
  // "the The Waters of Mars file" reads badly in the notes: drop a leading article
  const short = (showPrefix ? words.replace(showPrefix, '') : words).replace(/^the\s+/i, '');
  return short || words || base;
}

const EXTRA_LABEL: Record<ExtraType, string> = {
  opening: 'Opening (creditless)',
  ending: 'Ending (creditless)',
  sample: 'Sample',
  trailer: 'Trailer',
  featurette: 'Featurette',
  font: 'Font',
  info: 'Info file',
  image: 'Image',
  superseded: 'Older version',
  other: 'Extra',
};

/** What an ignored row says it is: the extra's kind, "Not used" for a movie file you didn't pick, or "Ignored". */
export function ignoredLabel(p: Pick<Proposal, 'kind' | 'extra_type'>): string {
  if (p.extra_type) return EXTRA_LABEL[p.extra_type];
  if (p.kind === 'movie') return 'Not used';
  return p.kind === 'extra' ? EXTRA_LABEL.other : STATUS.ignored.label;
}

/** A file's name without its folders: labels for movie files (both cuts of a release share the title). */
export const fileName = (path: string): string => path.split('/').pop() ?? path;

/** A collapsed Extras group's peek: its first three kinds, each once ("Font, Sample", not "Font, Font, Font"). */
export function extrasPeek(proposals: Proposal[]): string {
  const kinds = proposals.map((p) => (p.decision !== 'ignored' && p.targets.length ? targetCode(p.targets) : ignoredLabel(p)));
  return [...new Set(kinds)].slice(0, 3).join(', ');
}

// ---- Actions -----------------------------------------------------------------------------------

/**
 * Undo and Include: back to the group's default (pending for Needs review and
 * Unmatched; accepted once the file has an episode elsewhere). An extra with no
 * episode becomes pending, so it can be picked.
 */
export function restoredDecision(p: Proposal): ProposalDecision {
  if (p.group === 'needs_review' || !maps(p)) return 'pending';
  return 'accepted';
}

/** Where a group starts (PLAN §6): matched and specials accepted, extras ignored, the rest pending. */
export const groupDefault = (group: ProposalGroup): ProposalDecision =>
  group === 'matched' || group === 'specials' ? 'accepted' : group === 'extras' ? 'ignored' : 'pending';

/**
 * Undo a movie pick: using a file ignored the other candidates, so every
 * candidate goes back to its group's default (matched accepted, Needs review
 * pending, extras ignored) and you choose again. Back on its default, a
 * candidate is no longer `user_decided`, so nothing blocks Apply that didn't before.
 */
export function movieUndo(proposals: Proposal[]): ProposalChange[] {
  return proposals.flatMap((p): ProposalChange[] => {
    if (p.kind !== 'movie') return [];
    const decision = groupDefault(p.group);
    return p.decision === decision ? [] : [{ id: p.id, decision }];
  });
}

/** Undo can't put a picked file back to "no episode" (the contract never empties targets): Change and Ignore only. */
export const canToggleAccept = (p: Proposal): boolean => maps(p) && !(p.group === 'unmatched' && p.decision === 'accepted');

/** Decision-only changes, applied locally while the PATCH is on its way (the answer replaces them). */
export function withDecisions(proposals: Proposal[], changes: ProposalChange[]): Proposal[] {
  const decisions = new Map(changes.filter((c) => c.decision && !c.episode_ids).map((c) => [c.id, c.decision as ProposalDecision]));
  if (decisions.size === 0) return proposals;
  const acceptedMovie = proposals.find((p) => p.kind === 'movie' && decisions.get(p.id) === 'accepted');
  return proposals.map((p) => {
    const decision = decisions.get(p.id);
    if (decision) return { ...p, decision };
    // A movie job maps one file: accepting a candidate ignores the others
    if (acceptedMovie && p.kind === 'movie') return { ...p, decision: 'ignored' };
    return p;
  });
}

// ---- Sticky foot ----------------------------------------------------------------------------

export type FootTone = 'conflict' | 'blocked' | 'info' | 'ok';

export interface FootState {
  /** "Apply 12 mappings". */
  label: string;
  toApply: number;
  /** Looks disabled (aria-disabled) but stays clickable: a click jumps to the first blocker. */
  blocked: boolean;
  tone: FootTone;
  message: string;
}

/** The message line, highest priority first: conflict, pending, unmatched, all clear. A movie job waits for one pick. */
export function footState(counts: ReviewCounts, { movie = false }: { movie?: boolean } = {}): FootState {
  const label = `Apply ${counts.toApply} ${counts.toApply === 1 ? 'mapping' : 'mappings'}`;
  const blocked = counts.pending > 0 || counts.conflicts > 0;
  const base = { label, toApply: counts.toApply, blocked };
  if (counts.conflicts > 0) return { ...base, tone: 'conflict', message: 'Two files point at the same episode. Keep one.' };
  if (counts.pending > 0 && movie) return { ...base, tone: 'blocked', message: 'Pick the file to use' };
  if (counts.pending > 0) return { ...base, tone: 'blocked', message: `Decide on ${plural(counts.pending, 'file')} to apply` };
  if (counts.unmatched > 0) {
    const one = counts.unmatched === 1;
    return { ...base, tone: 'info', message: `${plural(counts.unmatched, 'file')} ${one ? 'stays' : 'stay'} unmapped. You can map ${one ? 'it' : 'them'} later.` };
  }
  return { ...base, tone: 'ok', message: 'Everything has a place' };
}

/** Apply's 409 codes as the line under the button. */
export function applyErrorMessage(code: string, status?: JobStatus): string {
  switch (code) {
    case 'conflicting_proposals':
      return 'Two files point at the same episode. Keep one.';
    case 'pending_proposals':
      return 'Some files still need a decision.';
    case 'job_working':
      return 'Noir is still matching files. Try again in a moment.';
    case 'dry_run':
      return 'This is a preview, so it can’t be applied.';
    case 'job_closed':
      return status === 'cancelled' ? 'This mapping was discarded.' : 'This mapping is already closed.';
    default:
      return 'Couldn’t apply the mapping.';
  }
}

// ---- Applied ----------------------------------------------------------------------------------

export interface AppliedSeason {
  season: number;
  label: string;
  range: string;
  /** Episodes ready in that season. */
  count: number;
}

export interface AppliedSummary {
  /** Files that went to Infuse. */
  files: number;
  seasons: AppliedSeason[];
  ignored: number;
  unmapped: number;
  /** "9 ignored · 1 unmapped · still in the torrent, hidden from Infuse", or null when nothing was left out. */
  leftOut: string | null;
}

/** What Apply wrote: per season (Specials last), and what stayed out. */
export function appliedSummary(proposals: Proposal[]): AppliedSummary {
  const applied = proposals.filter((p) => p.decision === 'accepted' && maps(p));
  const ignored = proposals.filter((p) => p.decision === 'ignored').length;
  const unmapped = proposals.filter((p) => p.decision !== 'ignored' && !maps(p)).length;
  const parts = [ignored && `${ignored} ignored`, unmapped && `${unmapped} unmapped`].filter(Boolean);
  return {
    files: applied.length,
    seasons: bySeason(applied).map(({ season, episodes }) => ({
      season,
      label: season === 0 ? 'Specials' : `Season ${season}`,
      range: episodeRanges(episodes),
      count: episodes.length,
    })),
    ignored,
    unmapped,
    leftOut: parts.length ? `${parts.join(' · ')} · still in the torrent, hidden from Infuse` : null,
  };
}

/** The ready toast: "Frieren S01E29–E38 are ready" (regular episodes first; specials when that's all). */
export function readyToastTitle(show: string, proposals: Proposal[]): string {
  const targets = proposals
    .filter((p) => p.decision === 'accepted')
    .flatMap((p) => p.targets)
    .sort((a, b) => a.season - b.season || a.episode - b.episode);
  const regular = targets.filter((t) => t.season > 0);
  const shown = regular.length ? regular : targets;
  if (shown.length === 0) return `${show} is ready`;
  const first = shown[0];
  const last = shown[shown.length - 1];
  const one = shown.length === 1;
  const code = one
    ? episodeCode(first.season, first.episode)
    : first.season === last.season
      ? `${episodeCode(first.season, first.episode)}–E${pad(last.episode)}`
      : `${episodeCode(first.season, first.episode)}–${episodeCode(last.season, last.episode)}`;
  return `${show} ${code} ${one ? 'is' : 'are'} ready`;
}

// ---- Progress -----------------------------------------------------------------------------------

export interface ProgressStep {
  label: string;
  detail: string | null;
  state: 'done' | 'active' | 'pending';
}

const WORKING_STEP: Partial<Record<JobStatus, number>> = { queued: 0, fetching_metadata: 0, classifying: 1, resolving: 2 };

/**
 * The three named steps and their details (`job.progress`). `done` once the
 * job has stopped working; the AI step says "Skipped" when it never ran.
 */
export function progressSteps(job: Pick<AssignmentJob, 'status' | 'progress' | 'item'>, done: boolean): ProgressStep[] {
  const { seasons, episodes, files, videos, ai_files: aiFiles } = job.progress;
  const active = done ? 3 : (WORKING_STEP[job.status] ?? 3);
  const details = [
    seasons !== null && episodes !== null ? `${shortTitle(job.item.title)} · ${plural(seasons, 'season')}, ${plural(episodes, 'episode')}` : null,
    files !== null ? [plural(files, 'file'), videos !== null ? plural(videos, 'video') : null].filter(Boolean).join(' · ') : null,
    aiFiles !== null ? plural(aiFiles, 'file') : active > 2 ? 'Skipped' : null,
  ];
  return ['Fetching metadata', 'Matching files', 'Asking AI about the tricky ones'].map((label, i) => ({
    label,
    detail: details[i],
    state: i < active ? 'done' : i === active ? 'active' : 'pending',
  }));
}

/**
 * Which view the page shows. `progress` while the first run works, `done`
 * ("Done. Here's the plan.") for a moment after it, then `settled`; a re-check
 * (working again after the review showed) keeps the review.
 */
export type ViewStage = 'progress' | 'done' | 'settled';

export function nextStage(stage: ViewStage | null, status: JobStatus | undefined): ViewStage | null {
  if (status === undefined) return stage;
  if (status === 'failed' || status === 'cancelled') return 'settled';
  if (isJobWorking(status)) return stage === 'settled' || stage === 'done' ? stage : 'progress';
  return stage === 'progress' || stage === 'done' ? 'done' : 'settled';
}

// ---- The job ---------------------------------------------------------------------------------

/** "Frieren: Beyond Journey’s End" → "Frieren": the back link and buttons. */
export const shortTitle = (title: string): string => title.split(/:\s/)[0] || title;

/** An open job in Activity: "Frieren: 2 files to check", "Frieren: ready to apply", or "Frieren: mapping files…" while it works. */
export function openJobLine(job: Pick<AssignmentJob, 'status' | 'item' | 'counts'>): string {
  const show = shortTitle(job.item.title);
  if (isJobWorking(job.status)) return `${show}: mapping files…`;
  if (job.counts.needs_review > 0) return `${show}: ${plural(job.counts.needs_review, 'file')} to check`;
  return `${show}: ready to apply`;
}

/** The title page the job belongs to. */
export const titleHref = (job: Pick<AssignmentJob, 'item'>): string => `/${job.item.media_type === 'movie' ? 'movie' : 'tv'}/${job.item.tmdb_id}`;

/** "13.2 GB", "16 files", "Nyaa", "SubsPlease": what's known so far. */
export function torrentFacts(job: Pick<AssignmentJob, 'torrent'>, formatSize: (bytes: number) => string): string[] {
  const { size_bytes: size, file_count: files, indexer, release_group: group } = job.torrent;
  return [size !== null ? formatSize(size) : null, files !== null ? plural(files, 'file') : null, indexer, group].filter(
    (fact): fact is string => Boolean(fact)
  );
}
