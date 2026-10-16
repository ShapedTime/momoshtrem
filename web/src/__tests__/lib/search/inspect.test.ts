import { ApiError } from '@/lib/api/client';
import type { AssignmentJob, AssignmentJobDetail, MediaType } from '@/lib/api/contract';
import {
  inspectAddLabel,
  inspectJobProblem,
  inspectProblem,
  inspectRequest,
  inspectSections,
  inspectSteps,
  inspectTitle,
} from '@/lib/search/inspect';
import { jobDetail, newJob } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/** A dry run from the mapping fixtures, classified: Frieren's batch, a movie, a season pack. */
function dryRun(mediaType: MediaType, tmdbId: number, name: string, libraryId: number | null): AssignmentJobDetail {
  const magnet = `magnet:?xt=urn:btih:${'a'.repeat(40)}&dn=${encodeURIComponent(name)}`;
  const job = newJob({ id: 900, media_type: mediaType, tmdb_id: tmdbId, library_id: libraryId, magnet_uri: magnet, info_hash: 'a'.repeat(40), dry_run: true, ai: true, now: 0 });
  job.walk = null;
  job.reached = 4;
  job.ai_files = job.first_run_files;
  job.status = 'needs_review';
  const entry = findTitle(mediaType, tmdbId);
  if (!entry) throw new Error('No such title');
  return jobDetail(job, { entry, ai_available: true, currentFile: () => null });
}

const FRIEREN_BATCH = '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]';

describe('inspectRequest', () => {
  it('sends the release, and the episode or season as the hint', () => {
    const release = { id: 'rel-0' };
    expect(inspectRequest({ kind: 'show', tmdbId: 209867, season: 1, episode: 29, key: '' }, release)).toEqual({
      media_type: 'tv',
      tmdb_id: 209867,
      release_id: 'rel-0',
      hint: { season: 1, episode: 29 },
    });
    expect(inspectRequest({ kind: 'show', tmdbId: 66573, season: 1, episode: null, key: '' }, release)).toMatchObject({ hint: { season: 1 } });
    expect(inspectRequest({ kind: 'show', tmdbId: 66573, season: null, episode: null, key: '' }, release)).not.toHaveProperty('hint');
    expect(inspectRequest({ kind: 'movie', tmdbId: 1054867, key: '' }, release)).toEqual({ media_type: 'movie', tmdb_id: 1054867, release_id: 'rel-0' });
  });
});

describe('inspectSteps', () => {
  const job = (status: AssignmentJob['status'], ai_files: number | null = null) =>
    ({ status, progress: { seasons: null, episodes: null, files: null, videos: null, ai_files } }) as Pick<AssignmentJob, 'status' | 'progress'>;
  const read = (steps: ReturnType<typeof inspectSteps>) => steps.map((step) => `${step.label} (${step.state})`);

  it('says each step as it happens, and the AI step only when the AI runs', () => {
    expect(read(inspectSteps(undefined))).toEqual(['Fetching the file list (active)', 'Matching files (pending)']);
    expect(read(inspectSteps(job('classifying')))).toEqual(['Fetched file list (done)', 'Matching files (active)']);
    expect(read(inspectSteps(job('resolving', 3)))).toEqual(['Fetched file list (done)', 'Matched files (done)', 'Asking AI about 3 tricky ones (active)']);
    expect(read(inspectSteps(job('needs_review', 3)))).toEqual(['Fetched file list (done)', 'Matched files (done)', 'Asked AI about 3 tricky ones (done)']);
    expect(read(inspectSteps(job('ready', 1)))).toContain('Asked AI about 1 tricky one (done)');
    expect(read(inspectSteps(job('ready')))).toEqual(['Fetched file list (done)', 'Matched files (done)']);
  });
});

describe('inspectSections', () => {
  it('groups Frieren’s batch: will map (four, then the rest), review, specials, ignored', () => {
    const detail = dryRun('tv', 209867, FRIEREN_BATCH, 1);
    const sections = inspectSections(detail.proposals);
    expect(sections.map(({ kind, title, rows, more }) => ({ kind, title, rows: rows.length, more }))).toEqual([
      { kind: 'map', title: 'Will map · 9 episodes', rows: 4, more: '+ 5 more, all high confidence' },
      { kind: 'review', title: 'You’ll review · 3 files', rows: 3, more: null },
      { kind: 'specials', title: 'Specials · 1', rows: 1, more: null },
      { kind: 'ignored', title: 'Ignored · 3', rows: 3, more: null },
    ]);
    expect(inspectTitle(detail.job)).toMatch(/^16 files · 1\d\.\d GB$/);
  });

  it('shows a movie’s file as what will map, and its competing cuts as a pick', () => {
    const web = inspectSections(dryRun('movie', 1054867, 'One.Battle.After.Another.2025.2160p.WEB-DL-FLUX', 3).proposals);
    expect(web.map((section) => section.title)).toEqual(['Will map · 1 file', 'Ignored · 1']);
    const bluray = inspectSections(dryRun('movie', 666277, 'Past.Lives.2023.1080p.BluRay.x264-GROUP', null).proposals);
    expect(bluray.map((section) => [section.kind, section.title])).toEqual([
      ['pick', 'You’ll pick one · 2 files'],
      ['ignored', 'Ignored · 1'],
    ]);
  });

  it('counts episodes, not files, and says nothing about confidence when some aren’t high', () => {
    const detail = dryRun('tv', 66573, 'The.Good.Place.S04.1080p.WEB-DL.x264-GROUP', null);
    const [map] = inspectSections(detail.proposals);
    expect(map.title).toMatch(/^Will map · \d+ episodes$/);
    const proposals = detail.proposals.map((p, i) => (i === detail.proposals.length - 2 ? { ...p, confidence: 'medium' as const } : p));
    expect(inspectSections(proposals)[0].more).toMatch(/^\+ \d+ more$/);
  });
});

describe('words', () => {
  it('waits for the file list before naming the size', () => {
    expect(inspectTitle(undefined)).toBe('Reading the torrent…');
  });

  it('adds as the card does: a show’s review, or a movie’s download', () => {
    expect(inspectAddLabel({ media_type: 'tv', library: { in_library: false } } as never)).toBe('Add & review mapping');
    expect(inspectAddLabel({ media_type: 'movie', library: { in_library: false } } as never)).toBe('Add to library & download');
    expect(inspectAddLabel({ media_type: 'movie', library: { in_library: true } } as never)).toBe('Add & download');
  });

  it('explains why it couldn’t start, and whether Retry helps', () => {
    expect(inspectProblem(new ApiError(404, 'not_found', 'Expired'))).toMatchObject({ title: 'This result expired', retry: false });
    expect(inspectProblem(new ApiError(502, 'torrent_unavailable', 'The indexer answered 500'))).toEqual({
      title: 'Couldn’t fetch the torrent',
      body: 'The indexer answered 500',
      retry: true,
    });
    expect(inspectProblem(new ApiError(504, 'timeout', ''))).toMatchObject({ title: 'The indexer didn’t answer in time', retry: true });
    expect(inspectJobProblem({ status: 'failed', error: 'No peers sent the torrent’s file list within 60 s.' })).toMatchObject({
      body: 'No peers sent the torrent’s file list within 60 s.',
      retry: true,
    });
  });
});
