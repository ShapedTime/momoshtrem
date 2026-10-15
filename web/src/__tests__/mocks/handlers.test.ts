/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import {
  assignmentApi,
  askStream,
  hdrezkaApi,
  libraryApi,
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
import type { AskEvent } from '@/lib/api/contract';
import { setMockScenario } from '@/mocks/scenario';
import { episodeId } from '@/mocks/fixtures/titles';

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
    ['POST /search/torrents/inspect', () => searchApi.inspect({ release_id: 'rel-0', media_type: 'tv', tmdb_id: 209867 })],
    ['GET /settings/quality-profile', () => searchApi.qualityProfile()],
    ['GET /integrations/trakt', () => traktApi.integration()],
    ['POST /integrations/trakt/connect', () => traktApi.connect()],
    ['POST /integrations/trakt/sync', () => traktApi.sync()],
    ['DELETE /integrations/trakt', () => traktApi.disconnect()],
    ['GET /me/up-next', () => traktApi.upNext()],
    ['GET /me/watchlist', () => traktApi.watchlist('movie')],
    ['GET /me/history', () => traktApi.history(2)],
    ['GET /recommendations/home', () => recApi.home()],
    ['POST /recommendations/refresh', () => recApi.refresh()],
    ['GET /recommendations/runs/{id}', () => recApi.run(42)],
    ['GET /recommendations/threads', () => recApi.threads()],
    ['GET /recommendations/threads/{id}', () => recApi.thread(7)],
    ['POST /recommendations/feedback', () => recApi.feedback({ media_type: 'tv', tmdb_id: 95396, signal: 'like' })],
    ['GET /settings/ai', () => settingsApi.ai()],
    ['GET /settings/ai/usage', () => settingsApi.aiUsage()],
    ['GET /status', () => systemApi.status()],
    ['GET /torrents/summary', () => torrentsApi.summary()],
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
    expect(events.map((e) => e.event)).toEqual(['status', 'status', 'item', 'text', 'text', 'done']);
    const done = events.at(-1);
    expect(done?.event === 'done' && done.data.refinements).toContain('lighter');
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
    const shows = await Promise.all([209867, 57243, 37854, 95396].map((id) => titlesApi.get('tv', id)));
    expect(shows.map((show) => show.media_type === 'tv' && [show.default_season, show.episode_ordering])).toEqual([
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
    await titlesApi.unassignEpisode(302005); // Severance S02E05
    expect((await titlesApi.get('tv', 95396)).library).toMatchObject({ availability_stage: 'ready', missing: null });
    await titlesApi.unassignEpisode(302009); // S02E09; E10 hasn't aired
    expect((await titlesApi.get('tv', 95396)).library).toMatchObject({
      availability_stage: 'needs_torrent',
      missing: { season_number: 2, from: 9, to: 9, count: 1 },
    });
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

  it('assigns a torrent from the legacy search: one plausible file applies on its own and starts downloading', async () => {
    const search = await (await fetch('/api/jackett/search?q=One%20Battle%20After%20Another%202025')).json();
    expect(search.results).toHaveLength(3);
    const { job_id: jobId } = await assignmentApi.assignMovie(3, { magnet_uri: search.results[0].magnetUri, indexer: search.results[0].indexer });
    // In Node a job moves one step per read: fetching metadata, classifying, then applied
    for (let read = 0; read < 2; read++) await assignmentApi.job(jobId);
    expect((await assignmentApi.job(jobId)).job).toMatchObject({ status: 'applied', ai_available: false, torrent: { indexer: 'TorrentLeech' } });
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
      stream: { provider: 'hdrezka', title: 'Дюна: Часть вторая', translator: 'Оригинал (+субтитры)', quality: '1080p', size_bytes: 4_453_333_333 },
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
    await expect(hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: 10, quality: '1080p', items: [] })).rejects.toMatchObject({ status: 422 });

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
