/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import {
  assignmentApi,
  askStream,
  hdrezkaApi,
  libraryApi,
  libraryIds,
  parseSseFrame,
  recApi,
  searchApi,
  settingsApi,
  subtitlesApi,
  systemApi,
  titlesApi,
  torrentsApi,
  traktApi,
} from '@/lib/api/endpoints';
import type { AskEvent, TorrentState } from '@/lib/api/contract';
import { setMockScenario } from '@/mocks/scenario';
import { episodeId } from '@/mocks/fixtures/titles';
import { mockRemovals } from '@/mocks/handlers/titles';
import { TORRENTS } from '@/mocks/fixtures/torrents';

// The endpoint modules call relative /api/* URLs, as the browser does; resolve
// them against a base. Wrap the fetch MSW installs on listen(), not the original.
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

const torrentWith = (state: TorrentState) => TORRENTS.find((t) => t.state === state)!.info_hash;
const DOWNLOADING = torrentWith('downloading');
const PAUSED = torrentWith('paused');
const STREAMING = torrentWith('streaming');
const FRIEREN_BATCH = TORRENTS.find((t) => t.open_job?.id === 101)!.info_hash; // not loaded, job 101 in review
const DUNE_TORRENT = TORRENTS.find((t) => !t.loaded && t.titles[0]?.media_type === 'movie')!.info_hash;

const INTERSTELLAR = 'https://rezka.ag/films/fiction/2259-interstellar-2014.html';
const SEVERANCE = 'https://hdrezka.me/series/thriller/47283-razdelenie-2022.html';
const BATTLE = 'https://rezka.ag/films/action/78655-bitva-za-bitvoy-2025.html';

describe('mock mode answers every PLAN §6 endpoint', () => {
  it.each([
    ['GET /titles/tv/{id}', () => titlesApi.get('tv', 209867)],
    ['GET /titles/movie/{id}', () => titlesApi.get('movie', 666277)],
    ['GET /titles/tv/{id}/seasons/{n}', () => titlesApi.season(37854, 22)],
    ['POST /movies', () => titlesApi.addMovie(666277)],
    ['POST /shows', () => titlesApi.addShow(66573)],
    ['DELETE /movies/{id}', () => titlesApi.removeMovie(3)],
    ['DELETE /shows/{id}', () => titlesApi.removeShow(6)],
    ['POST /movies/{id}/refresh', () => titlesApi.refreshMovie(1)],
    ['POST /shows/{id}/refresh', () => titlesApi.refreshShow(1)],
    ['DELETE /movies/{id}/assign', () => titlesApi.unassignMovie(1)],
    ['DELETE /episodes/{id}/assign', () => titlesApi.unassignEpisode(1028)],
    ['PATCH /shows/{id}', () => titlesApi.patchShow(1, { include_specials: false })],
    ['GET /shows/{id}/episode-groups', () => titlesApi.episodeGroups(1)],
    ['GET /movies/{id}/subtitles', () => subtitlesApi.list('movie', 1)],
    ['GET /episodes/{id}/subtitles/search', () => subtitlesApi.search('episode', 1028, ['en', 'az'])],
    ['POST /episodes/{id}/subtitles', () => subtitlesApi.download('episode', 1028, { file_id: 1, language_code: 'az' })],
    ['DELETE /movies/{id}/subtitles/{sid}', () => subtitlesApi.remove('movie', 1, 2)],
    ['GET /library', () => libraryApi.list({ type: 'tv' })],
    ['POST /shows/{id}/assign-torrent', () => assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:x' })],
    ['POST /movies/{id}/assign-torrent', () => assignmentApi.assignMovie(1, { magnet_uri: 'magnet:?xt=urn:btih:x' })],
    ['GET /assignment-jobs', () => assignmentApi.jobs()],
    ['GET /assignment-jobs/{id}', () => assignmentApi.job(101)],
    ['PATCH /assignment-jobs/{id}/proposals', async () => {
      const { proposals } = await assignmentApi.job(101);
      return assignmentApi.patchProposals(101, { items: [{ id: proposals[0].id, decision: 'ignored' }] });
    }],
    ['POST /assignment-jobs/{id}/reclassify', () => assignmentApi.reclassify(101, { scope: 'unresolved', hint: 'absolute numbers' })],
    ['POST /assignment-jobs/{id}/apply', async () => {
      const { proposals } = await assignmentApi.job(101);
      const pending = proposals.filter((p) => p.decision === 'pending' && p.targets.length > 0);
      await assignmentApi.patchProposals(101, { items: pending.map((p) => ({ id: p.id, decision: 'accepted' as const })) });
      return assignmentApi.apply(101);
    }],
    ['POST /assignment-jobs/{id}/discard', () => assignmentApi.discard(103)],
    ['GET /shows/{id}/episodes', () => assignmentApi.showEpisodes(1)],
    ['PUT /episodes/{id}/assignment', () => assignmentApi.setEpisodeAssignment(1029, { info_hash: 'x', file_path: 'a.mkv' })],
    ['PUT /movies/{id}/assignment', () => assignmentApi.setMovieAssignment(1, { info_hash: 'x', file_path: 'a.mkv' })],
    ['GET /torrents/{hash}/files', async () => assignmentApi.torrentFiles((await assignmentApi.job(101)).job.torrent.info_hash)],
    ['GET /search/torrents', () => searchApi.torrents({ media_type: 'tv', tmdb_id: 209867, season: 1, episode: 29 })],
    ['POST /search/torrents/resolve', () => searchApi.resolve({ release_id: 'rel-0' })],
    ['POST /search/torrents/inspect', () => searchApi.inspect({ release_id: 'rel-0', media_type: 'tv', tmdb_id: 209867 })],
    ['GET /settings/quality-profile', () => searchApi.qualityProfile()],
    ['PUT /settings/quality-profile', async () => searchApi.putQualityProfile({ ...(await searchApi.qualityProfile()), max_gb_per_hour: 6 })],
    ['GET /integrations/trakt', () => traktApi.integration()],
    ['POST /integrations/trakt/connect', async () => {
      await traktApi.disconnect(); // 409 while connected, as the mock starts
      return traktApi.connect();
    }],
    ['POST /integrations/trakt/sync', () => traktApi.sync()],
    ['PUT /integrations/trakt/settings', () => traktApi.updateSettings({ include_paused: true })],
    ['DELETE /integrations/trakt', () => traktApi.disconnect()],
    ['GET /me/up-next', () => traktApi.upNext()],
    ['GET /me/new-episodes', () => traktApi.newEpisodes(7)],
    ['GET /me/watchlist', () => traktApi.watchlist('movie')],
    ['GET /me/history', () => traktApi.history(2)],
    ['GET /recommendations/home', () => recApi.home()],
    ['POST /recommendations/refresh', () => recApi.refresh()],
    ['GET /recommendations/runs', () => recApi.runs(5)],
    ['GET /recommendations/runs/{id}', () => recApi.run(42)],
    ['GET /recommendations/threads', () => recApi.threads()],
    ['GET /recommendations/threads/{id}', () => recApi.thread(12)],
    ['POST /recommendations/feedback', () => recApi.feedback({ media_type: 'tv', tmdb_id: 95396, signal: 'like', source: 'title' })],
    ['DELETE /recommendations/feedback/{media_type}/{tmdb_id}', () => recApi.clearFeedback('tv', 95396)],
    ['GET /settings/ai', () => settingsApi.ai()],
    ['PUT /settings/ai', () => settingsApi.putAi({ rec_schedule: 'weekly' })],
    ['GET /settings/ai/usage', () => settingsApi.aiUsage()],
    ['GET /status', () => systemApi.status()],
    ['GET /torrents', () => torrentsApi.list()],
    ['GET /torrents/summary', () => torrentsApi.summary()],
    ['GET /torrents/{hash}', () => torrentsApi.detail(DOWNLOADING)],
    ['POST /shows/{id}/assign-torrent {info_hash}', () => assignmentApi.assignShow(1, { info_hash: FRIEREN_BATCH })],
    ['POST /torrents/{hash}/pause', () => torrentsApi.pause(DOWNLOADING)],
    ['POST /torrents/{hash}/resume', () => torrentsApi.resume(PAUSED)],
    ['POST /torrents/pause-all', () => torrentsApi.pauseAll()],
    ['POST /torrents/resume-all', () => torrentsApi.resumeAll()],
    ['DELETE /torrents/{hash}', () => torrentsApi.remove(DOWNLOADING, { deleteData: true })],
    ['GET /hdrezka/search', () => hdrezkaApi.search({ media_type: 'movie', tmdb_id: 157336 })],
    ['GET /hdrezka/titles', () => hdrezkaApi.title({ url: INTERSTELLAR, movie_id: 2 })],
    ['GET /hdrezka/streams', () => hdrezkaApi.streams({ url: INTERSTELLAR, translator_id: 238 })],
    ['POST /movies/{id}/assign-hdrezka', () => hdrezkaApi.assignMovie(2, { url: INTERSTELLAR, translator_id: 238, quality: '1080p' })],
    ['POST /shows/{id}/assign-hdrezka', () =>
      hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 238, quality: '720p', items: [{ episode_id: episodeId(95396, 2, 9), season: 2, episode: 9 }] })],
  ])('%s', async (_name, call) => {
    await call(); // throws an ApiError on any non-2xx answer
  });

  it('streams Ask as typed events, ending with done', async () => {
    const events: AskEvent[] = [];
    for await (const event of askStream({ message: 'something like Severance but lighter' })) events.push(event);
    const names = events.map((e) => e.event);
    expect(names[0]).toBe('start');
    expect(names.at(-1)).toBe('done');
    // start → status… → text… → item… → done
    expect(names.join(' ').replace(/(\w+)( \1)+/g, '$1')).toBe('start status text item done');
    const done = events.at(-1);
    expect(done?.event === 'done' && done.data.refinements).toContain('Lighter');
  });
});

describe('GET /library (#97)', () => {
  it('carries counts and the summary, and every item the card needs', async () => {
    const page = await libraryApi.list();
    expect(page.counts).toEqual({ all: 9, movie: 3, tv: 6, watching: 4, ready: 5, incomplete: 4, needs_review: 3 });
    expect(page.summary).toMatchObject({ total: 9, ready: 5 });
    expect(page.items.map((item) => item.title)[0]).toBe('Interstellar'); // newest first
    const byTitle = new Map(page.items.map((item) => [item.title, item]));
    expect(byTitle.get('Interstellar')).toMatchObject({ runtime: 169, resolution: '2160p', download: { pct: 42, info_hash: expect.any(String) }, streaming: false });
    expect(byTitle.get('One Battle After Another')).toMatchObject({ availability_stage: 'needs_torrent', webdav_path: null, size_bytes: 0 });
    expect(byTitle.get('Frieren: Beyond Journey’s End')).toMatchObject({
      needs_review: 2,
      review_job_id: 101,
      missing: { season_number: 1, from: 29, to: 38, count: 10 },
      webdav_url: expect.stringContaining('/TV%20Shows/'),
    });
    expect(byTitle.get('The Studio')).toMatchObject({ streaming: true, download: { pct: expect.any(Number) } });
  });

  it('filters, sorts and pages; the counts honour only the search', async () => {
    const shows = await libraryApi.list({ type: 'tv', status: 'needs_review', sort: 'title' });
    expect(shows.items.map((item) => item.title)).toEqual(['Doctor Who', 'Frieren: Beyond Journey’s End', 'One Piece']);
    expect(shows.counts.all).toBe(9);
    const searched = await libraryApi.list({ q: 'one' });
    expect(searched.items.map((item) => item.title).sort()).toEqual(['One Battle After Another', 'One Piece']);
    expect(searched.counts).toMatchObject({ all: 2, movie: 1, tv: 1 });
    expect(searched.summary.total).toBe(9);
    const second = await libraryApi.list({ sort: 'title', page: 2, page_size: 4 });
    expect(second).toMatchObject({ page: 2, page_size: 4, total: 9 });
    expect(second.items).toHaveLength(4);
    const watching = await libraryApi.list({ status: 'watching', sort: 'title' });
    expect(watching.items.map((item) => [item.title, item.episodes_watched, item.episodes_total])).toEqual([
      ['Doctor Who', 29, 52],
      ['Frieren: Beyond Journey’s End', 26, 38],
      ['One Piece', 175, 192],
      ['The Studio', 9, 10],
    ]);
  });

  it('watch state from the Trakt account: episodes with a play, the last play, and the last-watched sort', async () => {
    const lastWatched = await libraryApi.list({ sort: 'last_watched' });
    expect(lastWatched.items.slice(0, 4).map((item) => [item.title, item.last_watched_at !== null])).toEqual([
      ['The Studio', true],
      ['Dune: Part Two', true], // the same day as Frieren: then newest added first
      ['Frieren: Beyond Journey’s End', true],
      ['One Piece', true],
    ]);
    expect(lastWatched.items.at(-1)?.last_watched_at).toBeNull();
    const byTitle = new Map(lastWatched.items.map((item) => [item.title, item]));
    expect(byTitle.get('Severance')).toMatchObject({ episodes_watched: 18, episodes_total: 18 });
    expect(byTitle.get('Bluey')).toMatchObject({ episodes_watched: 0, last_watched_at: null });

    // Off or purged: nothing is watched
    setMockScenario('trakt-off');
    const off = await libraryApi.list();
    expect(off.counts.watching).toBe(0);
    expect(off.items.every((item) => !item.episodes_watched && item.last_watched_at === null)).toBe(true);
    expect((await titlesApi.get('tv', 209867)).watch_state).toBeNull();
  });

  it('looks titles up by ids', async () => {
    const page = await libraryApi.list({ ids: libraryIds([{ media_type: 'movie', tmdb_id: 693134 }, { media_type: 'tv', tmdb_id: 209867 }, { media_type: 'movie', tmdb_id: 666277 }]) });
    expect(page.items.map((item) => item.tmdb_id).sort()).toEqual([209867, 693134]);
    expect(page).toMatchObject({ page: 1, page_size: 3, total: 2 });
  });

  it.each([
    ['an unknown type', { type: 'music' }],
    ['an unknown status', { status: 'done' }],
    ['an unknown sort', { sort: 'rating' }],
    ['page 0', { page: 0 }],
    ['malformed ids', { ids: '603' }],
    ['201 ids', { ids: Array.from({ length: 201 }, (_, i) => `tv:${i + 1}`).join(',') }],
  ])('answers 400 bad_request for %s', async (_name, params) => {
    await expect(libraryApi.list(params as never)).rejects.toMatchObject({ status: 400, code: 'bad_request' });
  });

  it('removes with ?delete_data=, which it records', async () => {
    await titlesApi.removeMovie(2, { deleteData: true });
    await titlesApi.removeShow(1, { deleteData: false });
    await titlesApi.removeMovie(1);
    expect(mockRemovals()).toEqual([
      { media_type: 'movie', id: 2, delete_data: true },
      { media_type: 'tv', id: 1, delete_data: false },
      { media_type: 'movie', id: 1, delete_data: null },
    ]);
    expect((await libraryApi.list()).summary.total).toBe(6);
  });
});

describe('Activity in mock mode (B6, #98)', () => {
  it('has a torrent in every state, and idle ones the client has not loaded', () => {
    expect(new Set(TORRENTS.map((t) => t.state))).toEqual(new Set(['streaming', 'downloading', 'paused', 'idle']));
    const notLoaded = TORRENTS.filter((t) => !t.loaded);
    expect(notLoaded.length).toBeGreaterThan(0);
    for (const t of notLoaded) {
      expect(t).toMatchObject({ state: 'idle', progress: null, complete: null, peers: null, seeders: null, last_active_at: null, download_bps: 0 });
    }
    expect(TORRENTS.some((t) => t.loaded && t.titles.length === 0)).toBe(true); // an Inspect dry run
    expect(TORRENTS.some((t) => t.open_job !== null)).toBe(true);
  });

  it('answers GET /torrents with the B6 items, and the offline scenario as the proxy does', async () => {
    const { items } = await torrentsApi.list();
    expect(items.map((t) => t.info_hash)).toEqual(TORRENTS.map((t) => t.info_hash));
    setMockScenario('torrents-offline');
    await expect(torrentsApi.list()).rejects.toMatchObject({ status: 502, code: 'backend_unreachable' });
    await expect(torrentsApi.summary()).rejects.toMatchObject({ status: 502 });
  });

  it('pauses and resumes a torrent, and the summary counts follow', async () => {
    expect((await torrentsApi.summary()).counts).toEqual({ streaming: 1, downloading: 2, paused: 1, idle: 4 });
    expect(await torrentsApi.pause(DOWNLOADING.toUpperCase())).toMatchObject({ info_hash: DOWNLOADING, state: 'paused', download_bps: 0 });
    expect((await torrentsApi.summary()).counts).toMatchObject({ downloading: 1, paused: 2 });
    expect(TORRENTS.find((t) => t.info_hash === DOWNLOADING)?.state).toBe('downloading'); // the page's copy changed, not the fixture
    expect((await torrentsApi.pause(DOWNLOADING)).state).toBe('paused'); // idempotent
    expect((await torrentsApi.resume(DOWNLOADING)).state).toBe('downloading');
    expect((await torrentsApi.resume(PAUSED)).state).toBe('downloading'); // paused from the start, loaded and partial
  });

  it('pauses the streaming and downloading torrents, and resumes every paused one', async () => {
    expect(await torrentsApi.pauseAll()).toEqual({ count: 3 });
    expect(await torrentsApi.pauseAll()).toEqual({ count: 0 });
    expect(await torrentsApi.resumeAll()).toEqual({ count: 4 });
    expect((await torrentsApi.summary()).counts).toEqual({ streaming: 1, downloading: 3, paused: 0, idle: 4 });
  });

  it('deletes a torrent, and rejects unknown and malformed hashes', async () => {
    await torrentsApi.remove(DOWNLOADING);
    expect((await torrentsApi.list()).items.map((t) => t.info_hash)).not.toContain(DOWNLOADING);
    await expect(torrentsApi.pause(DOWNLOADING)).rejects.toMatchObject({ status: 404 });
    await expect(torrentsApi.resume('summary')).rejects.toMatchObject({ status: 400 });
  });
});

describe('Streaming sessions, the torrent detail and Re-run mapping in mock mode (B6, #99)', () => {
  it('has Infuse on the Apple TV streaming an episode, first in the summary, with an HDRezka session after it', async () => {
    const streaming = TORRENTS.find((t) => t.info_hash === STREAMING)!.streaming;
    expect(streaming).toHaveLength(1);
    expect(streaming[0]).toMatchObject({
      source: 'torrent',
      info_hash: STREAMING,
      client: 'Infuse',
      device: 'Apple TV',
      item: { media_type: 'tv', title: 'Severance', season_number: 2, episode_number: 5, episode_name: 'Trojan’s Horse' },
    });
    expect(streaming[0].buffered_ahead_bytes).toBeGreaterThan(0);
    expect(streaming[0].position_seconds).toBeLessThan(streaming[0].duration_seconds!);
    for (const t of TORRENTS.filter((other) => other.info_hash !== STREAMING)) expect(t.streaming).toEqual([]);

    const summary = await torrentsApi.summary();
    expect(summary.streaming.map((s) => s.source)).toEqual(['torrent', 'hdrezka']); // the most recently read first
    expect(summary.streaming[0]).toEqual(streaming[0]);
    expect(summary.streaming[1]).toMatchObject({ info_hash: null, file_path: null, buffered_ahead_bytes: null, buffered_ahead_seconds: null });

    await torrentsApi.remove(STREAMING); // its sessions go with it
    expect((await torrentsApi.summary()).streaming.map((s) => s.source)).toEqual(['hdrezka']);
  });

  it('details a loaded torrent: pieces, progress, totals, and the file Infuse reads', async () => {
    const d = await torrentsApi.detail(STREAMING.toUpperCase());
    expect(d).toMatchObject({ info_hash: STREAMING, state: 'streaming', loaded: true, swarm: { connected_peers: 12, total_seeders: null } });
    expect(d.pieces!.reduce((sum, r) => sum + r.count, 0)).toBe(d.pieces_total);
    expect(d.pieces_have).toBe(d.pieces_total); // complete
    expect(d.files).toHaveLength(10);
    const reading = d.files.find((f) => f.path === d.streaming[0].file_path)!;
    expect(reading).toMatchObject({ progress: 1, plays_as: [{ label: 'S02E05', episode_name: 'Trojan’s Horse' }], ignored: null });
    expect(d.trackers.every((t) => t.status === 'unknown')).toBe(true);

    const fetching = await torrentsApi.detail(DOWNLOADING);
    expect(fetching.pieces!.map((r) => r.state)).toEqual(['have', 'partial', 'priority', 'missing']);
    expect(fetching.ratio).toBeGreaterThan(0);
  });

  it('details a torrent the client has not loaded: no pieces, null progress, and why files are ignored', async () => {
    const d = await torrentsApi.detail(FRIEREN_BATCH);
    expect(d).toMatchObject({ loaded: false, pieces: null, pieces_have: null, downloaded_bytes: null, ratio: null, swarm: null });
    expect(d.files.every((f) => f.progress === null)).toBe(true);
    expect(d.files.map((f) => f.ignored?.label).filter(Boolean)).toEqual(['Ignored (NCOP)', 'Ignored (NCED)', 'Ignored (font)']);
    expect((await torrentsApi.detail(DUNE_TORRENT)).files[0].plays_as).toEqual([expect.objectContaining({ media_type: 'movie', label: 'Movie' })]);

    await torrentsApi.pause(DOWNLOADING);
    expect((await torrentsApi.detail(DOWNLOADING)).state).toBe('paused'); // the page's state
    await expect(torrentsApi.detail('0'.repeat(40))).rejects.toMatchObject({ status: 404 });
    await expect(torrentsApi.detail('not-a-hash')).rejects.toMatchObject({ status: 400 });
  });

  it('re-runs a mapping by info hash: the open job, or a new one from the stored magnet', async () => {
    expect(await assignmentApi.assignShow(1, { info_hash: FRIEREN_BATCH.toUpperCase() })).toEqual({ job_id: 101 }); // job 101 is open
    const { job_id: jobId } = await assignmentApi.assignMovie(1, { info_hash: DUNE_TORRENT });
    expect(jobId).toBeGreaterThan(103);
    expect((await assignmentApi.job(jobId)).job.torrent.info_hash).toBe(DUNE_TORRENT);

    await expect(assignmentApi.assignShow(1, { info_hash: '0'.repeat(40) })).rejects.toMatchObject({ status: 404 });
    await expect(assignmentApi.assignShow(1, { info_hash: 'summary' })).rejects.toMatchObject({ status: 400 });
    await expect(assignmentApi.assignShow(1, {} as { info_hash: string })).rejects.toMatchObject({ status: 400 });
  });
});

describe('the mock library keeps what the page changed', () => {
  it('walks a movie from not in the library to needs a torrent and back, in /library too', async () => {
    const listed = async () => (await libraryApi.list({ type: 'movie' })).items.map((item) => item.tmdb_id);
    expect((await titlesApi.get('movie', 666277)).library.availability_stage).toBe('none');
    expect(await listed()).not.toContain(666277);
    const added = await titlesApi.addMovie(666277);
    expect((await titlesApi.get('movie', 666277)).library).toMatchObject({ library_id: added.id, availability_stage: 'needs_torrent', webdav_path: null });
    expect(await listed()).toContain(666277);
    await titlesApi.removeMovie(added.id);
    expect((await titlesApi.get('movie', 666277)).library.availability_stage).toBe('none');
    expect(await listed()).not.toContain(666277);
  });

  it('answers 400 bad_request for a TMDB id it doesn’t know, as the legacy backend does', async () => {
    await expect(titlesApi.addShow(1)).rejects.toMatchObject({ status: 400, code: 'bad_request' });
  });

  it('opens each show on its default season and orders anime by absolute number', async () => {
    const defaults = async () => {
      const shows = await Promise.all([209867, 57243, 37854, 95396].map((id) => titlesApi.get('tv', id)));
      return shows.map((show) => show.media_type === 'tv' && [show.default_season, show.episode_ordering]);
    };
    // Trakt's next episode's season: Doctor Who S03E04, One Piece's rewatch at Wano (#892)
    expect(await defaults()).toEqual([
      [1, 'absolute'],
      [3, 'aired'],
      [21, 'absolute'],
      [2, 'aired'],
    ]);
    // Without Trakt: the frontier's, else the latest season with a file
    setMockScenario('trakt-off');
    expect(await defaults()).toEqual([
      [1, 'absolute'],
      [4, 'aired'],
      [22, 'absolute'],
      [2, 'aired'],
    ]);
  });

  it('promises no finish time while downloading', async () => {
    const titles = await Promise.all([titlesApi.get('movie', 157336), titlesApi.get('tv', 247767)]);
    expect(titles.map((title) => title.library.availability_stage)).toEqual(['downloading', 'downloading']);
    expect(titles.map((title) => title.library.download?.eta_seconds)).toEqual([null, null]);
  });

  it('gives a ready movie a path Infuse can open, until its torrent is unassigned', async () => {
    expect((await titlesApi.get('movie', 693134)).library).toMatchObject({
      availability_stage: 'ready',
      webdav_path: '/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mkv',
      webdav_url: 'https://momoshtrem.local:36911/Movies/Dune-%20Part%20Two%20(2024)/Dune-%20Part%20Two%20(2024).mkv',
    });
    await titlesApi.unassignMovie(1);
    expect((await titlesApi.get('movie', 693134)).library).toMatchObject({ availability_stage: 'needs_torrent', torrent: null, webdav_url: null });
  });

  it('reports Frieren’s frontier, files to review and season blocks', async () => {
    expect((await titlesApi.get('tv', 209867)).library).toMatchObject({
      availability_stage: 'needs_torrent',
      missing: { season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10 },
      needs_review: 2,
      review_job_id: 101,
    });
    const season = await titlesApi.season(209867, 1);
    // S01E35 is under review (job 101), so it isn't counted as missing, though it's in the frontier
    expect(season.ranges.map((range) => [range.label, range.missing])).toEqual([['1–10', 0], ['11–20', 0], ['21–28', 0], ['29–38', 9]]);
  });

  it('opens a frontier when the last episode loses its file, but not for a gap', async () => {
    setMockScenario('trakt-off'); // Severance's episodes are all watched, and watched ones count as covered
    await titlesApi.unassignEpisode(302005); // Severance S02E05
    expect((await titlesApi.get('tv', 95396)).library).toMatchObject({ availability_stage: 'ready', missing: null });
    await titlesApi.unassignEpisode(302009); // S02E09; E10 hasn't aired
    expect((await titlesApi.get('tv', 95396)).library).toMatchObject({
      availability_stage: 'needs_torrent',
      missing: { season_number: 2, from: 9, to: 9, count: 1 },
    });
  });

  it('keeps a watched episode that loses its file covered: watched, without a file, and no frontier', async () => {
    await titlesApi.unassignEpisode(302009); // Severance S02E09
    expect(await titlesApi.get('tv', 95396)).toMatchObject({
      library: { availability_stage: 'ready', missing: null, episodes_ready: 17 },
      seasons: [{ season_number: 1, ready: 9 }, { season_number: 2, ready: 8, missing: 0, upcoming: 1 }],
    });
    const season = await titlesApi.season(95396, 2);
    expect(season.episodes[8]).toMatchObject({ state: 'watched', webdav_path: null, source: null, pct: null });
  });

  it('gives the missing run absolute numbers, so the hero can offer #1121 from the title alone', async () => {
    await titlesApi.unassignEpisode(122036); // One Piece S22E36 (#1121); E37–E38 haven't aired
    expect((await titlesApi.get('tv', 37854)).library).toMatchObject({
      availability_stage: 'needs_torrent',
      missing: { season_number: 22, from: 36, to: 36, absolute_from: 1121, absolute_to: 1121, count: 1 },
    });
  });

  it('answers 404 not_in_library for the seasons of a show that isn’t added', async () => {
    await expect(titlesApi.season(66573, 1)).rejects.toMatchObject({ status: 404, code: 'not_in_library' });
  });

  it('rejects episode-group ordering without a group', async () => {
    await expect(titlesApi.patchShow(1, { episode_ordering: 'episode_group' })).rejects.toMatchObject({ status: 422, code: 'validation_error' });
  });

  it('hides specials while they are off', async () => {
    const show = await titlesApi.patchShow(3, { include_specials: false });
    expect(show.seasons.map((season) => season.season_number)).toEqual([1, 2, 3, 4]);
    await expect(titlesApi.season(57243, 0)).rejects.toMatchObject({ status: 404, code: 'not_found' });
  });

  it('ranks subtitle candidates and keeps one download per language', async () => {
    const { items } = await subtitlesApi.search('movie', 1, ['az']);
    expect(items.map((item) => item.match)).toEqual(['exact_release', 'same_source', 'different_timing']);
    const subtitle = await subtitlesApi.download('movie', 1, { file_id: items[0].file_id, language_code: 'az' });
    expect(subtitle).toMatchObject({ source: 'opensubtitles', match: 'exact_release' });
    await subtitlesApi.download('movie', 1, { file_id: items[1].file_id, language_code: 'az' });
    const { items: stored } = await subtitlesApi.list('movie', 1);
    expect(stored.map((s) => `${s.language_code}:${s.match}`)).toEqual(['en:exact_release', 'ru:exact_release', 'az:same_source']);
  });

  it('assigns a torrent from the search: one plausible file applies on its own and starts downloading', async () => {
    const search = await searchApi.torrents({ media_type: 'movie', tmdb_id: 1054867 });
    const [best] = search.items;
    expect(best.id).toBe(search.best_id);
    if (!best.magnet_uri) throw new Error('The best match has a magnet');
    const { job_id: jobId } = await assignmentApi.assignMovie(3, { magnet_uri: best.magnet_uri, indexer: best.indexer });
    // In Node a job moves one step per read: fetching metadata, classifying, then applied
    for (let read = 0; read < 2; read++) await assignmentApi.job(jobId);
    expect((await assignmentApi.job(jobId)).job).toMatchObject({ status: 'applied', ai_available: false, torrent: { indexer: best.indexer } });
    expect((await titlesApi.get('movie', 1054867)).library).toMatchObject({
      availability_stage: 'downloading',
      torrent: { release_name: 'One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', state: 'active' },
      download: { pct: 0, eta_seconds: null },
    });
  });

  it('keeps subtitles that came with the torrent', async () => {
    await expect(subtitlesApi.remove('movie', 1, 1)).rejects.toMatchObject({ status: 409, code: 'conflict' });
  });

  it('drops torrent subtitles and stops ranking once the file is unassigned', async () => {
    await titlesApi.unassignMovie(1);
    const search = await subtitlesApi.search('movie', 1, ['az']);
    expect(search.release_name).toBeNull();
    expect(new Set(search.items.map((item) => item.match))).toEqual(new Set(['unknown']));
    const { items: stored } = await subtitlesApi.list('movie', 1);
    expect(stored.map((s) => `${s.language_code}:${s.source}:${s.match}`)).toEqual(['ru:opensubtitles:unknown']);
  });
});

describe('torrent search in mock mode (#6)', () => {
  it('resolves a release to its magnet, and says when a result expired', async () => {
    const { items } = await searchApi.torrents({ media_type: 'tv', tmdb_id: 209867, season: 1, episode: 29 });
    expect(await searchApi.resolve({ release_id: items[0].id })).toEqual({ magnet_uri: items[0].magnet_uri, info_hash: items[0].info_hash });
    await expect(searchApi.resolve({ release_id: 'gone' })).rejects.toMatchObject({ status: 404, code: 'not_found' });
  });

  it('keeps a saved quality profile until the state is reset', async () => {
    const profile = await searchApi.qualityProfile();
    await searchApi.putQualityProfile({ ...profile, preferred_resolution: '1080p', max_resolution: null });
    expect(await searchApi.qualityProfile()).toMatchObject({ preferred_resolution: '1080p', max_resolution: null });
    resetMockState();
    expect(await searchApi.qualityProfile()).toEqual(profile);
  });
});

describe('HDRezka in mock mode (#63)', () => {
  it('finds the TMDB title best first, and nothing for "zzz"', async () => {
    const { items, query } = await hdrezkaApi.search({ media_type: 'movie', tmdb_id: 693134 });
    expect(query).toBe('Dune: Part Two');
    expect(items.map((item) => [item.title, item.year, item.match])).toEqual([
      ['Дюна: Часть вторая', 2024, 'exact'],
      ['Дюна', 2021, 'likely'],
      ['Дюна', 1984, 'weak'],
    ]);
    expect((await hdrezkaApi.search({ media_type: 'movie', tmdb_id: 693134, q: 'interstellar 2014' })).items[0]).toMatchObject({ title: 'Интерстеллар', match: 'exact' });
    expect((await hdrezkaApi.search({ media_type: 'movie', tmdb_id: 693134, q: 'zzz' })).items).toEqual([]);
  });

  it('opens a title on any mirror, flags its voice-overs and prices each quality', async () => {
    const title = await hdrezkaApi.title({ url: BATTLE.replace('rezka.ag', 'hdrezka.me'), movie_id: 3 });
    expect(title).toMatchObject({ post_id: 78655, kind: 'movie', translator_id: 111, seasons: [] });
    expect(title.translators.filter((t) => t.premium || t.ads || t.camrip || t.director).map((t) => t.name)).toEqual(['Дубляж', 'TVShows', 'Red Head Sound', 'HDrezka Studio']);
    const streams = await hdrezkaApi.streams({ url: BATTLE, translator_id: 405 });
    expect(streams.qualities.map((q) => [q.label, q.available && !q.premium])).toEqual([
      ['360p', true],
      ['480p', true],
      ['720p', true],
      ['1080p', false],
      ['1080p Ultra', false],
    ]);
    expect((await hdrezkaApi.streams({ url: BATTLE, translator_id: 238 })).subtitles.map((s) => s.language_name)).toEqual(['English', 'Русский']);
    await expect(hdrezkaApi.title({ url: 'https://rezka.ag/films/drama/1-nope.html' })).rejects.toMatchObject({ status: 404, code: 'not_found' });
  });

  it('streams a movie: ready at once, the torrent replaced, an .mp4 in Infuse, and no release to rank subtitles against', async () => {
    await hdrezkaApi.assignMovie(1, { url: 'https://rezka.ag/films/fiction/64829-dyuna-chast-vtoraya-2024.html', translator_id: 238, quality: '1080p' });
    const { library } = await titlesApi.get('movie', 693134);
    expect(library).toMatchObject({
      availability_stage: 'ready',
      torrent: null,
      stream: { provider: 'hdrezka', title: 'Dune: Part Two', translator: 'Оригинал (+субтитры)', quality: '1080p', size_bytes: 4_453_333_333 },
      webdav_path: '/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mp4',
    });
    expect((await subtitlesApi.search('movie', 1, ['en'])).release_name).toBeNull();
    await titlesApi.unassignMovie(1);
    expect((await titlesApi.get('movie', 693134)).library).toMatchObject({ availability_stage: 'needs_torrent', stream: null });
  });

  it('refuses premium voice-overs and qualities with 409 premium_only', async () => {
    await expect(hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 56, quality: '1080p' })).rejects.toMatchObject({ status: 409, code: 'premium_only' });
    await expect(hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 238, quality: '1080p Ultra' })).rejects.toMatchObject({ status: 409, code: 'premium_only' });
    setMockScenario('hdrezka-premium');
    expect((await hdrezkaApi.title({ url: BATTLE })).translators.every((t) => t.premium)).toBe(true);
  });

  it.each([
    ['hdrezka-blocked', 503, 'hdrezka_blocked'],
    ['hdrezka-down', 502, 'hdrezka_unavailable'],
    ['hdrezka-off', 503, 'unavailable'],
  ] as const)('%s: every call fails with %s %s, never 401 or 403', async (name, status, code) => {
    setMockScenario(name);
    await expect(hdrezkaApi.search({ media_type: 'movie', tmdb_id: 1054867 })).rejects.toMatchObject({ status, code });
    await expect(hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 238, quality: '1080p' })).rejects.toMatchObject({ status, code });
    expect((await systemApi.status()).hdrezka).toMatchObject({ configured: name !== 'hdrezka-off', reachable: false });
  });

  it('streams episodes, marks their source and reports the ones that failed', async () => {
    setMockScenario('hdrezka-partial');
    const items = [5, 6, 7].map((episode) => ({ episode_id: episodeId(95396, 2, episode), season: 2, episode }));
    const result = await hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 10, quality: '1080p', items });
    expect(result.assigned.map((a) => a.episode_id)).toEqual([items[0].episode_id, items[1].episode_id]);
    expect(result.failed).toEqual([{ episode_id: items[2].episode_id, code: 'timeout', message: expect.any(String) }]);
    const season = await titlesApi.season(95396, 2);
    expect(season.episodes.slice(4, 7).map((e) => [e.source, e.stream?.translator ?? null])).toEqual([
      ['hdrezka', 'LostFilm'],
      ['hdrezka', 'LostFilm'],
      ['torrent', null],
    ]);
    const title = await hdrezkaApi.title({ url: SEVERANCE, show_id: 4, translator_id: 10 });
    expect(title.seasons[1].episodes[4].library).toMatchObject({ season: 2, episode: 5, has_file: true, source: 'hdrezka', stream: { quality: '1080p' } });
    await expect(hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 10, quality: '1080p', items: [] })).rejects.toMatchObject({ status: 400 });
    await expect(hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 10, quality: '1080p', items: [items[0], items[0]] })).rejects.toMatchObject({ status: 400 });

    // An episode fails once: retrying it gets it through
    const retried = await hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 10, quality: '1080p', items: [items[2]] });
    expect(retried).toEqual({ assigned: [expect.objectContaining({ episode_id: items[2].episode_id })], failed: [] });
  });

  it('tells a translator’s cuts apart by their flags, and takes the first without them', async () => {
    const theatrical = await hdrezkaApi.streams({ url: BATTLE, translator_id: 111 });
    const director = await hdrezkaApi.streams({ url: BATTLE, translator_id: 111, camrip: false, ads: false, director: true });
    const size = (streams: typeof theatrical) => streams.qualities.find((q) => q.label === '1080p')?.size_bytes;
    expect(size(theatrical)).toBe(4_320_000_000);
    expect(size(director)).toBe(4_960_000_000);
    expect(size(await hdrezkaApi.streams({ url: BATTLE, translator_id: 111, camrip: false, ads: false, director: false }))).toBe(size(theatrical));

    await hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 111, camrip: false, ads: false, director: true, quality: '1080p' });
    expect((await titlesApi.get('movie', 1054867)).library.stream).toMatchObject({ translator: 'HDrezka Studio', size_bytes: 4_960_000_000 });
    await expect(hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 111, camrip: true, quality: '720p' })).rejects.toMatchObject({ status: 422 });
    // A flag left out doesn't constrain: director alone finds the cut
    expect(size(await hdrezkaApi.streams({ url: BATTLE, translator_id: 111, director: true }))).toBe(4_960_000_000);
  });
});

describe('parseSseFrame', () => {
  it('reads event and JSON data, ignoring comments', () => {
    expect(parseSseFrame(': keep-alive\nevent: text\ndata: {"delta":"hi"}')).toEqual({ event: 'text', data: { delta: 'hi' } });
  });

  it('skips frames without data or with broken JSON', () => {
    expect(parseSseFrame('event: ping')).toBeNull();
    expect(parseSseFrame('data: {nope')).toBeNull();
  });
});
