import {
  kindLine,
  lastWatchedLabel,
  librarySize,
  posterBadge,
  removeCopy,
  statusLine,
  totalsLine,
  watchedPct,
  watchLine,
} from '@/lib/library/display';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

const noFile = { availability_stage: 'needs_torrent' as const, webdav_path: null, webdav_url: null, size_bytes: 0 };

describe('posterBadge', () => {
  it('shows only what wants attention: Review, then Playing, then the download, then No torrent', () => {
    const everything = libraryShow({ needs_review: 2, streaming: true, download: { pct: 42, info_hash: 'h' } });
    expect(posterBadge(everything)).toEqual({ status: 'needs_review', label: 'Review' });
    expect(posterBadge({ ...everything, needs_review: 0 })).toEqual({ status: 'streaming', label: 'Playing' });
    expect(posterBadge({ ...everything, needs_review: 0, streaming: false })).toEqual({ status: 'downloading', label: '42%', pct: 42 });
    expect(posterBadge(libraryItem(noFile))).toEqual({ status: 'needs_torrent', label: 'No torrent' });
  });

  it('leaves ready titles, and shows missing a few episodes, to the status line', () => {
    expect(posterBadge(libraryItem())).toBeNull();
    expect(posterBadge(libraryShow())).toBeNull();
  });
});

describe('statusLine', () => {
  it('says what a movie is', () => {
    expect(statusLine(libraryItem())).toEqual({ kind: 'ready' });
    expect(statusLine(libraryItem({ availability_stage: 'downloading', download: { pct: 42, info_hash: 'h' } }))).toEqual({ kind: 'downloading', pct: 42 });
    expect(statusLine(libraryItem(noFile))).toEqual({ kind: 'needs_torrent' });
  });

  it('counts a show’s episodes', () => {
    expect(statusLine(libraryShow())).toEqual({ kind: 'episodes', ready: 28, total: 38, complete: false });
    expect(statusLine(libraryShow({ episodes_ready: 38 }))).toEqual({ kind: 'episodes', ready: 38, total: 38, complete: true });
    expect(statusLine(libraryShow({ ...noFile, episodes_ready: 0 }))).toEqual({ kind: 'needs_torrent' });
  });
});

describe('watch line and bar', () => {
  it('shows episodes watched', () => {
    expect(watchLine(libraryShow())).toBe('26 of 38 watched');
    expect(watchedPct(libraryShow())).toBe(68);
    expect(watchLine(libraryShow({ episodes_watched: 38 }))).toBe('Watched all');
    expect(watchLine(libraryShow({ episodes_watched: 0 }))).toBe('Not started');
    expect(watchedPct(libraryShow({ episodes_watched: 0 }))).toBe(0);
    expect(watchLine(libraryShow({ episodes_total: 1124, episodes_watched: 1071 }))).toBe('1,071 of 1,124 watched');
  });

  it('shows a movie watched, else its runtime', () => {
    expect(watchLine(libraryItem())).toBe('2h 46m');
    expect(watchedPct(libraryItem())).toBe(0);
    expect(watchLine(libraryItem({ last_watched_at: '2026-10-01T20:00:00Z' }))).toBe('Watched');
    expect(watchedPct(libraryItem({ last_watched_at: '2026-10-01T20:00:00Z' }))).toBe(100);
    expect(watchLine(libraryItem({ runtime: null }))).toBe('');
  });
});

describe('facts', () => {
  it('formats sizes, totals and kinds', () => {
    expect(librarySize(0)).toBe('—');
    expect(librarySize(63_100_000_000)).toBe('63.1 GB');
    expect(librarySize(1_454_000_000_000)).toBe('1.45 TB');
    expect(totalsLine({ total: 15, size_bytes: 2_000_000_000_000, ready: 7 })).toBe('15 titles · 2.00 TB · 7 ready in Infuse');
    expect(totalsLine({ total: 1, size_bytes: 0, ready: 0 })).toBe('1 title · 0 ready in Infuse');
    expect(kindLine(libraryShow())).toBe('Show · 2023');
    expect(kindLine(libraryItem({ year: null }))).toBe('Movie');
  });

  it('says when it was last watched', () => {
    const now = new Date(2026, 9, 16, 12);
    const at = (daysAgo: number) => ({ streaming: false, last_watched_at: new Date(2026, 9, 16 - daysAgo, 21).toISOString() });
    expect(lastWatchedLabel({ streaming: true, last_watched_at: null }, now)).toBe('Now');
    expect(lastWatchedLabel({ streaming: false, last_watched_at: null }, now)).toBeNull();
    expect(lastWatchedLabel(at(0), now)).toBe('Today');
    expect(lastWatchedLabel(at(1), now)).toBe('Yesterday');
    expect(lastWatchedLabel(at(5), now)).toBe('5 days ago');
    expect(lastWatchedLabel(at(7), now)).toBe('1 week ago');
    expect(lastWatchedLabel(at(21), now)).toBe('3 weeks ago');
    expect(lastWatchedLabel(at(47), now)).toBe('Aug 30');
  });
});

describe('removeCopy', () => {
  it('says what goes and what stays, and offers to delete the data', () => {
    expect(removeCopy(libraryShow())).toEqual({
      title: 'Remove Frieren: Beyond Journey’s End from your library?',
      body: 'This removes the show and its 28 episode mappings. Infuse stops showing it. Torrents other titles use are kept.',
      warning: null,
      checkbox: 'Also delete downloaded data (up to 39.2 GB)',
    });
    expect(removeCopy(libraryItem()).body).toBe('This removes the movie and its mapping. Infuse stops showing it. Torrents other titles use are kept.');
  });

  it('warns while it plays, and skips the checkbox without data', () => {
    expect(removeCopy(libraryItem({ streaming: true })).warning).toBe('It’s playing in Infuse right now; playback stops.');
    expect(removeCopy(libraryItem(noFile))).toMatchObject({
      body: 'This removes the movie. Nothing is assigned to it yet, so Infuse isn’t affected.',
      checkbox: null,
    });
  });
});
