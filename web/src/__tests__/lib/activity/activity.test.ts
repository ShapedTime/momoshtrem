import type { StreamingSession, TorrentDetail, TorrentListItem } from '@/lib/api/contract';
import { fileProgress, filesSummary, readersByPath, relativePath, unmappedLabel } from '@/lib/activity/files';
import { formatClock, formatPieceSize, nowTile, readerLine, sessionBanner, splitSpeed, torrentLabel } from '@/lib/activity/format';
import { healthFor, isWeak } from '@/lib/activity/health';
import { expandRuns, fileAtPiece, PIECE, pieceGrid, piecesHave } from '@/lib/activity/pieces';
import { removeTorrentCopy, torrentRemovalKey } from '@/lib/activity/remove';
import { groupTorrents, isQuiet, sortTorrents, streamingNow } from '@/lib/activity/sections';
import { SPARK_HEIGHT, SPARK_SCALE_BPS, sampleAt, sparkPaths, sparkTip, sparkY } from '@/lib/activity/sparkline';
import { statusLine } from '@/lib/activity/status';
import { HDREZKA_SESSION, STREAMING_SESSION, TORRENTS, torrentDetail } from '@/mocks/fixtures/torrents';

const byState = (state: TorrentListItem['state']) => TORRENTS.find((t) => t.state === state)!;
const STREAMING = byState('streaming');
const INTERSTELLAR = TORRENTS.find((t) => t.titles[0]?.title === 'Interstellar')!;
const FRIEREN = TORRENTS.find((t) => t.state === 'downloading' && t !== INTERSTELLAR)!;
const PAUSED = byState('paused');
const torrent = (patch: Partial<TorrentListItem>): TorrentListItem => ({ ...FRIEREN, ...patch });

describe('status line', () => {
  it('says all quiet when nothing streams or downloads', () => {
    expect(statusLine([], null)).toEqual({ lead: null, text: 'All quiet. Nothing in the queue, and the server is fine.' });
    expect(statusLine(TORRENTS.filter((t) => t.state === 'idle' || t.state === 'paused'), null).text).toMatch(/^All quiet/);
  });

  it('says everything works, who is streaming, and which torrent is slow', () => {
    expect(statusLine(TORRENTS, STREAMING_SESSION)).toEqual({
      lead: 'Everything’s working.',
      text: 'Infuse is streaming smoothly; Interstellar is slow because it has few peers.',
    });
    expect(statusLine([FRIEREN], STREAMING_SESSION).text).toBe('Infuse is streaming smoothly.');
    expect(statusLine([FRIEREN, INTERSTELLAR], null).text).toBe('Interstellar is slow because it has few peers.');
    expect(statusLine([FRIEREN], null)).toEqual({ lead: 'Everything’s working.', text: '' });
    expect(statusLine([FRIEREN], { ...STREAMING_SESSION, client: null }).text).toBe('A player is streaming smoothly.');
  });

  it('counts an HDRezka session as working, with no torrent busy', () => {
    expect(statusLine([], HDREZKA_SESSION).lead).toBe('Everything’s working.');
  });
});

describe('health', () => {
  it('flags few seeders only while fetching', () => {
    expect(isWeak(INTERSTELLAR)).toBe(true);
    expect(isWeak(FRIEREN)).toBe(false);
    expect(isWeak(torrent({ state: 'idle', seeders: 0 }))).toBe(false);
    expect(isWeak(torrent({ state: 'streaming', complete: true, seeders: 0 }))).toBe(false);
    expect(isWeak(torrent({ state: 'streaming', complete: false, seeders: 2 }))).toBe(true);
    expect(isWeak(torrent({ seeders: null }))).toBe(false);
  });

  it('writes Healthy, Slow but moving, Paused, Idle and Complete without an ETA', () => {
    const cases = [
      [healthFor(FRIEREN), 'healthy', 'Healthy', /31 seeders and 48 peers connected, at 6\.4 MB\/s/],
      [healthFor(INTERSTELLAR), 'slow', 'Slow but moving', /Only 3 seeders and 6 peers connected, at 2\.3 MB\/s/],
      [healthFor(torrent({ seeders: 0, peers: 0, download_bps: 0 })), 'slow', 'Waiting for peers', /No seeders are connected/],
      [healthFor(PAUSED), 'paused', 'Paused', /What’s on disk \(31%\) still plays in Infuse/],
      [healthFor(TORRENTS.find((t) => !t.loaded)!), 'idle', 'Idle', /It fetches pieces when Infuse plays a file/],
      [healthFor(TORRENTS.find((t) => t.state === 'idle' && t.complete)!), 'complete', 'Complete', /Everything is on disk/],
      [healthFor(STREAMING, STREAMING_SESSION), 'healthy', 'Healthy', /never waits for the network/],
    ] as const;
    for (const [health, kind, title, body] of cases) {
      expect(health).toMatchObject({ kind, title });
      expect(health.body).toMatch(body);
      expect(health.body).not.toMatch(/\bleft\b|minutes? to go|ETA|finish/i);
    }
    expect(healthFor(torrent({ state: 'streaming', complete: false }), STREAMING_SESSION).body).toMatch(/about 33 min ahead of playback/);
  });
});

describe('sections', () => {
  it('groups by state in the backend’s order, newest read first', () => {
    const shuffled = [...TORRENTS].reverse();
    expect(sortTorrents(shuffled).map((t) => t.info_hash)).toEqual(TORRENTS.map((t) => t.info_hash));
    const groups = groupTorrents(shuffled);
    expect(groups.streaming).toEqual([STREAMING]);
    expect(groups.downloading.map((t) => t.titles[0].title)).toEqual(['Frieren: Beyond Journey’s End', 'Interstellar']);
    expect(groups.paused).toEqual([PAUSED]);
    expect(groups.idle).toHaveLength(4);
    // Never loaded (no last read) after the loaded ones; then the newest added
    expect(groups.idle.map((t) => t.loaded)).toEqual([true, true, false, false]);
  });

  it('is quiet with no torrents, or nothing but idle ones', () => {
    expect(isQuiet([])).toBe(true);
    expect(isQuiet(TORRENTS.filter((t) => t.state === 'idle'))).toBe(true);
    expect(isQuiet([PAUSED])).toBe(false);
  });

  it('picks the newest session for “Streaming now”, skipping paused torrents and ones being removed', () => {
    const sessions = [STREAMING_SESSION, HDREZKA_SESSION];
    expect(streamingNow(sessions, TORRENTS)).toBe(STREAMING_SESSION);
    const paused = TORRENTS.map((t) => (t === STREAMING ? { ...t, state: 'paused' as const } : t));
    expect(streamingNow(sessions, paused)).toBe(HDREZKA_SESSION);
    expect(streamingNow([STREAMING_SESSION], paused)).toBeNull();
    // Removed: the list no longer has it, though its session lingers
    expect(streamingNow(sessions, TORRENTS.filter((t) => t !== STREAMING))).toBe(HDREZKA_SESSION);
    expect(streamingNow([], TORRENTS)).toBeNull();
  });
});

describe('words and numbers', () => {
  it('formats clocks, speeds and piece sizes', () => {
    expect(formatClock(1122)).toBe('18:42');
    expect(formatClock(3725)).toBe('1:02:05');
    expect(splitSpeed(29_800_000)).toEqual({ value: '29.8', unit: 'MB/s' });
    expect(splitSpeed(420_000)).toEqual({ value: '420', unit: 'KB/s' });
    expect(splitSpeed(0)).toEqual({ value: '0', unit: 'MB/s' });
    expect(formatPieceSize(16 * 1024 * 1024)).toBe('16 MB');
    expect(formatPieceSize(512 * 1024)).toBe('512 KB');
  });

  it('labels a torrent by its title and scope, and leaves one nothing links unlinked', () => {
    expect(torrentLabel(STREAMING)).toEqual({ text: 'Severance · Season 2', href: '/tv/95396', linked: true });
    expect(torrentLabel(INTERSTELLAR)).toEqual({ text: 'Interstellar', href: '/movie/157336', linked: true });
    expect(torrentLabel(TORRENTS.find((t) => t.titles.length === 0)!)).toMatchObject({ text: 'Past.Lives.2023.1080p.BluRay.x265', href: null, linked: false });
  });

  it('builds the “Streaming now” tile, leaving out what isn’t known', () => {
    expect(nowTile(STREAMING_SESSION)).toEqual({
      title: 'Severance S02E05 on Apple TV',
      detail: 'Trojan’s Horse · 18:42 of 52:00 · buffered 33 min ahead',
      href: `/activity/${STREAMING.info_hash}`,
    });
    const bare: StreamingSession = { ...STREAMING_SESSION, device: null, position_seconds: null, buffered_ahead_seconds: null };
    expect(nowTile(bare)).toMatchObject({ title: 'Severance S02E05 in Infuse', detail: 'Trojan’s Horse' });
    expect(nowTile({ ...bare, client: 'Unknown', item: null }).title).toBe(STREAMING_SESSION.file_name);
    // HDRezka has no torrent page: the title instead
    expect(nowTile(HDREZKA_SESSION)).toMatchObject({ title: 'Doctor Who S01E01 on iPhone', href: '/tv/57243' });
    expect(nowTile(HDREZKA_SESSION).detail).not.toMatch(/buffered/);
  });

  it('words the streaming banner and the active readers', () => {
    expect(sessionBanner(STREAMING_SESSION)).toEqual({ title: 'S02E05 Trojan’s Horse', detail: 'Apple TV · Infuse · 18:42 of 52:00 · buffered 33 min ahead' });
    expect(readerLine(STREAMING_SESSION)).toBe('Infuse · S02E05 · at 18:42 of 52:00 · 9.1 MB/s');
  });
});

describe('remove dialog', () => {
  it('says what Infuse loses and that the title stays', () => {
    expect(removeTorrentCopy(STREAMING)).toEqual({
      title: 'Remove this torrent?',
      body: 'Infuse will no longer be able to play Severance · Season 2. The show stays in your library and those episodes show “Needs a torrent”.',
      checkbox: 'Also delete downloaded data (48.3 GB)',
    });
    expect(removeTorrentCopy(INTERSTELLAR)).toMatchObject({
      body: 'Infuse will no longer be able to play Interstellar. The movie stays in your library and shows “Needs a torrent”.',
      checkbox: 'Also delete downloaded data (up to 63.1 GB)',
    });
    expect(removeTorrentCopy({ ...INTERSTELLAR, size_bytes: null }).checkbox).toBeNull();
    expect(removeTorrentCopy(TORRENTS.find((t) => t.titles.length === 0)!).body).toBe(
      'Infuse will no longer be able to play its files. Nothing in your library uses it, so nothing else changes.'
    );
    expect(torrentRemovalKey('ABC')).toBe('torrent:abc');
  });
});

describe('piece map', () => {
  it('expands run-length states into one cell per piece', () => {
    const cells = expandRuns([
      { state: 'have', count: 3 },
      { state: 'partial', count: 1 },
      { state: 'priority', count: 2 },
      { state: 'missing', count: 2 },
    ]);
    expect(Array.from(cells)).toEqual([1, 1, 1, 2, 3, 3, 0, 0]);
    expect(PIECE).toEqual({ missing: 0, have: 1, partial: 2, priority: 3 });
    // pieces_total wins: padded with missing, or cut
    expect(Array.from(expandRuns([{ state: 'have', count: 2 }], 4))).toEqual([1, 1, 0, 0]);
    expect(Array.from(expandRuns([{ state: 'have', count: 6 }], 3))).toEqual([1, 1, 1]);
    expect(piecesHave([{ state: 'have', count: 3 }, { state: 'missing', count: 1 }, { state: 'have', count: 2 }])).toBe(5);
  });

  it('lays out squares to the width and finds a piece’s file', () => {
    expect(pieceGrid(390, 1000)).toEqual({ cell: 5, step: 6, cols: 65, rows: 16 });
    expect(pieceGrid(822, 1718)).toMatchObject({ cell: 6, step: 7, cols: 117, rows: 15 });
    const files = [
      { path: 'a.mkv', size_bytes: 100 },
      { path: 'b.mkv', size_bytes: 50 },
    ];
    expect(fileAtPiece(files, 40, 0)?.path).toBe('a.mkv');
    expect(fileAtPiece(files, 40, 2)?.path).toBe('a.mkv');
    expect(fileAtPiece(files, 40, 3)?.path).toBe('b.mkv');
  });
});

describe('sparkline', () => {
  it('draws on a fixed 0–40 MB/s scale, clamped', () => {
    expect(SPARK_SCALE_BPS).toBe(40_000_000);
    expect(sparkY(0)).toBe(SPARK_HEIGHT);
    expect(sparkY(20_000_000)).toBe(SPARK_HEIGHT / 2);
    expect(sparkY(80_000_000)).toBe(0);
    expect(sparkPaths([0, 40_000_000], 120, 40).line).toBe('M0.0 40.0L120.0 0.0');
    // Fewer than two samples: a flat line
    expect(sparkPaths([], 120, 40).line).toBe('M0.0 40.0L120.0 40.0');
  });

  it('reads the sample under the pointer, and how long ago it was', () => {
    expect(sampleAt(60, 120, 31)).toBe(15);
    expect(sampleAt(-5, 120, 31)).toBe(0);
    expect(sampleAt(500, 120, 31)).toBe(30);
    const samples = [1_000_000, 6_400_000];
    expect(sparkTip(1, samples, 2)).toBe('Now · 6.4 MB/s');
    expect(sparkTip(0, samples, 2)).toBe('2 s ago · 1.0 MB/s');
  });
});

describe('files', () => {
  const severance: TorrentDetail = torrentDetail(STREAMING);

  it('sums them up and strips the torrent’s folder from each row', () => {
    const summary = filesSummary(severance.files);
    expect(summary).toMatchObject({ mapped: 10, ignored: 0, root: 'Severance.S02.2160p.ATVP.WEB-DL.DV.H.265/', text: '10 mapped' });
    expect(relativePath(severance.files[0].path, summary.root)).toBe('Severance.S02E01.Hello.Ms.Cobel.2160p.ATVP.WEB-DL.DV.H.265.mkv');
    const batch = torrentDetail(TORRENTS.find((t) => t.open_job !== null)!);
    expect(filesSummary(batch.files)).toMatchObject({ mapped: 0, ignored: 3, text: '0 mapped · 3 ignored' });
  });

  it('shows Ready, a percentage, Skipped, or nothing while it isn’t loaded', () => {
    const file = severance.files[0];
    expect(fileProgress(file)).toEqual({ kind: 'ready' });
    expect(fileProgress({ ...file, progress: 0.926 })).toEqual({ kind: 'partial', pct: 92 });
    expect(fileProgress({ ...file, progress: null })).toEqual({ kind: 'unknown' });
    expect(fileProgress({ ...file, plays_as: [], ignored: { reason: 'opening', label: 'Ignored (NCOP)' } })).toEqual({ kind: 'skipped' });
    // A subtitle that follows its video plays as nothing and isn't ignored
    expect(unmappedLabel('Sintel/Sintel.de.srt', false)).toBe('Subtitle');
    expect(unmappedLabel('Batch/Show - 29.mkv', true)).toBe('Not mapped yet');
    expect(unmappedLabel('Batch/extra.mkv', false)).toBe('Not mapped');
  });

  it('marks the file a session reads, but not a paused torrent’s', () => {
    expect([...readersByPath(severance).keys()]).toEqual([STREAMING_SESSION.file_path]);
    expect(severance.files.some((f) => f.path === STREAMING_SESSION.file_path)).toBe(true);
    expect(readersByPath({ ...severance, state: 'paused' }).size).toBe(0);
  });
});
