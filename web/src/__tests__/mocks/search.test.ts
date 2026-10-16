/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { searchApi } from '@/lib/api/endpoints';
import type { Release, ReleaseAttributes, SearchTorrentsParams } from '@/lib/api/contract';
import { setMockScenario } from '@/mocks/scenario';
import {
  bestReleaseId,
  inScope,
  matchesFilters,
  matchesQuery,
  missingFor,
  type MockRelease,
  parseSearchParams,
  planKey,
  profileHides,
  searchFacets,
  sortReleases,
} from '@/mocks/fixtures/search-rules';
import { findMockRelease } from '@/mocks/fixtures/search';

/**
 * The context-aware torrent search mock (#92): the generic rules
 * (fixtures/search-rules.ts) on hand-made releases, then the handler on the
 * fixtures (fixtures/search.ts) behind MSW.
 */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(() => {
  resetMockState();
  jest.restoreAllMocks();
});
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const ATTRS: ReleaseAttributes = {
  resolution: '1080p',
  source: 'WEB-DL',
  codec: 'x264',
  bit_depth: null,
  hdr: [],
  audio: [],
  channels: null,
  languages: [],
  subs: null,
  group: null,
  edition: null,
  repack: false,
  pack: null,
};

function entry(id: string, patch: Partial<Release> & { attrs?: Partial<ReleaseAttributes> } = {}, covers: MockRelease['covers'] = []): MockRelease {
  const { attrs, ...rest } = patch;
  return {
    release: {
      id,
      raw_title: id,
      display_title: id,
      info_hash: id,
      magnet_uri: `magnet:?xt=urn:btih:${id}`,
      size_bytes: 1e9,
      seeders: 100,
      leechers: 1,
      published_at: null,
      indexer: 'Nyaa',
      attrs: { ...ATTRS, ...attrs },
      score: 70,
      score_reasons: [],
      notes: [],
      warnings: [],
      flags: [],
      ...rest,
    },
    covers,
  };
}

const parse = (query: string) => {
  const parsed = parseSearchParams(new URLSearchParams(query));
  if ('error' in parsed) throw new Error(parsed.error);
  return parsed;
};

describe('the rules', () => {
  it('validates like the backend', () => {
    const error = (query: string) => ('error' in parseSearchParams(new URLSearchParams(query)) ? 'error' : 'ok');
    expect(error('tmdb_id=1')).toBe('error');
    expect(error('media_type=tv&tmdb_id=x')).toBe('error');
    expect(error('media_type=tv&tmdb_id=1&scope=episode&season=1')).toBe('error');
    expect(error('media_type=tv&tmdb_id=1&scope=season')).toBe('error');
    expect(error('media_type=tv&tmdb_id=1&scope=everything')).toBe('error');
    expect(error('media_type=tv&tmdb_id=1&sort=random')).toBe('error');
    expect(error('media_type=tv&tmdb_id=1&min_seeders=many')).toBe('error');
    expect(error('media_type=movie&tmdb_id=1&scope=episode')).toBe('ok');
  });

  it('defaults the scope from the context, caps the page size, and repeats filters', () => {
    expect(parse('media_type=tv&tmdb_id=1&season=1&episode=29')).toMatchObject({ scope: 'episode', params: { page: 1, page_size: 25, sort: 'best' } });
    expect(parse('media_type=tv&tmdb_id=1&season=1').scope).toBe('season');
    expect(parse('media_type=tv&tmdb_id=1').scope).toBe('any');
    expect(parse('media_type=movie&tmdb_id=1&scope=season').scope).toBeNull();
    const parsed = parse('media_type=tv&tmdb_id=1&page_size=500&resolution=2160p&resolution=1080p&hdr=DV&min_size_gb=0.5&refresh=1');
    expect(parsed.params.page_size).toBe(100);
    expect(parsed.filters).toEqual({ resolution: ['2160p', '1080p'], hdr: ['DV'], min_size_gb: 0.5 });
    expect(parsed.refresh).toBe(true);
    expect(planKey(parsed.params)).toBe('tv:1:any:::');
  });

  it('scopes: the episode with packs; season packs; everything', () => {
    const single = entry('single', { attrs: { pack: { type: 'episode', label: '29', seasons: [], episodes: { from: 29, to: 29 } } } }, [{ season: 1, from: 29, to: 29 }]);
    const batch = entry('batch', { attrs: { pack: { type: 'multi_episode', label: '29–38', seasons: [], episodes: { from: 29, to: 38 } } } }, [{ season: 1, from: 29, to: 38 }]);
    const early = entry('early', { attrs: { pack: { type: 'multi_episode', label: '1–28', seasons: [], episodes: { from: 1, to: 28 } } } }, [{ season: 1, from: 1, to: 28 }]);
    const other = entry('other', { attrs: { pack: { type: 'episode', label: '30', seasons: [], episodes: { from: 30, to: 30 } } } }, [{ season: 1, from: 30, to: 30 }]);
    const keep = (scope: 'episode' | 'season' | 'any') => [single, batch, early, other].filter((e) => inScope(e, scope, 1, 29)).map((e) => e.release.id);
    expect(keep('episode')).toEqual(['single', 'batch']);
    expect(keep('season')).toEqual(['batch', 'early']);
    expect(keep('any')).toEqual(['single', 'batch', 'early', 'other']);
  });

  it('a typed q keeps names holding every word', () => {
    expect(matchesQuery({ raw_title: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]' }, 'frieren 29')).toBe(true);
    expect(matchesQuery({ raw_title: 'Frieren.S02E01.1080p' }, 'Sousou no Frieren')).toBe(false);
  });

  it('the profile hides avoid terms as whole words and resolutions above the maximum', () => {
    const profile = { avoid_terms: ['CAM', 'TS'], max_resolution: '1080p' as const };
    expect(profileHides({ raw_title: 'Movie.2025.HDCAM.x264', attrs: ATTRS }, { ...profile, avoid_terms: ['HDCAM'] })).toBe(true);
    expect(profileHides({ raw_title: 'Movie 2025 CAMRip', attrs: ATTRS }, profile)).toBe(false);
    expect(profileHides({ raw_title: 'Movie.2025.DTS-HD.MA', attrs: ATTRS }, profile)).toBe(false);
    expect(profileHides({ raw_title: 'Movie.2025.cam.x264', attrs: ATTRS }, profile)).toBe(true);
    expect(profileHides({ raw_title: 'Movie.2025.2160p', attrs: { ...ATTRS, resolution: '2160p' } }, profile)).toBe(true);
    expect(profileHides({ raw_title: 'Movie.2025.2160p', attrs: { ...ATTRS, resolution: '2160p' } }, { ...profile, max_resolution: null })).toBe(false);
  });

  const dv = entry('dv', { indexer: 'TorrentLeech', seeders: 300, score: 96, attrs: { resolution: '2160p', hdr: ['DV', 'HDR10'], audio: ['DD+', 'Atmos'], group: 'FLUX' } });
  const hdr = entry('hdr', { seeders: 4, score: 55, size_bytes: 31e9, attrs: { resolution: '2160p', hdr: ['HDR10'], group: 'CAKES' } }, [{ season: 1, from: 1, to: 13 }]);
  const sdr = entry('sdr', { indexer: 'EZTV', seeders: 540, score: 88, size_bytes: 2.6e9, attrs: { audio: ['DD+'], group: 'NTb' } });
  const cam = entry('cam', { seeders: 900, score: 4, flags: ['cam'], attrs: { resolution: 'unknown', source: 'CAM' } });
  const all = [dv, hdr, sdr, cam];

  it('filters: OR within one, AND across them; unknown values match nothing; sizes per episode for TV', () => {
    const ids = (filters: Parameters<typeof matchesFilters>[2], mediaType: 'tv' | 'movie' = 'tv') =>
      all.filter((e) => matchesFilters(e, mediaType, filters)).map((e) => e.release.id);
    expect(ids({ resolution: ['2160p', 'unknown'] })).toEqual(['dv', 'hdr', 'cam']);
    expect(ids({ resolution: ['2160p'], hdr: ['DV'] })).toEqual(['dv']);
    expect(ids({ hdr: ['SDR'] })).toEqual(['sdr', 'cam']);
    expect(ids({ audio: ['Atmos'] })).toEqual(['dv']);
    expect(ids({ group: ['nobody'] })).toEqual([]);
    expect(ids({ min_seeders: 100 })).toEqual(['dv', 'sdr', 'cam']);
    // 31 GB over 13 episodes is 2.4 GB an episode; a movie compares the whole release
    expect(ids({ min_size_gb: 2, max_size_gb: 3 })).toEqual(['hdr', 'sdr']);
    expect(ids({ min_size_gb: 2, max_size_gb: 3 }, 'movie')).toEqual(['sdr']);
  });

  it('facets are disjunctive, list every value in scope plus selected ones, in the allowed order', () => {
    const facets = searchFacets(all, 'tv', { resolution: ['2160p'], hdr: ['DV'] });
    // Each facet leaves its own filter out: resolution counts under hdr=DV, hdr under resolution=2160p (once per value)
    expect(facets.resolution).toEqual([
      { value: '2160p', count: 1 },
      { value: '1080p', count: 0 },
      { value: 'unknown', count: 0 },
    ]);
    expect(facets.hdr).toEqual([
      { value: 'DV', count: 1 },
      { value: 'HDR10', count: 2 },
      { value: 'SDR', count: 0 },
    ]);
    // Indexers and groups by count
    expect(facets.indexer).toEqual([
      { value: 'TorrentLeech', count: 1 },
      { value: 'EZTV', count: 0 },
      { value: 'Nyaa', count: 0 },
    ]);
    expect(facets.group.map((b) => b.value)).toEqual(['FLUX', 'CAKES', 'NTb']);
    // A selected value nothing has still shows
    expect(searchFacets(all, 'tv', { group: ['ghost'] }).group).toEqual([
      { value: 'CAKES', count: 1 },
      { value: 'FLUX', count: 1 },
      { value: 'NTb', count: 1 },
      { value: 'ghost', count: 0 },
    ]);
    expect(searchFacets(all, 'movie', {}).pack).toEqual([]);
  });

  it('best match: top score, never CAM or seedless, at least 50; sorts', () => {
    const releases = all.map((e) => e.release);
    expect(bestReleaseId(releases)).toBe('dv');
    expect(bestReleaseId([cam.release, { ...hdr.release, score: 49 }])).toBeNull();
    expect(sortReleases(releases, 'best', 'sdr').map((r) => r.id)).toEqual(['sdr', 'dv', 'hdr', 'cam']);
    expect(sortReleases(releases, 'seeders', null).map((r) => r.id)).toEqual(['cam', 'sdr', 'dv', 'hdr']);
    expect(sortReleases(releases, 'size', null).map((r) => r.id)).toEqual(['dv', 'cam', 'sdr', 'hdr']);
    const dated = [
      { ...dv.release, published_at: '2026-01-01T00:00:00Z' },
      { ...sdr.release, published_at: null },
      { ...hdr.release, published_at: '2026-03-01T00:00:00Z' },
    ];
    expect(sortReleases(dated, 'newest', null).map((r) => r.id)).toEqual(['hdr', 'dv', 'sdr']);
  });

  it('missing: only inside the run, for runs of more than one; best_covers when the best holds it all', () => {
    const run = { season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10 };
    const batch = entry('batch', {}, [{ season: 1, from: 29, to: 38 }]);
    expect(missingFor(run, { season: 1, episode: 29 }, batch)).toEqual({ ...run, best_covers: true });
    expect(missingFor(run, { season: 1 }, entry('one', {}, [{ season: 1, from: 29, to: 29 }]))).toEqual({ ...run, best_covers: false });
    expect(missingFor(run, { season: 1, episode: 12 }, batch)).toBeNull();
    expect(missingFor(run, { season: 2, episode: 29 }, batch)).toBeNull();
    expect(missingFor({ ...run, to: 29, count: 1 }, { season: 1, episode: 29 }, batch)).toBeNull();
    expect(missingFor(null, { season: 1, episode: 29 }, batch)).toBeNull();
  });
});

describe('the handler', () => {
  const FRIEREN_E29: SearchTorrentsParams = { media_type: 'tv', tmdb_id: 209867, season: 1, episode: 29 };

  it('Frieren S01E29: 27 in episode scope, 13 season packs, 45 in all; the 29–38 batch leads and covers the missing run', async () => {
    const episode = await searchApi.torrents(FRIEREN_E29);
    expect(episode).toMatchObject({ total: 27, total_unfiltered: 27, hidden: 0, best_id: 'rel-0', complete: true, page_size: 25 });
    expect(episode.items).toHaveLength(25);
    expect(episode.items[0].raw_title).toBe('[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]');
    expect(episode.queries.map((q) => q.q)).toEqual(['Frieren 29', 'Sousou no Frieren 29', 'Frieren S02E01']);
    expect(episode.missing).toEqual({ season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10, best_covers: true });
    expect(episode.indexers.map((i) => `${i.name}:${i.status}:${i.count}`)).toEqual(['Nyaa:ok:16', 'TorrentLeech:ok:7', 'The Pirate Bay:ok:4']);
    expect((await searchApi.torrents({ ...FRIEREN_E29, page: 2 })).items).toHaveLength(2);
    expect((await searchApi.torrents({ ...FRIEREN_E29, scope: 'season' })).total).toBe(13);
    expect((await searchApi.torrents({ ...FRIEREN_E29, scope: 'any' })).total).toBe(45);
    expect((await searchApi.torrents({ media_type: 'tv', tmdb_id: 209867, season: 1 })).total).toBe(13);
  });

  it('filters, facets and sort apply to the cached results', async () => {
    const filtered = await searchApi.torrents({ ...FRIEREN_E29, resolution: ['2160p'] });
    expect(filtered.total).toBe(3);
    expect(filtered.total_unfiltered).toBe(27);
    expect(filtered.facets.resolution.find((b) => b.value === '1080p')?.count).toBeGreaterThan(0);
    const none = await searchApi.torrents({ ...FRIEREN_E29, resolution: ['2160p'], hdr: ['DV'] });
    expect(none).toMatchObject({ total: 0, total_unfiltered: 27, best_id: null, items: [] });
    const bySeeders = await searchApi.torrents({ ...FRIEREN_E29, sort: 'seeders' });
    expect(bySeeders.items[0].seeders).toBe(Math.max(...bySeeders.items.map((r) => r.seeders)));
  });

  it('a typed q replaces the queries and still filters by scope', async () => {
    const answer = await searchApi.torrents({ ...FRIEREN_E29, q: 'Sousou no Frieren 29' });
    expect(answer.queries).toEqual([{ q: 'Sousou no Frieren 29', label: null }]);
    expect(answer.items.every((r) => /sousou no frieren/i.test(r.raw_title))).toBe(true);
    expect((await searchApi.torrents({ ...FRIEREN_E29, q: 'zzz' })).total).toBe(0);
  });

  it('a movie hides what the profile avoids, warns about the CAM it keeps, and has no scope or missing run', async () => {
    const movie = await searchApi.torrents({ media_type: 'movie', tmdb_id: 1054867 });
    expect(movie).toMatchObject({ total: 11, hidden: 1, missing: null });
    expect(movie.items[0].raw_title).toBe('One.Battle.After.Another.2025.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX');
    expect(movie.items.find((r) => r.attrs.source === 'CAM')).toMatchObject({ flags: ['cam'], warnings: ['CAM recording · filmed in a cinema'] });
    expect(movie.facets.pack).toEqual([]);
  });

  it('Severance: The Pirate Bay times out until a Retry asks again', async () => {
    const params: SearchTorrentsParams = { media_type: 'tv', tmdb_id: 95396, season: 2, episode: 8 };
    const first = await searchApi.torrents(params);
    expect(first.indexers.find((i) => i.name === 'The Pirate Bay')).toMatchObject({ status: 'timeout', count: 0, ms: 8000 });
    expect(first.items.some((r) => r.indexer === 'The Pirate Bay')).toBe(false);
    expect((await searchApi.torrents(params)).indexers.at(-1)?.status).toBe('timeout');
    const retried = await searchApi.torrents({ ...params, refresh: 1 });
    expect(retried.indexers.every((i) => i.status === 'ok')).toBe(true);
    expect(retried.total).toBeGreaterThan(first.total);
    expect((await searchApi.torrents(params)).indexers.at(-1)?.status).toBe('ok');
  });

  it('search-partial: the last indexer times out until a Retry', async () => {
    setMockScenario('search-partial');
    expect((await searchApi.torrents(FRIEREN_E29)).indexers.at(-1)).toMatchObject({ name: 'The Pirate Bay', status: 'timeout' });
    expect((await searchApi.torrents({ ...FRIEREN_E29, refresh: 1 })).indexers.at(-1)?.status).toBe('ok');
  });

  it('search-slow: pending indexers, then one more each second until complete', async () => {
    setMockScenario('search-slow');
    const now = jest.spyOn(Date, 'now').mockReturnValue(1_000_000);
    const start = await searchApi.torrents(FRIEREN_E29);
    expect(start).toMatchObject({ complete: false, total: 0 });
    expect(start.indexers.map((i) => i.status)).toEqual(['pending', 'pending', 'pending']);
    now.mockReturnValue(1_001_200);
    const one = await searchApi.torrents(FRIEREN_E29);
    expect(one.indexers.map((i) => i.status)).toEqual(['ok', 'pending', 'pending']);
    expect(one.indexers[1].ms).toBe(1200);
    expect(one.total).toBe(16);
    now.mockReturnValue(1_003_100);
    expect(await searchApi.torrents(FRIEREN_E29)).toMatchObject({ complete: true, total: 27 });
  });

  it('search-none finds nothing; search-down is 503; unknown titles 404; bad params 400', async () => {
    setMockScenario('search-none');
    expect(await searchApi.torrents(FRIEREN_E29)).toMatchObject({ total: 0, total_unfiltered: 0, complete: true });
    setMockScenario('search-down');
    await expect(searchApi.torrents(FRIEREN_E29)).rejects.toMatchObject({ status: 503, code: 'unavailable' });
    setMockScenario(null);
    await expect(searchApi.torrents({ media_type: 'tv', tmdb_id: 1 })).rejects.toMatchObject({ status: 404, code: 'not_found' });
    await expect(searchApi.torrents({ media_type: 'tv', tmdb_id: 209867, scope: 'episode' })).rejects.toMatchObject({ status: 400, code: 'bad_request' });
  });

  it('resolves a release without a magnet, and magnets carry a name for the mapping job', async () => {
    const { items } = await searchApi.torrents({ ...FRIEREN_E29, page_size: 100 });
    const fetchNeeded = items.find((r) => r.flags.includes('needs_torrent_fetch'));
    expect(fetchNeeded).toMatchObject({ magnet_uri: null, info_hash: null });
    const resolved = await searchApi.resolve({ release_id: fetchNeeded!.id });
    expect(resolved.magnet_uri).toContain(`dn=${encodeURIComponent(fetchNeeded!.raw_title)}`);
    expect(resolved.info_hash).toHaveLength(40);
    expect(findMockRelease('rel-0')?.release.magnet_uri).toContain('dn=');
  });
});
