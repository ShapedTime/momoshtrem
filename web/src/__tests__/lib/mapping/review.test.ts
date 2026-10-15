import type { AssignmentJobDetail, Proposal } from '@/lib/api/contract';
import {
  appliedSummary,
  applyErrorMessage,
  canToggleAccept,
  conflictMap,
  defaultGroupOpen,
  displayPath,
  episodeRanges,
  extrasPeek,
  firstBlocker,
  firstPending,
  footState,
  groupProposals,
  ignoredLabel,
  nextStage,
  progressSteps,
  readyToastTitle,
  restoredDecision,
  reviewCounts,
  seasonRangeSummary,
  shortName,
  shortTitle,
  summaryParts,
  targetCode,
  titleHref,
  torrentFacts,
  withDecisions,
} from '@/lib/mapping/review';
import { formatBytes } from '@/lib/title/format';
import { jobDetail, seedJobs } from '@/mocks/fixtures/assignment';
import { findTitle } from '@/mocks/fixtures/catalog';

/** A seeded mock job as the API answers it (101 Frieren, 102 One Piece, 103 Doctor Who). */
function seeded(id: number): AssignmentJobDetail {
  const job = seedJobs().get(id);
  const entry = job && findTitle(job.media_type, job.tmdb_id);
  if (!job || !entry) throw new Error(`No seeded job ${id}`);
  return jobDetail(job, { entry, ai_available: true, currentFile: () => null });
}

const named = (detail: AssignmentJobDetail, part: string): Proposal => {
  const found = detail.proposals.find((p) => p.file_path.includes(part));
  if (!found) throw new Error(`No proposal for ${part}`);
  return found;
};

const decide = (detail: AssignmentJobDetail, ...changes: Array<[string, Proposal['decision']]>) =>
  withDecisions(
    detail.proposals,
    changes.map(([part, decision]) => ({ id: named(detail, part).id, decision }))
  );

describe('groups and counts', () => {
  const frieren = seeded(101);

  it('orders groups by urgency and drops empty ones', () => {
    expect(groupProposals(frieren.proposals).map((g) => [g.group, g.proposals.length])).toEqual([
      ['needs_review', 2],
      ['unmatched', 1],
      ['specials', 1],
      ['matched', 9],
      ['extras', 3],
    ]);
  });

  it('counts each file once, and what blocks Apply', () => {
    expect(reviewCounts(frieren.proposals)).toEqual({ matched: 9, review: 2, specials: 1, ignored: 3, unmatched: 1, toApply: 12, pending: 2, conflicts: 0 });
  });

  it('opens everything on desktop; below 1024 the settled groups start collapsed', () => {
    expect(['needs_review', 'unmatched', 'specials', 'matched', 'extras'].map((g) => defaultGroupOpen(g as Proposal['group'], false))).toEqual([
      true,
      true,
      true,
      false,
      false,
    ]);
    expect(defaultGroupOpen('matched', true)).toBe(true);
  });

  it('pre-selects the first pending file, and the next one after it', () => {
    expect(firstPending(frieren.proposals)?.file_path).toContain('35v2');
    expect(firstPending(frieren.proposals, named(frieren, '35v2').id)?.file_path).toContain('28.5');
  });
});

describe('the summary line', () => {
  it('says what the design says, singular and plural', () => {
    const text = (proposals: Proposal[]) => summaryParts(reviewCounts(proposals)).map((p) => `${p.count} ${p.label}`);
    const frieren = seeded(101);
    expect(text(frieren.proposals)).toEqual(['9 matched', '2 need review', '1 special', '3 ignored', '1 unmatched']);
    expect(text(decide(frieren, ['35v2', 'accepted']))).toEqual(['10 matched', '1 needs review', '1 special', '3 ignored', '1 unmatched']);
    // Zero counts hide, except review
    expect(text(decide(frieren, ['35v2', 'accepted'], ['28.5', 'accepted'], ['Special Extra', 'ignored']))).toEqual([
      '10 matched',
      '0 left to review',
      '2 specials',
      '4 ignored',
    ]);
  });
});

describe('conflicts', () => {
  const doctorWho = seeded(103);

  it('finds non-ignored files that want the same episode, both ways', () => {
    const conflicts = conflictMap(doctorWho.proposals);
    const waters = named(doctorWho, 'S00E02.The.Waters.of.Mars');
    const extended = named(doctorWho, 'Extended.Edition');
    expect([...conflicts.keys()].sort()).toEqual([waters.id, extended.id].sort());
    expect(conflicts.get(waters.id)?.map((p) => p.id)).toEqual([extended.id]);
    // Ignoring one of them settles it
    expect(conflictMap(decide(doctorWho, ['Extended.Edition', 'ignored'])).size).toBe(0);
  });

  it('counts a conflict as review even once both are accepted', () => {
    const counts = reviewCounts(decide(doctorWho, ['S00E02.The.Waters', 'accepted'], ['Extended.Edition', 'accepted'], ['Easter.Special', 'accepted']));
    expect(counts).toMatchObject({ review: 2, pending: 0, conflicts: 2 });
    expect(footState(counts)).toMatchObject({ blocked: true, tone: 'conflict', message: 'Two files point at the same episode. Keep one.' });
  });

  it('a disabled Apply goes to the first conflict, then the first pending file', () => {
    expect(firstBlocker(doctorWho.proposals)).toEqual({ proposal: named(doctorWho, 'S00E02.The.Waters.of.Mars'), reason: 'conflict' });
    const frieren = seeded(101);
    expect(firstBlocker(frieren.proposals)).toEqual({ proposal: named(frieren, '35v2'), reason: 'pending' });
    expect(firstBlocker(decide(frieren, ['35v2', 'accepted'], ['28.5', 'accepted']))).toBeUndefined();
  });
});

describe('the sticky foot', () => {
  const frieren = seeded(101);

  it('blocks while files are pending', () => {
    expect(footState(reviewCounts(frieren.proposals))).toEqual({
      label: 'Apply 12 mappings',
      toApply: 12,
      blocked: true,
      tone: 'blocked',
      message: 'Decide on 2 files to apply',
    });
  });

  it('never blocks on unmatched files, and says so', () => {
    const decided = decide(frieren, ['35v2', 'accepted'], ['28.5', 'accepted']);
    expect(footState(reviewCounts(decided))).toMatchObject({ blocked: false, tone: 'info', message: '1 file stays unmapped. You can map it later.' });
    const all = withDecisions(decided, [{ id: named(frieren, 'Special Extra').id, decision: 'ignored' }]);
    expect(footState(reviewCounts(all))).toMatchObject({ blocked: false, tone: 'ok', message: 'Everything has a place', label: 'Apply 12 mappings' });
  });

  it('turns Apply’s 409 codes into the line under the button', () => {
    expect(applyErrorMessage('conflicting_proposals')).toBe('Two files point at the same episode. Keep one.');
    expect(applyErrorMessage('job_closed', 'cancelled')).toBe('This mapping was discarded.');
    expect(applyErrorMessage('dry_run')).toBe('This is a preview, so it can’t be applied.');
  });
});

describe('codes and ranges', () => {
  it('collapses runs of three or more', () => {
    expect(episodeRanges([29, 30, 31, 32, 33, 34, 36, 37, 38])).toBe('E29–E34, E36–E38');
    expect(episodeRanges([37, 36])).toBe('E36, E37');
    expect(episodeRanges([3, 6, 7])).toBe('E03, E06, E07');
  });

  it('peeks a collapsed group', () => {
    const frieren = seeded(101);
    const matched = frieren.proposals.filter((p) => p.group === 'matched');
    expect(seasonRangeSummary(matched)).toBe('S01 E29–E34, E36–E38');
    expect(extrasPeek(frieren.proposals.filter((p) => p.group === 'extras'))).toBe('Font, Ending (creditless), Opening (creditless)');
  });

  it('writes multi-episode targets as one range', () => {
    const t = (season: number, episode: number) => ({ season, episode });
    expect(targetCode([t(1, 5)])).toBe('S01E05');
    expect(targetCode([t(1, 1), t(1, 2)])).toBe('S01E01–E02');
    expect(targetCode([t(1, 12), t(2, 1)])).toBe('S01E12 + S02E01');
  });
});

describe('files', () => {
  it('shows paths without the torrent’s folder', () => {
    expect(displayPath('Pack/Sample/a.mkv', 'Pack')).toBe('Sample/a.mkv');
    expect(displayPath('single.mkv', 'Pack')).toBe('single.mkv');
  });

  it('names files the way the notes and tags do', () => {
    expect(shortName('[SubsPlease] Sousou no Frieren - 35v2 (1080p) [5A1B3C9D].mkv')).toBe('#35v2');
    expect(shortName('x/[SubsPlease] Sousou no Frieren - 28.5 (1080p) [0C4E9A12].mkv')).toBe('#28.5');
    expect(shortName('[SubsPlease] Sousou no Frieren - Mini Anime 05 (1080p) [AB12CD34].mkv')).toBe('Mini Anime 05');
    expect(shortName('One.Piece.892.1080p.WEB.x264-GROUP.mkv', 'One Piece')).toBe('#892');
    expect(shortName('One.Piece.Fan.Letter.1080p.WEB.x264-GROUP.mkv', 'One Piece')).toBe('Fan Letter');
    expect(shortName('Doctor.Who.2005.S00E02.The.Waters.of.Mars.720p.BluRay.x264-SHORTBREHD.mkv', 'Doctor Who')).toBe('S00E02');
    expect(shortName('Doctor.Who.2005.The.Waters.of.Mars.Extended.Edition.720p.BluRay.x264-SHORTBREHD.mkv', 'Doctor Who')).toBe(
      'Waters of Mars Extended Edition'
    );
    expect(shortName('Show.2005.017.720p.mkv', 'Show')).toBe('#017');
  });

  it('labels what an ignored file is', () => {
    expect(ignoredLabel({ kind: 'extra', extra_type: 'opening' })).toBe('Opening (creditless)');
    expect(ignoredLabel({ kind: 'extra', extra_type: 'info' })).toBe('Info file');
    expect(ignoredLabel({ kind: 'episode', extra_type: null })).toBe('Ignored');
  });
});

describe('decisions', () => {
  const frieren = seeded(101);

  it('Undo and Include go back to the group’s default', () => {
    expect(restoredDecision(named(frieren, '35v2'))).toBe('pending');
    expect(restoredDecision(named(frieren, ' - 29 '))).toBe('accepted');
    expect(restoredDecision(named(frieren, 'Special Extra'))).toBe('pending');
    // An extra with no episode becomes pending, to pick one
    expect(restoredDecision(named(frieren, 'NCOP2'))).toBe('pending');
  });

  it('can’t un-accept a picked unmatched file (the contract never empties targets)', () => {
    const extra = named(frieren, 'Special Extra');
    const picked: Proposal = { ...extra, targets: [named(frieren, ' - 29 ').targets[0]], decision: 'accepted', source: 'manual' };
    expect(canToggleAccept(picked)).toBe(false);
    expect(canToggleAccept(named(frieren, '35v2'))).toBe(true);
    expect(canToggleAccept(extra)).toBe(false);
  });

  it('a movie job keeps one file: accepting a candidate ignores the others', () => {
    const cuts: Proposal[] = [1, 2].map((id) => ({ ...named(frieren, '35v2'), id, kind: 'movie', targets: [], suggested_targets: [] }));
    expect(withDecisions(cuts, [{ id: 2, decision: 'accepted' }]).map((p) => p.decision)).toEqual(['ignored', 'accepted']);
  });
});

describe('applied', () => {
  it('summarises per season with Specials last, and what was left out', () => {
    const frieren = seeded(101);
    const applied = decide(frieren, ['35v2', 'accepted'], ['28.5', 'accepted']);
    expect(appliedSummary(applied)).toEqual({
      files: 12,
      seasons: [
        { season: 1, label: 'Season 1', range: 'E29–E38', count: 10 },
        { season: 0, label: 'Specials', range: 'E03, E06', count: 2 },
      ],
      ignored: 3,
      unmapped: 1,
      leftOut: '3 ignored · 1 unmapped · still in the torrent, hidden from Infuse',
    });
    expect(readyToastTitle('Frieren', applied)).toBe('Frieren S01E29–E38 are ready');
  });

  it('names a single episode, and specials when that’s all', () => {
    const frieren = seeded(101);
    const only = frieren.proposals.filter((p) => p.file_path.includes('Mini Anime'));
    expect(readyToastTitle('Frieren', only)).toBe('Frieren S00E06 is ready');
  });
});

describe('progress', () => {
  const frieren = seeded(101);

  it('names the three steps with their details', () => {
    const steps = progressSteps({ ...frieren.job, status: 'classifying', progress: { ...frieren.job.progress, ai_files: null } }, false);
    expect(steps).toEqual([
      { label: 'Fetching metadata', detail: 'Frieren · 2 seasons, 44 episodes', state: 'done' },
      { label: 'Matching files', detail: '16 files · 13 videos', state: 'active' },
      { label: 'Asking AI about the tricky ones', detail: null, state: 'pending' },
    ]);
  });

  it('marks the AI step skipped when it never ran', () => {
    const steps = progressSteps({ ...frieren.job, progress: { ...frieren.job.progress, ai_files: null } }, true);
    expect(steps.map((s) => [s.state, s.detail])).toEqual([
      ['done', 'Frieren · 2 seasons, 44 episodes'],
      ['done', '16 files · 13 videos'],
      ['done', 'Skipped'],
    ]);
    expect(progressSteps(frieren.job, true)[2].detail).toBe('3 files');
  });

  it('shows progress for the first run, a beat of “done”, and keeps the review through a re-check', () => {
    expect(nextStage(null, 'classifying')).toBe('progress');
    expect(nextStage('progress', 'needs_review')).toBe('done');
    expect(nextStage('done', 'needs_review')).toBe('done');
    expect(nextStage(null, 'needs_review')).toBe('settled');
    expect(nextStage('settled', 'resolving')).toBe('settled');
    expect(nextStage('progress', 'failed')).toBe('settled');
    expect(nextStage(null, undefined)).toBeNull();
  });
});

describe('the job', () => {
  const frieren = seeded(101);

  it('links back to its title, with a short name', () => {
    expect(titleHref(frieren.job)).toBe('/tv/209867');
    expect(shortTitle('Frieren: Beyond Journey’s End')).toBe('Frieren');
    expect(shortTitle('Doctor Who')).toBe('Doctor Who');
  });

  it('lists the torrent’s facts in the design’s order', () => {
    expect(torrentFacts(frieren.job, formatBytes)).toEqual(['14.4 GB', '16 files', 'Nyaa', 'SubsPlease']);
  });
});
