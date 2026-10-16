import type { Download, DownloadList, TorrentListItem, TorrentsSummary, TorrentWanted } from '@/lib/api/contract';
import { withDiskFields, withDownloadFields } from '@/lib/api/endpoints';
import { diskBar } from '@/lib/activity/disk';
import {
  downloadExpiry,
  downloadingLine,
  downloadName,
  downloadRow,
  downloadsUsageLine,
  formatEta,
  holdReasons,
  pausedBanner,
  stopKeepingCopy,
  torrentPill,
  wantedPct,
  wantedProgress,
  wantsMore,
} from '@/lib/activity/downloads';
import { formatTimeOfDay } from '@/lib/activity/format';
import { healthFor } from '@/lib/activity/health';
import { groupTorrents, isQuiet, onlyIdle } from '@/lib/activity/sections';
import { statusLine } from '@/lib/activity/status';
import { formatShortDate } from '@/lib/title/format';
import { DOWNLOAD_HASHES, mockDownloadList, resetMockDownloads } from '@/mocks/fixtures/downloads';
import { TORRENTS, TORRENTS_SUMMARY } from '@/mocks/fixtures/torrents';

/**
 * Background downloads on Activity (#77, P9 #207): the cards' pills, progress
 * and ETA from `wanted`, the Kept and Smart pills from `holds`, the "Kept &
 * smart" list, the disk tile, and health for the new states. Built on the
 * mocks' seeds: Interstellar kept and downloading (41% of 63.1 GB at 2.3 MB/s),
 * One Piece #1100 kept with its torrent paused, Bluey's season 3 kept and on disk.
 */

const byHash = (hash: string) => TORRENTS.find((t) => t.info_hash === hash)!;
const INTERSTELLAR = byHash(DOWNLOAD_HASHES.interstellar);
const ONE_PIECE = byHash(DOWNLOAD_HASHES.onePiece);
const BLUEY = byHash(DOWNLOAD_HASHES.bluey);
const FRIEREN = TORRENTS.find((t) => t.state === 'downloading' && t !== INTERSTELLAR)!;

/** An idle, loaded torrent whose downloads want something in `state`. */
function waiting(state: TorrentWanted['state'], patch: Partial<TorrentWanted> = {}): TorrentListItem {
  return {
    ...INTERSTELLAR,
    state: 'idle',
    download_bps: 0,
    wanted: { bytes: 63_100_000_000, done_bytes: 26_500_000_000, eta_seconds: null, state, resumes_at: null, ...patch },
  };
}

/** 01:00 tonight, in the browser's time (what "Waits until 01:00" reads). */
const ONE_AM = new Date(2026, 9, 18, 1, 0).toISOString();

afterEach(() => resetMockDownloads());

describe('wanted progress', () => {
  it('floors what its downloads want, and reads 100% only once it’s done', () => {
    expect(wantedPct(INTERSTELLAR.wanted!)).toBe(41);
    expect(wantedPct({ bytes: 100, done_bytes: 99.9, state: 'downloading' })).toBe(99);
    expect(wantedPct({ bytes: 100, done_bytes: 100, state: 'queued' })).toBe(99);
    expect(wantedPct({ bytes: 100, done_bytes: 40, state: 'done' })).toBe(100);
    expect(wantedPct({ bytes: 0, done_bytes: 0, state: 'queued' })).toBe(0);
  });

  it('writes the ETA as “about … left”, only ever an estimate', () => {
    expect(formatEta(30)).toBe('about a minute left');
    expect(formatEta(600)).toBe('about 10 min left');
    expect(formatEta(3600)).toBe('about 1 h left');
    expect(formatEta(15_913)).toBe('about 4 h 25 min left');
    expect(formatEta(90_000)).toBe('about 1 d 1 h left');
  });

  it('shows the bar and what’s on disk while downloads still want bytes', () => {
    expect(wantedProgress(INTERSTELLAR)).toEqual({ pct: 41, detail: '26.5 GB of 63.1 GB on disk · about 4 h 25 min left' });
    expect(wantedProgress(ONE_PIECE)).toEqual({ pct: 31, detail: '124 MB of 400 MB on disk' });
    expect(wantedProgress(BLUEY)).toBeNull();
    expect(wantedProgress(FRIEREN)).toBeNull();
    expect(wantsMore(INTERSTELLAR)).toBe(true);
    expect(wantsMore(BLUEY)).toBe(false);
    expect(wantsMore(FRIEREN)).toBe(false);
  });

  it('counts the bytes downloads still want in the Downloading section, not the whole torrents', () => {
    const downloading = groupTorrents(TORRENTS).downloading;
    expect(downloadingLine(downloading)).toBe('36.6 GB left for kept downloads');
    const smart = { ...INTERSTELLAR, holds: [...INTERSTELLAR.holds, { download_id: 9, reason: 'smart' as const, scope: 'episode' as const, title: 'X' }] };
    expect(downloadingLine([smart])).toBe('36.6 GB left for kept and smart downloads');
    expect(downloadingLine([FRIEREN])).toBeUndefined();
  });
});

describe('the card’s pills', () => {
  it('takes the state pill’s percentage from wanted, and says why an idle torrent waits', () => {
    expect(torrentPill(INTERSTELLAR)).toEqual({ status: 'downloading', label: '41%', pct: 41 });
    expect(torrentPill(INTERSTELLAR, 'lg')).toEqual({ status: 'downloading', label: 'Downloading 41%', pct: 41 });
    // Nothing wants it: anacrolix's progress, as before
    expect(torrentPill(FRIEREN)).toEqual({ status: 'downloading', label: '64%', pct: 64 });
    expect(torrentPill(BLUEY)).toEqual({ status: 'on_disk' });
    expect(torrentPill(ONE_PIECE)).toEqual({ status: 'paused' });
    expect(torrentPill(waiting('waiting_for_stream'))).toEqual({ status: 'waiting_for_stream', label: 'Waits for the stream to end' });
    expect(torrentPill(waiting('waiting_for_window', { resumes_at: ONE_AM }))).toEqual({ status: 'waiting_for_window', label: 'Waits until 01:00' });
    expect(torrentPill(waiting('stalled'))).toEqual({ status: 'stalled', label: 'No peers · retrying' });
    expect(torrentPill(waiting('queued'))).toEqual({ status: 'queued', label: 'Queued' });
    expect(torrentPill(waiting('disk_full'))).toEqual({ status: 'disk_full', label: 'Disk full' });
    expect(torrentPill({ ...BLUEY, wanted: null })).toEqual({ status: 'complete', label: undefined });
    expect(torrentPill({ ...BLUEY, wanted: null }, 'lg')).toEqual({ status: 'complete', label: 'Complete · ready in Infuse' });
  });

  it('shows Kept and Smart once each, kept first', () => {
    const hold = (reason: 'kept' | 'smart', id: number) => ({ download_id: id, reason, scope: 'episode' as const, title: 'Frieren' });
    expect(holdReasons([hold('kept', 2), hold('kept', 3), hold('smart', 1)])).toEqual(['kept', 'smart']);
    expect(holdReasons([hold('smart', 1)])).toEqual(['smart']);
    expect(holdReasons([])).toEqual([]);
    expect(holdReasons(BLUEY.holds)).toEqual(['kept']);
  });
});

describe('sections and the status line', () => {
  it('isn’t quiet while an idle torrent’s downloads wait, but is once they’re on disk', () => {
    expect(isQuiet([BLUEY])).toBe(true);
    expect(isQuiet([waiting('waiting_for_window')])).toBe(false);
    expect(onlyIdle([waiting('waiting_for_window')])).toBe(true);
    expect(statusLine([waiting('queued')], null)).toEqual({ lead: 'Everything’s working.', text: '' });
    // A torrent you paused waits for you, not in the queue
    expect(statusLine([ONE_PIECE, BLUEY], null).text).toMatch(/^All quiet/);
  });
});

describe('health', () => {
  it('says why an idle torrent’s downloads wait, and what still plays', () => {
    const cases = [
      [waiting('waiting_for_stream'), 'waiting', 'Waits for the stream to end', /Something else is streaming.*a minute after it ends\. 41% of what you keep is on disk\./],
      [waiting('waiting_for_window', { resumes_at: ONE_AM }), 'waiting', 'Waits until 01:00', /outside your download windows.*Infuse still plays it/],
      [waiting('queued'), 'waiting', 'Queued', /Two downloads run at a time/],
      [waiting('stalled', { resumes_at: ONE_AM }), 'slow', 'No peers · retrying', /Nothing arrived for 10 minutes.*It tries again at 01:00\./],
      [waiting('disk_full'), 'slow', 'Disk full', /keeps 5 GB of it free/],
      [waiting('paused'), 'paused', 'Paused', /Background downloads are off in Settings › Storage/],
      [BLUEY, 'complete', 'On disk', /Everything is on disk/],
      [{ ...BLUEY, complete: false }, 'complete', 'On disk', /Everything you keep of it is on disk/],
    ] as const;
    for (const [torrent, kind, title, body] of cases) {
      const health = healthFor(torrent);
      expect(health).toMatchObject({ kind, title });
      expect(health.body).toMatch(body);
    }
  });

  it('gives the ETA of what downloads want while they fetch, and says a paused torrent’s downloads wait', () => {
    expect(healthFor(INTERSTELLAR).body).toContain('It downloads what you keep in the background: 41% is on disk, about 4 h 25 min left.');
    expect(healthFor({ ...INTERSTELLAR, seeders: 20 }).body).toMatch(/41% is on disk, about 4 h 25 min left\. You can start watching now\.$/);
    expect(healthFor(ONE_PIECE).body).toContain('Its downloads wait too: 31% of what you keep is on disk.');
  });
});

describe('the “Kept & smart” list', () => {
  const list = () => mockDownloadList();
  const find = (title: string, scope?: Download['scope']) => list().items.find((d) => d.item.title === title && (!scope || d.scope === scope))!;

  it('gives each download its state, size, progress and expiry', () => {
    const interstellar = find('Interstellar');
    expect(downloadRow(interstellar)).toMatchObject({
      title: 'Interstellar',
      scope: null,
      href: '/movie/157336',
      pill: { status: 'downloading', label: 'Downloading 41%', pct: 41 },
      pct: 41,
      size: '26.5 GB of 63.1 GB · about 4 h 25 min left',
      files: null,
      retry: null,
      expiry: `Kept until watched · by ${formatShortDate(interstellar.expires_at!)}`,
    });
    expect(downloadRow(find('Bluey'))).toMatchObject({
      scope: 'Season 3',
      pill: { status: 'on_disk', label: 'On disk' },
      pct: null,
      size: '21.7 GB',
      files: '49 episodes',
      expiry: 'Kept',
    });
    expect(downloadRow(find('One Piece'))).toMatchObject({ scope: 'S22E15 · Episode 1100', pill: { status: 'paused', label: 'Paused' }, pct: 31 });
    const studio = find('The Studio');
    expect(downloadRow(studio)).toMatchObject({
      pill: { status: 'stalled', label: 'No peers · retrying' },
      retry: `Tries again at ${formatTimeOfDay(Date.parse(studio.resumes_at!))}`,
    });
    expect(downloadRow(find('One Battle After Another'))).toMatchObject({ pill: { status: 'no_file', label: 'No file yet' }, pct: null, size: null });
    const frieren = list().items.find((d) => d.reason === 'smart')!;
    expect(downloadRow(frieren)).toMatchObject({ scope: 'S01E27 · An Ordinary Mage', pill: { status: 'on_disk' } });
    expect(downloadExpiry(frieren)).toBe(`Smart pick · until watched · by ${formatShortDate(frieren.expires_at!)} · from Up next`);
    expect(downloadExpiry({ ...frieren, smart_source: 'streaming' })).toMatch(/· after what’s streaming$/);
  });

  it('names a download for Stop keeping, in the title pages’ words; a smart pick is dismissed', () => {
    expect(downloadName(find('Bluey'))).toBe('Bluey · Season 3');
    expect(downloadName(find('One Piece'))).toBe('One Piece · S22E15');
    expect(stopKeepingCopy(find('Interstellar'))).toEqual({
      action: 'Stop keeping',
      title: 'Stopped keeping Interstellar',
      body: 'What’s downloaded stays as cache until it’s cleared to make room.',
      failed: 'Couldn’t stop keeping Interstellar',
    });
    const smart = stopKeepingCopy(list().items.find((d) => d.reason === 'smart')!);
    expect(smart).toMatchObject({ action: 'Dismiss', title: 'Dismissed the smart pick for Frieren: Beyond Journey’s End · S01E27' });
    expect(smart.body).toMatch(/^It won’t be downloaded ahead again\./);
  });

  it('sums up what’s on disk against the caps', () => {
    const current = list();
    expect(downloadsUsageLine(current)).toBe('48.8 GB of 375 GB kept · 1.9 GB of 113 GB smart');
    expect(downloadsUsageLine({ ...current, items: current.items.filter((d) => d.reason === 'kept'), usage: { ...current.usage, smart_bytes: 0 } })).toBe(
      '48.8 GB of 375 GB kept'
    );
  });

  it('says why background downloads wait, only while something still wants bytes', () => {
    const base: DownloadList = { ...list(), paused_reason: null, resumes_at: null };
    expect(pausedBanner(base)).toBeNull();
    expect(pausedBanner({ ...base, paused_reason: 'streaming' })).toEqual({
      title: 'Downloads wait for the stream to end',
      body: 'So Infuse gets the bandwidth. They pick up again a minute after it ends.',
      settings: false,
    });
    expect(pausedBanner({ ...base, paused_reason: 'streaming', resumes_at: ONE_AM })?.body).toBe('The stream ended, so they pick up again at 01:00.');
    expect(pausedBanner({ ...base, paused_reason: 'window', resumes_at: ONE_AM })).toEqual({
      title: 'Downloads wait until 01:00',
      body: 'That’s when your next download window opens. What’s on disk still plays in Infuse.',
      settings: true,
    });
    expect(pausedBanner({ ...base, paused_reason: 'disabled' })).toMatchObject({ title: 'Background downloads are off', settings: true });
    // Everything is on disk: nothing waits
    expect(pausedBanner({ ...base, paused_reason: 'disabled', items: base.items.filter((d) => d.state === 'done') })).toBeNull();
  });
});

describe('the disk tile', () => {
  const disk = TORRENTS_SUMMARY.disk!;

  it('splits the cache into kept, smart and streaming cache, with a marker at the limit', () => {
    const bar = diskBar(disk);
    expect(bar.segments.map((s) => s.kind)).toEqual(['kept', 'smart', 'cache', 'other']);
    expect(bar.segments.reduce((sum, s) => sum + s.bytes, 0)).toBe(disk.total_bytes - disk.free_bytes);
    expect(bar.limitAt).toBe('75.0%');
    expect(bar.cacheLine).toBe('62.4 GB of 750 GB cache · 1.0 TB disk');
    expect(bar.pinned).toEqual({ kept: '48.8 GB kept', smart: '1.9 GB smart' });
    expect(bar.label).toBe(
      'Disk: 48.8 GB kept, 1.9 GB smart, 11.7 GB streaming cache, 126 GB other, 812 GB free of 1.0 TB; the cache may grow to 750 GB'
    );
  });

  it('draws today’s bar when nothing is pinned (a backend without downloads)', () => {
    const bar = diskBar({ ...disk, kept_bytes: 0, smart_bytes: 0 });
    expect(bar.segments.map((s) => s.kind)).toEqual(['cache', 'other']);
    expect(bar.pinned).toBeNull();
    expect(bar.label).toMatch(/^Disk: 62\.4 GB in torrents, 126 GB other, 812 GB free/);
  });
});

describe('a backend without downloads', () => {
  it('reads missing wanted, holds and pinned bytes as nothing', () => {
    const { wanted: _wanted, holds: _holds, ...old } = INTERSTELLAR;
    void _wanted;
    void _holds;
    expect(withDownloadFields(old as TorrentListItem)).toMatchObject({ wanted: null, holds: [] });
    expect(withDownloadFields(INTERSTELLAR)).toEqual(INTERSTELLAR);
    const { kept_bytes: _kept, smart_bytes: _smart, ...oldDisk } = TORRENTS_SUMMARY.disk!;
    void _kept;
    void _smart;
    const summary = withDiskFields({ ...TORRENTS_SUMMARY, disk: oldDisk as NonNullable<TorrentsSummary['disk']> });
    expect(summary.disk).toMatchObject({ kept_bytes: 0, smart_bytes: 0, cache_limit_bytes: TORRENTS_SUMMARY.disk!.cache_limit_bytes });
    expect(withDiskFields({ ...TORRENTS_SUMMARY, disk: null }).disk).toBeNull();
  });
});
