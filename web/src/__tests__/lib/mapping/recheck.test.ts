import type { AssignmentJobDetail, Proposal } from '@/lib/api/contract';
import { defaultScope, doneMessage, recheckCounts, updatedCount, workingDetail, workingTitle } from '@/lib/mapping/recheck';
import { canHaveEpisodes, fileClass } from '@/lib/mapping/files';
import { assignHint } from '@/hooks/useAssignTorrent';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

function seeded(id: number): AssignmentJobDetail {
  const job = seedJobs().get(id);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error(`No seeded job ${id}`);
  return jobDetail(job, { entry, ai_available: true, currentFile: () => null });
}

describe('re-check scopes', () => {
  const { proposals } = seeded(101);

  it('as the backend asks: episode files you haven’t decided; unresolved ones are pending or in a conflict', () => {
    // 13 episode files (the extras keep the rules' reading), 3 pending
    expect(recheckCounts(proposals)).toEqual({ unresolved: 3, all: 13 });
    const [first, ...rest] = proposals;
    const decided: Proposal[] = [{ ...first, decision: 'accepted', user_decided: true }, ...rest];
    expect(recheckCounts(decided)).toEqual({ unresolved: 2, all: 12 });
    // A matched file that collides with another is unresolved too
    const matched = proposals.filter((p) => p.group === 'matched');
    const clash: Proposal[] = proposals.map((p) => (p.id === matched[1].id ? { ...p, targets: matched[0].targets } : p));
    expect(recheckCounts(clash)).toEqual({ unresolved: 5, all: 13 });
  });

  it('opens on the unresolved files, unless there are none', () => {
    expect(defaultScope({ unresolved: 3, all: 16 })).toBe('unresolved');
    expect(defaultScope({ unresolved: 0, all: 16 })).toBe('all');
  });

  it('counts the rows the run changed', () => {
    expect(updatedCount(proposals)).toBe(0);
    expect(updatedCount(proposals.map((p, i) => ({ ...p, updated: i < 2 })))).toBe(2);
  });
});

describe('re-check words', () => {
  it('says how many files and whether a hint went with them', () => {
    expect(workingTitle(3, true)).toBe('Re-checking 3 files with your hint…');
    expect(workingTitle(1, false)).toBe('Re-checking 1 file…');
    expect(workingTitle(null, false)).toBe('Re-checking the files…');
    expect(workingDetail('28.5 is a recap')).toBe('“28.5 is a recap”');
    expect(workingDetail('')).toBe('Comparing names, air dates and runtimes');
  });

  it('reports the changes, or none', () => {
    expect(doneMessage(1)?.lead).toBe('Updated 1 suggestion, marked ');
    expect(doneMessage(3)).toEqual({ lead: 'Updated 3 suggestions, marked ', tail: ' below. Accept the ones that look right.' });
    expect(doneMessage(0)).toBeNull();
  });
});

describe('file classes', () => {
  it('sorts files by extension as the backend does', () => {
    expect(fileClass('Show/Show.S01E01.1080p.mkv')).toBe('video');
    expect(fileClass('Show.S01E01.en.ASS')).toBe('subtitle');
    expect(fileClass('Fonts/OpenSans-Semibold.ttf')).toBe('font');
    expect(fileClass('Show.2009.Specials.nfo')).toBe('info');
    expect(fileClass('poster.jpg')).toBe('image');
    expect(fileClass('Show.part.r01')).toBe('archive');
    expect(fileClass('README')).toBe('other');
  });

  it('only videos and subtitles can have episodes (Include)', () => {
    expect(canHaveEpisodes('NCOP2 (1080p).mkv')).toBe(true);
    expect(canHaveEpisodes('Show.S00E01.srt')).toBe(true);
    expect(canHaveEpisodes('Fonts/OpenSans-Semibold.ttf')).toBe(false);
    expect(canHaveEpisodes('Show.nfo')).toBe(false);
  });
});

describe('assign hints from the legacy search', () => {
  const base = { query: 'Frieren', title: 'Frieren', tmdbId: 209867 };
  it('an episode search hints its episode, a season search its season', () => {
    expect(assignHint({ ...base, mediaType: 'episode', season: 1, episode: 30 })).toEqual({ season: 1, episode: 30 });
    expect(assignHint({ ...base, mediaType: 'tv', season: 2 })).toEqual({ season: 2 });
    expect(assignHint({ ...base, mediaType: 'tv' })).toBeUndefined();
    expect(assignHint({ ...base, mediaType: 'movie' })).toBeUndefined();
  });
});
