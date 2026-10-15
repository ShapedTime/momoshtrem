import { http, HttpResponse } from 'msw';
import type {
  AssignHint,
  AssignmentJob,
  AssignmentJobDetail,
  JobAccepted,
  JobStatus,
  MediaType,
  ProposalChange,
  ShowEpisodesSeason,
  TorrentFileEntry,
} from '@/lib/api/contract';
import {
  autoApplies,
  CLOSED,
  conflictIds,
  finishRecheck,
  inScope,
  jobCounts,
  jobDetail,
  type MockJob,
  newJob,
  patchProposals,
  reviewOf,
  seedJobs,
  settledStatus,
  WORKING,
} from '../fixtures/assignment';
import { findTitle } from '../fixtures/catalog';
import { assignMovieRelease, episodeFile, findByLibraryId, libraryKey, showEpisodes } from '../fixtures/titles';
import { HOLD_STEPS, heldStep, scenario } from '../scenario';
import { mockAiSettings } from './settings';
import { mockLibrary } from './titles';
import { accepted, apiError, isRecord, jsonBody, latency, notFound } from './util';

/**
 * This page's mapping jobs (fixtures/assignment.ts has the data and rules).
 * A job walks its statuses as it's read; PATCH, re-check, apply and discard
 * stick until a reload, and every change updates the titles' review state.
 */
let jobs = seedJobs();
let nextId = 104;

/** Tests: back to the seeded jobs. */
export function resetMockJobs(): void {
  jobs = seedJobs();
  nextId = 104;
  syncReviews();
}

/** In the browser a step takes about 1.1 s, so the progress UI shows; in Node and Jest each read moves one step. */
const STEP_MS = 1100;
const instant = () => typeof window === 'undefined' || process.env.NODE_ENV === 'test';

const ALL: JobStatus[] = [...WORKING, 'needs_review', 'ready', ...CLOSED];
const OPEN: JobStatus[] = [...WORKING, 'needs_review', 'ready'];
const UNCLASSIFIED: JobStatus[] = ['queued', 'fetching_metadata', 'classifying'];

/** What the catalog says about every title in the library, from the open jobs. */
function syncReviews(): void {
  for (const row of mockLibrary().values()) row.review = reviewOf(jobs.values(), row.media_type, row.library_id);
}
syncReviews();

const now = () => new Date().toISOString();
const touch = (job: MockJob) => {
  job.updated_at = now();
};

const libraryRow = (mediaType: MediaType, libraryId: number | null) =>
  libraryId === null ? undefined : findByLibraryId(mockLibrary(), mediaType, libraryId);

function aiAvailable(mediaType: MediaType, libraryId: number | null): boolean {
  const mode = libraryRow(mediaType, libraryId)?.row.settings.classifier_mode;
  return scenario() !== 'ai-off' && mockAiSettings().classifier_enabled && mode !== 'rules_only';
}

/** A job whose title was removed is gone too (ON DELETE CASCADE). */
function findJob(id: unknown): MockJob | undefined {
  const job = jobs.get(Number(id));
  if (!job || (job.library_id !== null && !libraryRow(job.media_type, job.library_id))) return undefined;
  return job;
}

function detail(job: MockJob): AssignmentJobDetail {
  const entry = findTitle(job.media_type, job.tmdb_id);
  if (!entry) throw new Error(`No catalog entry for job ${job.id}`);
  return jobDetail(job, {
    entry,
    ai_available: aiAvailable(job.media_type, job.library_id),
    currentFile: (episodeId) => episodeFile(mockLibrary(), episodeId),
  });
}

// ---- Walking ----------------------------------------------------------------------

function fail(job: MockJob): void {
  job.status = 'failed';
  job.error = 'No peers sent the torrent’s file list within 60 s.';
  job.walk = null;
  job.proposals = [];
  touch(job);
}

/** The worker's writes: one transaction per job (PLAN §6 "Apply"). */
function applyJob(job: MockJob): void {
  const found = libraryRow(job.media_type, job.library_id);
  if (found && job.media_type === 'tv') {
    const { row } = found;
    // Authoritative for this torrent: its earlier files the job no longer maps go
    for (const [id, file] of row.filed) if (file.info_hash === job.info_hash) row.filed.delete(id);
    for (const p of job.proposals) {
      if (p.decision !== 'accepted') continue;
      for (const target of p.targets) {
        row.filed.set(target.episode_id, { info_hash: job.info_hash, release_name: job.name, file_path: p.file_path });
        row.unassigned.delete(target.episode_id);
      }
    }
  } else if (found) {
    const file = job.proposals.find((p) => p.kind === 'movie' && p.decision === 'accepted');
    if (file) {
      assignMovieRelease(found.row, {
        info_hash: job.info_hash,
        release_name: job.name,
        file_name: file.file_path.split('/').pop() ?? file.file_path,
        size_bytes: file.file_size,
      });
    }
  }
  job.status = 'applied';
  job.applied_at = now();
  touch(job);
}

/** Classification is done: settle the status, then apply on its own when nothing needs a look. */
function classified(job: MockJob): void {
  job.reached = 4;
  // Set here too: a poll can miss the short `resolving` step
  if (job.first_run_files > 0) job.ai_files = job.first_run_files;
  const picked = job.pick && job.proposals.find((p) => p.kind === 'movie' && (p.file_path === job.pick || p.file_path.endsWith(`/${job.pick}`)));
  if (picked) patchProposals(job, [{ id: picked.id, decision: 'accepted' }]);
  job.status = settledStatus(job.proposals);
  if (job.dry_run || job.status !== 'ready') return;
  if (picked || (mockAiSettings().auto_apply_high_confidence && autoApplies(job))) applyJob(job);
}

/** `job-hold:<step>`: the last step of this walk a held job may reach (-1: the hold doesn't apply). */
function holdIndex(steps: JobStatus[]): number {
  const hold = heldStep();
  if (!hold) return -1;
  const limit = HOLD_STEPS.indexOf(hold);
  return steps.reduce((last, status, i) => (HOLD_STEPS.findIndex((known) => known === status) <= limit ? i : last), -1);
}

/** Move a working job on: per read in Node, by the clock in the browser. */
function tick(job: MockJob): void {
  if (scenario() === 'job-failed' && !CLOSED.includes(job.status)) return fail(job);
  const walk = job.walk;
  if (!walk) return;
  walk.polls++;
  const held = holdIndex(walk.steps);
  const walked = instant() ? walk.polls : Math.floor((Date.now() - walk.started) / STEP_MS);
  const step = held >= 0 ? Math.min(walked, held) : walked;
  if (step < walk.steps.length) {
    const status = walk.steps[step];
    if (walk.kind === 'classify') job.reached = Math.max(job.reached, step);
    if (status === 'resolving' && walk.kind === 'classify') job.ai_files = job.first_run_files;
    if (status !== job.status) {
      job.status = status;
      touch(job);
    }
    return;
  }
  job.walk = null;
  if (walk.kind === 'classify') classified(job);
  else {
    // `ai-error`: the run fails and changes nothing
    if (scenario() === 'ai-error') job.ai_error = 'The AI didn’t answer in time, so nothing changed.';
    else finishRecheck(job, walk.scope ?? 'unresolved');
    job.status = settledStatus(job.proposals);
  }
  touch(job);
}

/** Read a job (GET only, so in Node each read is one step): walk it, then tell the titles. */
function read(job: MockJob): MockJob {
  tick(job);
  syncReviews();
  return job;
}

interface CreateJob {
  media_type: MediaType;
  tmdb_id: number;
  library_id: number | null;
  magnet_uri: string;
  info_hash: string;
  hint?: AssignHint;
  indexer?: string;
  pick?: string;
  dry_run?: boolean;
}

function create(input: CreateJob): MockJob {
  const job = newJob({ ...input, id: nextId++, ai: aiAvailable(input.media_type, input.library_id), now: Date.now() });
  jobs.set(job.id, job);
  return job;
}

/** Inspect (#6): a dry-run job for a release, whether or not the title is in the library. */
export function createInspectJob(mediaType: MediaType, tmdbId: number, magnetUri: string): number | undefined {
  const hash = infoHash(magnetUri);
  if (!hash || !findTitle(mediaType, tmdbId)) return undefined;
  const libraryId = mockLibrary().get(libraryKey(mediaType, tmdbId))?.library_id ?? null;
  return create({ media_type: mediaType, tmdb_id: tmdbId, library_id: libraryId, magnet_uri: magnetUri, info_hash: hash, dry_run: true }).id;
}

// ---- Requests -------------------------------------------------------------------

const infoHash = (magnet: string) => /[?&]xt=urn:btih:([^&]+)/i.exec(magnet)?.[1]?.toLowerCase();

const isHint = (value: unknown): value is AssignHint =>
  isRecord(value) &&
  (value.season === undefined || typeof value.season === 'number') &&
  (value.episode === undefined || typeof value.episode === 'number') &&
  (value.text === undefined || typeof value.text === 'string');

interface AssignBody {
  magnet_uri: string;
  hint?: AssignHint;
  indexer?: string;
  file_path?: string;
}

const isAssignBody = (body: unknown): body is AssignBody =>
  isRecord(body) &&
  typeof body.magnet_uri === 'string' &&
  (body.hint === undefined || isHint(body.hint)) &&
  (body.indexer === undefined || typeof body.indexer === 'string') &&
  (body.file_path === undefined || typeof body.file_path === 'string');

const DECISIONS = ['pending', 'accepted', 'ignored'];

/** Well-formed: an id and at least one change; `episode_ids` is never empty (to leave a file out, ignore it). */
const isChange = (item: unknown): item is ProposalChange =>
  isRecord(item) &&
  typeof item.id === 'number' &&
  (item.decision !== undefined || item.episode_ids !== undefined) &&
  (item.decision === undefined || DECISIONS.includes(String(item.decision))) &&
  (item.episode_ids === undefined ||
    (Array.isArray(item.episode_ids) && item.episode_ids.length > 0 && item.episode_ids.every((id) => typeof id === 'number')));

const isPatch = (body: unknown): body is { items: ProposalChange[] } =>
  isRecord(body) && Array.isArray(body.items) && body.items.length > 0 && body.items.every(isChange);

const isReclassify = (body: unknown): body is { scope: 'unresolved' | 'all'; hint?: string } =>
  isRecord(body) && (body.scope === 'unresolved' || body.scope === 'all') && (body.hint === undefined || typeof body.hint === 'string');

const noJob = () => notFound('No such mapping job');
const closed = (job: MockJob) => apiError(409, 'job_closed', `This mapping job is already ${job.status}.`);
const working = () => apiError(409, 'job_working', 'The files are still being matched.');

const MEDIA = [
  ['movie', 'movies'],
  ['tv', 'shows'],
] as const;

export const assignmentHandlers = [
  ...MEDIA.map(([mediaType, path]) =>
    http.post(`*/api/${path}/:id/assign-torrent`, async ({ params, request }) => {
      await latency();
      const found = findByLibraryId(mockLibrary(), mediaType, Number(params.id));
      if (!found) return notFound(mediaType === 'tv' ? 'No such show' : 'No such movie');
      const body = await jsonBody(request);
      if (!isAssignBody(body)) return apiError(400, 'bad_request', 'Send {magnet_uri, hint?, indexer?}.');
      const hash = infoHash(body.magnet_uri);
      if (!hash) return apiError(400, 'bad_request', 'The magnet link has no BitTorrent info hash.');
      // The same torrent for the same title: the job that's already open
      const open = [...jobs.values()].find(
        (job) => !job.dry_run && job.media_type === mediaType && job.library_id === found.row.library_id && job.info_hash === hash && !CLOSED.includes(job.status)
      );
      if (open) return HttpResponse.json<JobAccepted>({ job_id: open.id });
      const job = create({
        media_type: mediaType,
        tmdb_id: found.entry.tmdb_id,
        library_id: found.row.library_id,
        magnet_uri: body.magnet_uri,
        info_hash: hash,
        hint: body.hint,
        indexer: body.indexer,
        pick: mediaType === 'movie' ? body.file_path : undefined,
      });
      return accepted({ job_id: job.id });
    })
  ),
  http.get('*/api/assignment-jobs', async ({ request }) => {
    await latency();
    const param = new URL(request.url).searchParams.get('status') ?? 'open';
    const statuses = param === 'open' ? OPEN : param.split(',');
    if (statuses.some((status) => !ALL.some((known) => known === status))) return apiError(400, 'bad_request', `Unknown status in "${param}".`);
    const items: AssignmentJob[] = [...jobs.values()]
      .filter((job) => !job.dry_run && findJob(job.id))
      .map(read)
      .filter((job) => statuses.includes(job.status))
      .sort((a, b) => b.id - a.id)
      .map((job) => detail(job).job);
    const body: { items: AssignmentJob[] } = { items };
    return HttpResponse.json(body);
  }),
  http.get('*/api/assignment-jobs/:id', async ({ params }) => {
    await latency();
    const job = findJob(params.id);
    return job ? HttpResponse.json(detail(read(job))) : noJob();
  }),
  http.patch('*/api/assignment-jobs/:id/proposals', async ({ params, request }) => {
    await latency();
    const job = findJob(params.id);
    if (!job) return noJob();
    const body = await jsonBody(request);
    if (!isPatch(body)) return apiError(400, 'bad_request', 'Send {items: [{id, decision?, episode_ids?}]}, each with a decision or a non-empty episode_ids.');
    if (CLOSED.includes(job.status)) return closed(job);
    if (UNCLASSIFIED.includes(job.status)) return working();
    const error = patchProposals(job, body.items);
    if (error) return apiError(422, 'validation_error', error);
    // During a re-check the status settles when the AI is done
    if (job.status !== 'resolving') job.status = settledStatus(job.proposals);
    touch(job);
    syncReviews();
    return HttpResponse.json(detail(job));
  }),
  http.post('*/api/assignment-jobs/:id/reclassify', async ({ params, request }) => {
    await latency();
    const job = findJob(params.id);
    if (!job) return noJob();
    if (CLOSED.includes(job.status)) return closed(job);
    if (WORKING.includes(job.status)) return apiError(409, 'job_working', 'Noir is still on it.');
    if (!aiAvailable(job.media_type, job.library_id)) return apiError(503, 'unavailable', 'AI isn’t available for this job.');
    const body = await jsonBody(request);
    if (!isReclassify(body)) return apiError(400, 'bad_request', 'Send {scope: "unresolved" | "all", hint?}.');
    // The re-check's hint replaces the text one; season and episode stay
    const hint: AssignHint = { ...job.hint };
    delete hint.text;
    if (body.hint) hint.text = body.hint;
    job.hint = Object.keys(hint).length ? hint : null;
    job.ai_error = null;
    job.proposals = job.proposals.map((p) => ({ ...p, updated: false, hinted: false }));
    const conflicts = conflictIds(job.proposals);
    job.ai_files = job.proposals.filter((p) => inScope(p, body.scope, conflicts)).length;
    job.status = 'resolving';
    job.walk = { kind: 'recheck', steps: ['resolving', 'resolving'], started: Date.now(), polls: 0, scope: body.scope };
    touch(job);
    syncReviews();
    return accepted({ job_id: job.id });
  }),
  http.post('*/api/assignment-jobs/:id/apply', async ({ params }) => {
    await latency();
    const job = findJob(params.id);
    if (!job) return noJob();
    if (job.dry_run) return apiError(409, 'dry_run', 'A dry run is never applied.');
    if (CLOSED.includes(job.status)) return closed(job);
    if (WORKING.includes(job.status)) return working();
    const counts = jobCounts(job.proposals);
    if (counts.conflicts > 0) return apiError(409, 'conflicting_proposals', 'Two files point at the same episode. Keep one.');
    if (counts.pending > 0) return apiError(409, 'pending_proposals', `Decide on ${counts.pending} file${counts.pending === 1 ? '' : 's'} first.`);
    applyJob(job);
    syncReviews();
    return HttpResponse.json(detail(job));
  }),
  http.post('*/api/assignment-jobs/:id/discard', async ({ params }) => {
    await latency();
    const job = findJob(params.id);
    if (!job) return noJob();
    if (job.status === 'applied' || job.status === 'failed') return closed(job);
    if (job.status !== 'cancelled') {
      job.status = 'cancelled';
      job.walk = null;
      touch(job);
      syncReviews();
    }
    return HttpResponse.json(detail(job));
  }),
  http.get('*/api/shows/:id/episodes', async ({ params }) => {
    await latency();
    const found = findByLibraryId(mockLibrary(), 'tv', Number(params.id));
    if (!found) return notFound('No such show');
    const body: { items: ShowEpisodesSeason[] } = { items: showEpisodes(found.entry.tmdb_id, mockLibrary()) };
    return HttpResponse.json(body);
  }),
  http.put('*/api/episodes/:id/assignment', () => new HttpResponse(null, { status: 204 })),
  http.put('*/api/movies/:id/assignment', () => new HttpResponse(null, { status: 204 })),
  http.get('*/api/torrents/:hash/files', async ({ params }) => {
    await latency();
    const hash = String(params.hash).toLowerCase();
    const job = [...jobs.values()].filter((j) => j.info_hash === hash).sort((a, b) => b.id - a.id)[0];
    if (!job) return notFound('No such torrent');
    const items = job.proposals.map((p): TorrentFileEntry => {
      const mapped = job.status === 'applied' && p.decision === 'accepted';
      const movie = p.kind === 'movie' && job.library_id !== null ? { movie_id: job.library_id } : null;
      return { path: p.file_path, size: p.file_size, mapped_to: !mapped ? null : (movie ?? (p.targets.length ? { episodes: p.targets } : null)) };
    });
    const body: { items: TorrentFileEntry[] } = { items };
    return HttpResponse.json(body);
  }),
];
