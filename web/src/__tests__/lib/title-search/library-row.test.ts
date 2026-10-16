import {
  findTarget,
  lacksFile,
  libraryRowAction,
  libraryRowPill,
  libraryRowState,
  libraryRowSub,
  librarySecondary,
  libraryStatusText,
  missingSubject,
} from '@/lib/title-search/library-row';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

const off = { torrentSheet: false };
const live = { torrentSheet: true };

const readyMovie = libraryItem();
const partialShow = libraryShow();
const reviewShow = libraryShow({ needs_review: 2, review_job_id: 101 });
const downloading = libraryItem({ tmdb_id: 157336, title: 'Interstellar', availability_stage: 'downloading', download: { pct: 42, info_hash: 'h' } });
const noTorrent = libraryItem({ tmdb_id: 1054867, title: 'One Battle After Another', availability_stage: 'needs_torrent', webdav_path: null, webdav_url: null, size_bytes: 0 });
const readyShow = libraryShow({ availability_stage: 'ready', episodes_ready: 38, missing: null, episodes_watched: 38 });

describe('libraryRowState', () => {
  it('puts review first, then the download, then nothing to play, then missing episodes', () => {
    expect(libraryRowState(reviewShow)).toBe('needs_review');
    expect(libraryRowState(downloading)).toBe('downloading');
    expect(libraryRowState(noTorrent)).toBe('needs_torrent');
    expect(libraryRowState(partialShow)).toBe('partial');
    expect(libraryRowState(readyMovie)).toBe('ready');
    expect(libraryRowState(readyShow)).toBe('ready');
  });

  it('needs a job to call it review', () => {
    expect(libraryRowState(libraryShow({ needs_review: 2, review_job_id: null }))).toBe('partial');
  });

  it('calls a show with no file at all a needs-torrent', () => {
    expect(libraryRowState(libraryShow({ webdav_path: null, episodes_ready: 0 }))).toBe('needs_torrent');
  });
});

describe('pills, lines and status words', () => {
  it('words each state in full for a search row', () => {
    expect(libraryRowPill(readyMovie)).toEqual({ status: 'ready_infuse', label: 'Ready in Infuse' });
    expect(libraryRowPill(readyShow)).toEqual({ status: 'ready', label: '38/38 ready' });
    expect(libraryRowPill(partialShow)).toEqual({ status: 'partial', label: '28/38 ready' });
    expect(libraryRowPill(downloading)).toEqual({ status: 'downloading', label: 'Downloading 42%', pct: 42 });
    expect(libraryRowPill(reviewShow)).toEqual({ status: 'needs_review', label: 'Needs review' });
    expect(libraryRowPill(noTorrent)).toEqual({ status: 'needs_torrent', label: 'Needs a torrent' });
  });

  it('keeps them short for the palette', () => {
    expect(libraryRowPill(readyMovie, { short: true }).label).toBe('Ready');
    expect(libraryRowPill(partialShow, { short: true }).label).toBe('28/38');
    expect(libraryRowPill(downloading, { short: true }).label).toBe('42%');
    expect(libraryRowPill(reviewShow, { short: true }).label).toBe('Review');
    expect(libraryRowPill(noTorrent, { short: true }).label).toBe('No torrent');
  });

  it('says what matters under the title', () => {
    expect(libraryRowSub(readyMovie)).toBe('2160p · 18.4 GB');
    expect(libraryRowSub(readyShow)).toBe('Watched all');
    expect(libraryRowSub(partialShow)).toBe('10 episodes missing');
    expect(libraryRowSub(reviewShow)).toBe('28 of 38 ready · 2 files to check');
    expect(libraryRowSub(libraryItem({ needs_review: 1, review_job_id: 7 }))).toBe('1 file to check');
    expect(libraryRowSub(downloading)).toBe('Plays while it downloads');
    expect(libraryRowSub(noTorrent)).toBe('In your library, no torrent yet');
  });

  it('uses the shared status words in the palette', () => {
    expect(libraryStatusText(readyMovie)).toBe('Ready in Infuse');
    expect(libraryStatusText(readyShow)).toBe('Ready');
    expect(libraryStatusText(partialShow)).toBe('28/38 ready');
    expect(libraryStatusText(reviewShow)).toBe('Needs review');
    expect(libraryStatusText(downloading)).toBe('Downloading');
    expect(libraryStatusText(noTorrent)).toBe('Needs a torrent');
  });
});

describe('actions', () => {
  it('gives each state its action', () => {
    expect(libraryRowAction(readyMovie, off)).toEqual({ kind: 'play', label: 'Play' });
    expect(libraryRowAction(downloading, off)).toEqual({ kind: 'activity', label: 'Activity', href: '/activity/h' });
    expect(libraryRowAction(reviewShow, off)).toEqual({ kind: 'review', label: 'Review', href: '/mapping/101' });
    expect(libraryRowAction(partialShow, off)).toEqual({ kind: 'find', label: 'Find missing', target: { kind: 'page', href: '/tv/209867?season=1' } });
    expect(libraryRowAction(noTorrent, off)).toEqual({ kind: 'find', label: 'Find', target: { kind: 'page', href: '/movie/1054867' } });
  });

  it('opens the torrent sheet once it’s live', () => {
    expect(findTarget(partialShow, live)).toEqual({ kind: 'sheet', params: { show: '209867', season: '1', se: 'S01E29' } });
    expect(findTarget(noTorrent, live)).toEqual({ kind: 'sheet', params: { movie: '1054867' } });
  });

  it('falls back to the title page when nothing scopes the search', () => {
    expect(findTarget(libraryShow({ missing: null, webdav_path: null }), live)).toEqual({ kind: 'page', href: '/tv/209867' });
  });

  it('knows which titles lack a file, whatever else is going on', () => {
    expect([partialShow, reviewShow, noTorrent, libraryShow({ missing: null, webdav_path: null })].map(lacksFile)).toEqual([true, true, true, true]);
    expect([readyMovie, readyShow, downloading].map(lacksFile)).toEqual([false, false, false]);
  });

  it('offers ⌘↵ only to titles that lack a file', () => {
    expect(librarySecondary(partialShow, off)).toEqual({ label: 'Find missing', target: { kind: 'page', href: '/tv/209867?season=1' } });
    expect(librarySecondary(reviewShow, off)?.label).toBe('Find missing');
    expect(librarySecondary(noTorrent, off)?.label).toBe('Find a torrent');
    expect(librarySecondary(readyMovie, off)).toBeNull();
    expect(librarySecondary(downloading, off)).toBeNull();
  });
});

describe('missingSubject', () => {
  const run = { season_number: 1, from: 29, to: 38, absolute_from: null, absolute_to: null, count: 10 };
  it('names the run as the title page does', () => {
    expect(missingSubject(run)).toBe('episodes 29–38');
    expect(missingSubject({ ...run, to: 29, count: 1 })).toBe('episode 29');
    expect(missingSubject({ ...run, absolute_from: 1122, absolute_to: 1123, count: 2 })).toBe('episodes 1122–1123');
    expect(missingSubject({ ...run, season_number: 2, from: 8, to: 10, count: 3 })).toBe('S02E08–E10');
    expect(missingSubject({ ...run, season_number: 3, from: 5, to: 5, count: 1 })).toBe('S03E05');
  });
});
