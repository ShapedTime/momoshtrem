/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { setMockScenario } from '@/mocks/scenario';
import { episodeId } from '@/mocks/fixtures/titles';
import { QueryClient } from '@tanstack/react-query';
import { assignmentApi, assignmentJobQuery, invalidateAfterJob, searchApi, settingsApi, titlesApi } from '@/lib/api/endpoints';
import type { AssignmentJobDetail, JobStatus, Proposal } from '@/lib/api/contract';

/**
 * The stateful mapping-job mock (PLAN §6 "Assignment"; #5): the seeded jobs
 * agree with the show pages, and PATCH, re-check, apply and discard behave as
 * the contract says. In Node a job moves one step per read.
 */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(resetMockState);
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const FRIEREN = 209867;
const ONE_PIECE = 37854;
const DOCTOR_WHO = 57243;

const code = (p: Proposal) => p.targets.map((t) => `S${String(t.season).padStart(2, '0')}E${String(t.episode).padStart(2, '0')}`).join('+');
const named = (detail: AssignmentJobDetail, part: string) => {
  const found = detail.proposals.find((p) => p.file_path.includes(part));
  if (!found) throw new Error(`No proposal for ${part}`);
  return found;
};
const reviewEpisodes = async (tmdbId: number, season: number) =>
  (await titlesApi.season(tmdbId, season)).episodes.filter((e) => e.state === 'needs_review').map((e) => e.episode_number);

async function statuses(jobId: number, reads: number): Promise<JobStatus[]> {
  const seen: JobStatus[] = [];
  for (let i = 0; i < reads; i++) seen.push((await assignmentApi.job(jobId)).job.status);
  return seen;
}

describe('seeded jobs', () => {
  it('serves the design’s Frieren job: 16 files in five groups, two to review', async () => {
    const detail = await assignmentApi.job(101);
    expect(detail.job).toMatchObject({
      status: 'needs_review',
      dry_run: false,
      item: { id: 1, media_type: 'tv', tmdb_id: FRIEREN, title: 'Frieren: Beyond Journey’s End', year: 2023 },
      // The design's "16 files · 13 videos": openings and endings are extras
      torrent: { name: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]', file_count: 16, video_count: 13, release_group: 'SubsPlease', indexer: 'Nyaa' },
      progress: { seasons: 2, episodes: 44, files: 16, videos: 13, ai_files: 3 },
      ai_available: true,
      ai_error: null,
      suggested_hints: ['these are absolute numbers', '28.5 is a recap', 'v2 replaces v1'],
      counts: { files: 16, to_apply: 12, pending: 2, conflicts: 0, needs_review: 2, matched: 9, specials: 1, ignored: 3, unmatched: 1 },
    });
    // Review order, then by target (Specials after regular seasons)
    expect(detail.proposals.map((p) => `${p.group}:${code(p)}`)).toEqual([
      'needs_review:S01E35',
      'needs_review:S00E03',
      'unmatched:',
      'specials:S00E06',
      ...[29, 30, 31, 32, 33, 34, 36, 37, 38].map((n) => `matched:S01E${n}`),
      'extras:',
      'extras:',
      'extras:',
    ]);
    expect(named(detail, '35v2')).toMatchObject({
      kind: 'episode',
      targets: [{ episode_id: episodeId(FRIEREN, 1, 35), season: 1, episode: 35, absolute_number: 35, name: 'The Hero of the South' }],
      confidence: 'medium',
      source: 'ai',
      decision: 'pending',
      user_decided: false,
      replaces: null,
    });
    expect(named(detail, '28.5')).toMatchObject({ targets: [{ season: 0, episode: 3, absolute_number: null, name: 'Recap: The Journey So Far' }], confidence: 'low' });
    expect(named(detail, ' - 29 ')).toMatchObject({ decision: 'accepted', confidence: 'high', source: 'rules', reason: 'Absolute #29 → S01E29 (single-season show)' });
    expect(detail.proposals.filter((p) => p.group === 'extras').map((p) => [p.kind, p.extra_type, p.decision])).toEqual([
      ['extra', 'font', 'ignored'],
      ['extra', 'ending', 'ignored'],
      ['extra', 'opening', 'ignored'],
    ]);
    expect(named(detail, 'Fonts/').file_path).toBe('[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]/Fonts/OpenSans-Semibold.ttf');
  });

  it('keeps reasons short and leaves the "AI:" label to the UI', async () => {
    const details = await Promise.all([101, 102, 103].map((id) => assignmentApi.job(id)));
    const reasons = details.flatMap((d) => d.proposals.map((p) => p.reason));
    expect(reasons.filter((reason) => [...reason].length > 120 || /^AI\b/.test(reason))).toEqual([]);
  });

  it('agrees with the show pages: banner counts, review jobs, and review only where there’s no file', async () => {
    const details = await Promise.all([101, 102, 103].map((id) => assignmentApi.job(id)));
    const titles = await Promise.all([FRIEREN, ONE_PIECE, DOCTOR_WHO].map((id) => titlesApi.get('tv', id)));
    expect(details.map((d) => d.job.counts.needs_review)).toEqual([2, 1, 3]);
    expect(titles.map((t) => [t.library.needs_review, t.library.review_job_id])).toEqual([
      [2, 101],
      [1, 102],
      [3, 103],
    ]);
    expect(await reviewEpisodes(FRIEREN, 1)).toEqual([35]);
    expect(await reviewEpisodes(FRIEREN, 0)).toEqual([3]);
    expect(await reviewEpisodes(ONE_PIECE, 21)).toEqual([1]);
    // Doctor Who's files to review all target specials that have a file (S00E01, S00E06): no review rows
    expect(await reviewEpisodes(DOCTOR_WHO, 0)).toEqual([]);
    // Frieren still needs 29–38: the episode under review has no file yet
    expect(titles[0].library.missing).toMatchObject({ from: 29, to: 38, count: 10 });
  });

  it('Doctor Who: two files want S00E06, which already has a file', async () => {
    const detail = await assignmentApi.job(103);
    const mars = detail.proposals.filter((p) => code(p) === 'S00E06');
    expect(mars.map((p) => [p.group, p.decision])).toEqual([
      ['needs_review', 'pending'],
      ['needs_review', 'pending'],
    ]);
    expect(mars.map((p) => p.replaces)).toEqual([
      expect.objectContaining({ release_name: 'Doctor.Who.2005.S01-S04.1080p.BluRay.x264-SHORTBREHD' }),
      expect.objectContaining({ release_name: 'Doctor.Who.2005.S01-S04.1080p.BluRay.x264-SHORTBREHD' }),
    ]);
    expect(detail.job.counts).toMatchObject({ conflicts: 2, pending: 3, needs_review: 3 });
  });

  it('lists open jobs newest first, and filters by status', async () => {
    expect((await assignmentApi.jobs()).items.map((job) => job.id)).toEqual([103, 102, 101]);
    expect((await assignmentApi.jobs(['applied', 'cancelled'])).items).toEqual([]);
    expect((await fetch('/api/assignment-jobs?status=nope')).status).toBe(400);
  });
});

describe('a new job', () => {
  it('walks one step per read, then stops for review; the same torrent gets the same job', async () => {
    const magnet = 'magnet:?xt=urn:btih:ABC123&dn=Frieren.29-38.Batch';
    const { job_id } = await assignmentApi.assignShow(1, { magnet_uri: magnet, hint: { season: 1, text: 'these are absolute numbers' }, indexer: 'Nyaa' });
    expect(job_id).toBe(104);
    const first = await assignmentApi.job(job_id);
    expect(first.job).toMatchObject({ status: 'fetching_metadata', hint: { season: 1, text: 'these are absolute numbers' }, torrent: { info_hash: 'abc123', name: 'Frieren.29-38.Batch', file_count: null } });
    expect(first.proposals).toEqual([]);
    expect(await statuses(job_id, 4)).toEqual(['classifying', 'resolving', 'needs_review', 'needs_review']);
    const done = await assignmentApi.job(job_id);
    expect(done.proposals).toHaveLength(16);
    expect(done.proposals.every((p) => p.file_path.startsWith('Frieren.29-38.Batch/'))).toBe(true);
    expect(await assignmentApi.assignShow(1, { magnet_uri: magnet })).toEqual({ job_id: 104 });
    // Now two open jobs: the title points at the newest
    expect((await titlesApi.get('tv', FRIEREN)).library).toMatchObject({ needs_review: 4, review_job_id: 104 });
  });

  it('a season pack that maps cleanly applies on its own', async () => {
    const show = await titlesApi.addShow(66573); // The Good Place: nothing assigned yet
    const { job_id } = await assignmentApi.assignShow(show.id, { magnet_uri: 'magnet:?xt=urn:btih:goodplace4' });
    // No AI step: nothing needs asking
    expect(await statuses(job_id, 3)).toEqual(['fetching_metadata', 'classifying', 'applied']);
    const season = await titlesApi.season(66573, 4);
    expect(new Set(season.episodes.map((e) => e.state))).toEqual(new Set(['ready']));
  });

  it('discarded before it’s classified, it has no proposals', async () => {
    const { job_id } = await assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:early' });
    expect((await assignmentApi.job(job_id)).job.status).toBe('fetching_metadata');
    const discarded = await assignmentApi.discard(job_id);
    expect(discarded.job).toMatchObject({ status: 'cancelled', counts: { files: 0 } });
    expect(discarded.proposals).toEqual([]);
  });

  it('answers 400 for a magnet without an info hash and 404 for a show not in the library', async () => {
    await expect(assignmentApi.assignShow(1, { magnet_uri: 'magnet:?dn=nothing' })).rejects.toMatchObject({ status: 400, code: 'bad_request' });
    await expect(assignmentApi.assignShow(99, { magnet_uri: 'magnet:?xt=urn:btih:abc' })).rejects.toMatchObject({ status: 404 });
  });

  it('movies: no AI; accepting one cut ignores the other, Undo brings both back, and applying assigns it', async () => {
    const release = 'One.Battle.After.Another.2025.1080p.BluRay.x264-SPARKS';
    const { job_id } = await assignmentApi.assignMovie(3, { magnet_uri: `magnet:?xt=urn:btih:oba1&dn=${release}` });
    // The resolver maps episodes: a movie job never asks the AI
    expect(await statuses(job_id, 3)).toEqual(['fetching_metadata', 'classifying', 'needs_review']);
    const detail = await assignmentApi.job(job_id);
    expect(detail.job).toMatchObject({ ai_available: false, suggested_hints: [], progress: { ai_files: null } });
    await expect(assignmentApi.reclassify(job_id, { scope: 'all' })).rejects.toMatchObject({ status: 503 });
    const cuts = detail.proposals.filter((p) => p.kind === 'movie');
    expect(cuts.map((p) => [p.group, p.targets, p.decision])).toEqual([
      ['needs_review', [], 'pending'],
      ['needs_review', [], 'pending'],
    ]);
    await expect(assignmentApi.patchProposals(job_id, { items: [{ id: cuts[0].id, episode_ids: [1] }] })).rejects.toMatchObject({ status: 422 });
    // Two accepted in one batch: the first wins
    const both = await assignmentApi.patchProposals(job_id, {
      items: [
        { id: cuts[1].id, decision: 'accepted' },
        { id: cuts[0].id, decision: 'accepted' },
      ],
    });
    expect(both.proposals.filter((p) => p.kind === 'movie').map((p) => p.decision)).toEqual(['ignored', 'accepted']);
    const picked = await assignmentApi.patchProposals(job_id, { items: [{ id: cuts[1].id, decision: 'accepted' }] });
    expect(picked.proposals.filter((p) => p.kind === 'movie').map((p) => p.decision)).toEqual(['ignored', 'accepted']);
    expect(picked.job.status).toBe('ready');
    // Undo is one PATCH that puts every candidate back to pending, so you choose again
    const undone = await assignmentApi.patchProposals(job_id, { items: cuts.map((p) => ({ id: p.id, decision: 'pending' as const })) });
    expect(undone.proposals.filter((p) => p.kind === 'movie').map((p) => [p.decision, p.user_decided])).toEqual([
      ['pending', false],
      ['pending', false],
    ]);
    expect(undone.job.status).toBe('needs_review');
    await assignmentApi.patchProposals(job_id, { items: [{ id: cuts[0].id, decision: 'accepted' }] });
    await assignmentApi.apply(job_id);
    expect((await titlesApi.get('movie', 1054867)).library).toMatchObject({
      availability_stage: 'downloading',
      torrent: { release_name: release, file_name: `${release}.mkv` },
    });
  });

  it('movies: one plausible file applies on its own', async () => {
    const release = 'One.Battle.After.Another.2025.2160p.WEB-DL';
    const { job_id } = await assignmentApi.assignMovie(3, { magnet_uri: `magnet:?xt=urn:btih:oba2&dn=${release}` });
    expect(await statuses(job_id, 3)).toEqual(['fetching_metadata', 'classifying', 'applied']);
    expect((await assignmentApi.job(job_id)).proposals.map((p) => [p.kind, p.group, p.decision])).toEqual([
      ['movie', 'matched', 'accepted'],
      ['extra', 'extras', 'ignored'],
    ]);
    expect((await titlesApi.get('movie', 1054867)).library).toMatchObject({ availability_stage: 'downloading', torrent: { release_name: release } });
  });

  it('Inspect makes a dry run: same shape, never applied, not on the title', async () => {
    const { job_id } = await searchApi.inspect({ release_id: 'rel-0', media_type: 'tv', tmdb_id: FRIEREN });
    await statuses(job_id, 4);
    const detail = await assignmentApi.job(job_id);
    expect(detail.job).toMatchObject({ dry_run: true, status: 'needs_review', torrent: { name: 'Frieren.Beyond.Journeys.End.S01E29.2160p.WEB-DL.DV.H.265-SubsPlease' } });
    await expect(assignmentApi.apply(job_id)).rejects.toMatchObject({ status: 409, code: 'dry_run' });
    expect((await assignmentApi.jobs()).items.map((job) => job.id)).not.toContain(job_id);
    expect((await titlesApi.get('tv', FRIEREN)).library).toMatchObject({ needs_review: 2, review_job_id: 101 });
  });
});

describe('reviewing', () => {
  it('PATCH accepts, picks another episode and undoes; status and counts follow', async () => {
    const before = await assignmentApi.job(101);
    const v2 = named(before, '35v2');
    const recap = named(before, '28.5');

    let detail = await assignmentApi.patchProposals(101, { items: [{ id: v2.id, decision: 'accepted' }] });
    expect(named(detail, '35v2')).toMatchObject({ decision: 'accepted', user_decided: true, source: 'ai', group: 'needs_review' });
    expect(detail.job).toMatchObject({ status: 'needs_review', counts: { pending: 1, needs_review: 1 } });

    detail = await assignmentApi.patchProposals(101, { items: [{ id: recap.id, episode_ids: [episodeId(FRIEREN, 0, 1)], decision: 'accepted' }] });
    expect(named(detail, '28.5')).toMatchObject({
      targets: [{ season: 0, episode: 1, name: '“Frieren” Mini Anime 1' }],
      suggested_targets: recap.targets,
      source: 'manual',
      confidence: null,
      reason: 'You picked this episode',
      user_decided: true,
      group: 'needs_review',
      // S00E01 already has a file
      replaces: expect.objectContaining({ release_name: '[SubsPlease] Sousou no Frieren (01-28) (1080p) [Batch]' }),
    });
    expect(detail.job).toMatchObject({ status: 'ready', counts: { pending: 0, needs_review: 0, to_apply: 12 } });
    // Nothing left to review, but the ready job stays the title's review job until it's applied
    expect((await titlesApi.get('tv', FRIEREN)).library).toMatchObject({ needs_review: 0, review_job_id: 101 });

    // Undo: back on the suggestion, pending, no longer the user's
    detail = await assignmentApi.patchProposals(101, { items: [{ id: recap.id, episode_ids: recap.suggested_targets.map((t) => t.episode_id), decision: 'pending' }] });
    expect(named(detail, '28.5')).toEqual(recap);
    expect(detail.job.status).toBe('needs_review');
  });

  it('PATCH is all or nothing: 400 for a malformed body, 422 for a change that can’t apply', async () => {
    const detail = await assignmentApi.job(101);
    const v2 = named(detail, '35v2');
    const unmatched = named(detail, 'Special Extra');
    // An item with neither field, an empty episode_ids (leave a file out by ignoring it), no items
    for (const items of [[{ id: v2.id }], [{ id: v2.id, episode_ids: [] }], []]) {
      await expect(assignmentApi.patchProposals(101, { items })).rejects.toMatchObject({ status: 400, code: 'bad_request' });
    }
    // An unknown proposal, another show's episode, accepting without a target
    const invalid = [
      [{ id: v2.id, decision: 'accepted' as const }, { id: 1, decision: 'accepted' as const }],
      [{ id: v2.id, episode_ids: [episodeId(DOCTOR_WHO, 0, 5)] }],
      [{ id: unmatched.id, decision: 'accepted' as const }],
    ];
    for (const items of invalid) {
      await expect(assignmentApi.patchProposals(101, { items })).rejects.toMatchObject({ status: 422, code: 'validation_error' });
    }
    expect(named(await assignmentApi.job(101), '35v2').decision).toBe('pending');
    const reclassify = await fetch('/api/assignment-jobs/101/reclassify', { method: 'POST', body: JSON.stringify({ scope: 'some' }) });
    expect(reclassify.status).toBe(400);
  });

  it('conflicts block Apply until one file is ignored, then Doctor Who’s specials are ready', async () => {
    const detail = await assignmentApi.job(103);
    const easter = named(detail, 'Easter.Special');
    const [mars, extended] = detail.proposals.filter((p) => code(p) === 'S00E06');
    await expect(assignmentApi.apply(103)).rejects.toMatchObject({ status: 409, code: 'conflicting_proposals' });
    await assignmentApi.patchProposals(103, {
      items: [
        { id: easter.id, episode_ids: [episodeId(DOCTOR_WHO, 0, 5)], decision: 'accepted' },
        { id: mars.id, decision: 'accepted' },
      ],
    });
    await expect(assignmentApi.apply(103)).rejects.toMatchObject({ status: 409, code: 'conflicting_proposals' });
    const resolved = await assignmentApi.patchProposals(103, { items: [{ id: extended.id, decision: 'ignored' }] });
    expect(resolved.job).toMatchObject({ status: 'ready', counts: { conflicts: 0, pending: 0 } });

    const applied = await assignmentApi.apply(103);
    expect(applied.job.status).toBe('applied');
    expect(applied.job.applied_at).not.toBeNull();
    // The files are this job's now: nothing left to replace
    expect(named(applied, 'S00E02.The.Waters.of.Mars').replaces).toBeNull();
    const specials = (await titlesApi.season(DOCTOR_WHO, 0)).episodes;
    expect(specials.map((e) => [e.episode_number, e.state])).toEqual([
      [1, 'watched'],
      [2, 'watched'],
      [3, 'watched'],
      [4, 'ready'],
      [5, 'ready'],
      [6, 'ready'],
      [7, 'ready'],
      [8, 'ready'],
    ]);
    expect((await titlesApi.get('tv', DOCTOR_WHO)).library).toMatchObject({ needs_review: 0, review_job_id: null });
    await expect(assignmentApi.apply(103)).rejects.toMatchObject({ status: 409, code: 'job_closed' });
  });

  it('Apply refuses pending files, then Frieren is ready and its banner is gone', async () => {
    await expect(assignmentApi.apply(101)).rejects.toMatchObject({ status: 409, code: 'pending_proposals' });
    const { proposals } = await assignmentApi.job(101);
    const pending = proposals.filter((p) => p.decision === 'pending' && p.targets.length > 0);
    await assignmentApi.patchProposals(101, { items: pending.map((p) => ({ id: p.id, decision: 'accepted' as const })) });
    // The unmatched file never blocks
    expect((await assignmentApi.apply(101)).job).toMatchObject({ status: 'applied', counts: { unmatched: 1 } });
    const title = await titlesApi.get('tv', FRIEREN);
    expect(title.library).toMatchObject({ availability_stage: 'ready', missing: null, needs_review: 0, review_job_id: null });
    const season = await titlesApi.season(FRIEREN, 1);
    expect(season.episodes.filter((e) => e.episode_number >= 29).map((e) => e.state)).toEqual(Array(10).fill('ready'));
    expect(season.episodes.find((e) => e.episode_number === 35)?.webdav_path).toContain('S01E35');
    expect((await assignmentApi.jobs()).items.map((job) => job.id)).toEqual([103, 102]);
    const { items } = await assignmentApi.torrentFiles((await assignmentApi.job(101)).job.torrent.info_hash);
    expect(items.find((file) => file.path.includes(' - 29 '))?.mapped_to).toEqual({ episodes: [expect.objectContaining({ season: 1, episode: 29 })] });
  });

  it('Discard cancels the job, and the show’s review goes with it', async () => {
    expect((await assignmentApi.discard(102)).job.status).toBe('cancelled');
    expect((await titlesApi.get('tv', ONE_PIECE)).library).toMatchObject({ needs_review: 0, review_job_id: null });
    expect(await reviewEpisodes(ONE_PIECE, 21)).toEqual([]);
    expect((await assignmentApi.discard(102)).job.status).toBe('cancelled');
    await expect(assignmentApi.apply(102)).rejects.toMatchObject({ status: 409, code: 'job_closed' });
    await expect(assignmentApi.patchProposals(102, { items: [{ id: 102001, decision: 'accepted' }] })).rejects.toMatchObject({ status: 409, code: 'job_closed' });
  });

  it('re-check with AI: resolving, then updated suggestions; what the user decided stays', async () => {
    const before = await assignmentApi.job(101);
    const v2 = named(before, '35v2');
    await assignmentApi.patchProposals(101, { items: [{ id: v2.id, decision: 'accepted' }] });
    expect(await assignmentApi.reclassify(101, { scope: 'unresolved', hint: '28.5 is a recap' })).toEqual({ job_id: 101 });

    const working = await assignmentApi.job(101);
    // 28.5 and the unmatched file: pending and not decided by the user
    expect(working.job).toMatchObject({ status: 'resolving', progress: { ai_files: 2 }, hint: { text: '28.5 is a recap' } });
    await expect(assignmentApi.reclassify(101, { scope: 'all' })).rejects.toMatchObject({ status: 409, code: 'job_working' });

    const done = await assignmentApi.job(101);
    expect(done.job.status).toBe('needs_review');
    expect(named(done, '28.5')).toMatchObject({ confidence: 'high', source: 'ai', updated: true, hinted: true, decision: 'pending', group: 'needs_review' });
    expect(named(done, 'Special Extra')).toMatchObject({ targets: [], updated: true, group: 'unmatched' });
    expect(named(done, '35v2')).toMatchObject({ confidence: 'medium', updated: false, decision: 'accepted' });
    expect(named(done, '28.5').reason).not.toBe(named(before, '28.5').reason);
  });

  it('re-check scopes are the backend’s: undecided episode files, pending or in a conflict for unresolved', async () => {
    // Frieren: 13 episode files (extras keep the rules' reading); the hint is trimmed
    await assignmentApi.reclassify(101, { scope: 'all', hint: '  these are absolute numbers  ' });
    expect((await assignmentApi.job(101)).job).toMatchObject({ status: 'resolving', progress: { ai_files: 13 }, hint: { text: 'these are absolute numbers' } });
    // Doctor Who: the Easter special, the two Waters of Mars files and the unmatched video; not the .nfo
    await assignmentApi.reclassify(103, { scope: 'unresolved' });
    expect((await assignmentApi.job(103)).job.progress.ai_files).toBe(4);
  });

  it('a re-check never overrides a high rules answer: the dispute keeps its episode and goes back to pending', async () => {
    const before = named(await assignmentApi.job(102), '/One.Piece.897.');
    expect(before).toMatchObject({ group: 'matched', decision: 'accepted', confidence: 'high', source: 'rules' });
    await assignmentApi.reclassify(102, { scope: 'all' });
    expect(await statuses(102, 2)).toEqual(['resolving', 'needs_review']);
    const disputed = named(await assignmentApi.job(102), '/One.Piece.897.');
    expect(disputed).toMatchObject({
      targets: before.targets,
      suggested_targets: before.targets,
      confidence: 'medium',
      source: 'rules',
      reason: 'AI suggests S21E07: TMDB’s air order puts #897 a week later',
      decision: 'pending',
      group: 'matched',
      user_decided: false,
      updated: true,
    });
  });

  it('a re-check with nothing in scope settles at once, without resolving', async () => {
    const detail = await assignmentApi.job(102);
    const pending = detail.proposals.filter((p) => p.decision === 'pending');
    await assignmentApi.patchProposals(102, { items: pending.map((p) => ({ id: p.id, decision: 'accepted' as const })) });
    expect(await assignmentApi.reclassify(102, { scope: 'unresolved' })).toEqual({ job_id: 102 });
    expect((await assignmentApi.job(102)).job).toMatchObject({ status: 'ready', progress: { ai_files: 0 } });
  });

  it('reclassify checks the body first: 400 even on a closed job, then 409', async () => {
    await assignmentApi.discard(101);
    const bad = await fetch('/api/assignment-jobs/101/reclassify', { method: 'POST', body: JSON.stringify({ scope: 'some' }) });
    expect(bad.status).toBe(400);
    const long = await fetch('/api/assignment-jobs/101/reclassify', { method: 'POST', body: JSON.stringify({ scope: 'all', hint: 'x'.repeat(501) }) });
    expect(long.status).toBe(400);
    await expect(assignmentApi.reclassify(101, { scope: 'all' })).rejects.toMatchObject({ status: 409, code: 'job_closed' });
  });

  it('a re-check can move a target, and Undo then goes back to the AI’s new suggestion', async () => {
    const before = named(await assignmentApi.job(103), 'Easter.Special');
    expect(before).toMatchObject({ targets: [{ season: 0, episode: 1 }], confidence: 'low', replaces: expect.objectContaining({ file_path: expect.stringContaining('S00E01') }) });
    await assignmentApi.reclassify(103, { scope: 'unresolved' });
    expect(await statuses(103, 2)).toEqual(['resolving', 'needs_review']);
    const moved = named(await assignmentApi.job(103), 'Easter.Special');
    const planet = { episode_id: episodeId(DOCTOR_WHO, 0, 5), season: 0, episode: 5, absolute_number: null, name: 'Planet of the Dead' };
    expect(moved).toMatchObject({ targets: [planet], suggested_targets: [planet], confidence: 'high', source: 'ai', updated: true, hinted: false, replaces: null });
    // S00E05 has no file: now it's under review
    expect(await reviewEpisodes(DOCTOR_WHO, 0)).toEqual([5]);

    await assignmentApi.patchProposals(103, { items: [{ id: moved.id, episode_ids: [episodeId(DOCTOR_WHO, 0, 4)], decision: 'accepted' }] });
    const undone = await assignmentApi.patchProposals(103, { items: [{ id: moved.id, episode_ids: [planet.episode_id], decision: 'pending' }] });
    expect(named(undone, 'Easter.Special')).toEqual(moved);
  });

  it('GET /shows/{id}/episodes lists every season, Specials included even while they’re off', async () => {
    await titlesApi.patchShow(1, { include_specials: false });
    const { items } = await assignmentApi.showEpisodes(1);
    expect(items.map((s) => [s.season_number, s.episodes.length])).toEqual([
      [0, 6],
      [1, 38],
    ]);
    const s1 = items[1].episodes;
    expect(s1[27]).toMatchObject({ episode_number: 28, absolute_number: 28, has_file: true });
    expect(s1[28]).toMatchObject({ episode_number: 29, has_file: false, name: 'Let’s Go Somewhere We Can’t Reach' });
    expect(items[0].episodes[2]).toMatchObject({ episode_number: 3, absolute_number: null, name: 'Recap: The Journey So Far', has_file: false });
    expect((await assignmentApi.showEpisodes(2)).items.map((s) => [s.season_number, s.episodes.length])).toEqual([
      [0, 42],
      [21, 156],
      [22, 38],
    ]);
  });
});

describe('endpoints', () => {
  it('polls a job while it works, and stops once it waits for the user', async () => {
    const { job_id } = await assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:poll' });
    const interval = assignmentJobQuery(job_id).refetchInterval as (query: { state: { data?: AssignmentJobDetail } }) => number | false;
    const working = await assignmentApi.job(job_id);
    expect(interval({ state: { data: working } })).toBe(1500);
    await statuses(job_id, 3);
    expect(interval({ state: { data: await assignmentApi.job(job_id) } })).toBe(false);
    expect(interval({ state: {} })).toBe(false);
  });

  it('after apply or discard, refetches the job and everything a mapping changes', async () => {
    const client = new QueryClient();
    const invalidate = jest.spyOn(client, 'invalidateQueries');
    await invalidateAfterJob(client, 101);
    expect(invalidate.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([
      ['assignment-jobs', 101],
      ['assignment-jobs', 'list'],
      ['titles'],
      ['library'],
      ['subtitles'],
      ['show-episodes'],
      ['torrents'],
    ]);
  });
});

describe('scenarios', () => {
  it('ai-off: no AI step, and re-check answers 503', async () => {
    setMockScenario('ai-off');
    expect((await assignmentApi.job(101)).job.ai_available).toBe(false);
    await expect(assignmentApi.reclassify(101, { scope: 'all' })).rejects.toMatchObject({ status: 503, code: 'unavailable' });
    const { job_id } = await assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:noai' });
    expect(await statuses(job_id, 3)).toEqual(['fetching_metadata', 'classifying', 'needs_review']);
    expect((await assignmentApi.job(job_id)).job.progress.ai_files).toBeNull();
  });

  it('ai-error: the re-check ends with ai_error and nothing changed; the next one clears it', async () => {
    setMockScenario('ai-error');
    const before = await assignmentApi.job(101);
    await assignmentApi.reclassify(101, { scope: 'all', hint: '28.5 is a recap' });
    expect(await statuses(101, 2)).toEqual(['resolving', 'needs_review']);
    const after = await assignmentApi.job(101);
    // The backend's own words
    expect(after.job.ai_error).toBe("The AI didn't answer in time, so nothing changed.");
    expect(after.proposals).toEqual(before.proposals);
    setMockScenario(null);
    await assignmentApi.reclassify(101, { scope: 'unresolved' });
    expect((await assignmentApi.job(101)).job).toMatchObject({ status: 'resolving', ai_error: null });
  });

  it('job-hold keeps a new job at a step until it’s lifted', async () => {
    setMockScenario('job-hold:classifying');
    const { job_id } = await assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:hold' });
    expect(await statuses(job_id, 5)).toEqual(['fetching_metadata', 'classifying', 'classifying', 'classifying', 'classifying']);
    expect((await assignmentApi.job(job_id)).proposals).toEqual([]);
    // Lifted: it jumps past `resolving` without a read there, and still reports the AI run
    setMockScenario(null);
    expect((await assignmentApi.job(job_id)).job).toMatchObject({ status: 'needs_review', progress: { ai_files: 3 } });

    setMockScenario('job-hold:resolving');
    const second = await assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:hold2' });
    expect(await statuses(second.job_id, 5)).toEqual(['fetching_metadata', 'classifying', 'resolving', 'resolving', 'resolving']);
    expect((await assignmentApi.job(second.job_id)).proposals).toHaveLength(16);
    // Re-checks hold in `resolving` too
    await assignmentApi.reclassify(101, { scope: 'unresolved' });
    expect(await statuses(101, 3)).toEqual(['resolving', 'resolving', 'resolving']);
  });

  it('resetMockState puts the AI settings back', async () => {
    await settingsApi.putAi({ ...(await settingsApi.ai()), auto_apply_high_confidence: false });
    resetMockState();
    expect((await settingsApi.ai()).auto_apply_high_confidence).toBe(true);
  });

  it('rules_only shows have no AI either', async () => {
    await titlesApi.patchShow(1, { classifier_mode: 'rules_only' });
    expect((await assignmentApi.job(101)).job.ai_available).toBe(false);
  });

  it('job-failed: an open job fails when it’s next read', async () => {
    setMockScenario('job-failed');
    const detail = await assignmentApi.job(101);
    expect(detail.job).toMatchObject({ status: 'failed', error: expect.stringContaining('60 s') });
    expect(detail.proposals).toEqual([]);
    expect((await titlesApi.get('tv', FRIEREN)).library).toMatchObject({ needs_review: 0, review_job_id: null });
    await expect(assignmentApi.discard(101)).rejects.toMatchObject({ status: 409, code: 'job_closed' });
  });
});
