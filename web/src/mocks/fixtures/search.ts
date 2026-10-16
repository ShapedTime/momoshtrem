import type { MediaType, Release, ReleaseAttributes, ReleaseFlag, SearchQuery } from '@/lib/api/contract';
import { normaliseTitle } from '@/lib/search/query';
import { type CatalogEntry, findTitle, iso } from './catalog';
import type { Coverage, MockRelease } from './search-rules';
import { SHOWS, type ShowSeed } from './title-seeds';

/**
 * What the indexers find per context (#92), from the prototype's data
 * (design-cinematic/js/screens/torrent.js and js/data.js):
 * - Frieren (209867): anime with absolute and scene numbering. S01E29 finds
 *   27 releases in episode scope, 13 season packs and 45 in all; the best
 *   match is the 29–38 batch, which covers the missing run.
 * - Severance (95396) and every other show: a season pack set plus, for an
 *   episode, singles named after it. The Pirate Bay times out on Severance
 *   until a Retry.
 * - Every movie: the prototype's set (best 2160p DV WEB-DL, an oversized
 *   REMUX, a CAM, a CAM the profile hides and one without a magnet).
 * search-rules.ts turns them into answers.
 */

export interface SearchFixture {
  /** The text queries sent, primary first. */
  queries: SearchQuery[];
  /** Indexers searched, in order. */
  indexers: string[];
  /** Times out until a Retry (`refresh=1`) asks again. */
  flaky?: string;
  releases: MockRelease[];
}

const GB = 1e9;
const pad = (n: number) => String(n).padStart(2, '0');

/** 40 stable hex characters per name. */
function hashOf(text: string): string {
  let out = '';
  for (let round = 0; out.length < 40; round++) {
    let h = 0x811c9dc5 ^ round;
    for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), 0x01000193);
    out += (h >>> 0).toString(16).padStart(8, '0');
  }
  return out.slice(0, 40);
}

/** The magnet a release carries, named so the mapping mock builds a matching job (fixtures/assignment.ts reads `dn`). */
const magnetFor = (hash: string, name: string) => `magnet:?xt=urn:btih:${hash}&dn=${encodeURIComponent(name)}&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337`;

interface Spec {
  raw: string;
  gb: number;
  seeders: number;
  leechers: number;
  /** Published this many days before the fixtures' "today". */
  days: number;
  indexer: string;
  attrs?: Partial<ReleaseAttributes>;
  score: number;
  reasons?: string[];
  notes?: string[];
  warnings?: string[];
  flags?: ReleaseFlag[];
  covers?: Coverage[];
  /** No magnet in the indexer's answer: resolve fetches the .torrent (`needs_torrent_fetch`). */
  fetch?: boolean;
}

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

/** Every release built so far, by id: resolve and inspect find them here. */
const known = new Map<string, MockRelease>();

function build(id: string, spec: Spec): MockRelease {
  const hash = hashOf(spec.raw);
  const magnet = magnetFor(hash, spec.raw);
  const attrs = { ...ATTRS, ...spec.attrs };
  const flags = new Set(spec.flags);
  if (spec.seeders === 0) flags.add('no_seeders');
  else if (spec.seeders < 5) flags.add('low_seeders');
  if (attrs.source === 'CAM') flags.add('cam');
  if (spec.fetch) flags.add('needs_torrent_fetch');
  const release: Release = {
    id,
    raw_title: spec.raw,
    display_title: spec.raw.replace(/[._]/g, ' '),
    info_hash: spec.fetch ? null : hash,
    magnet_uri: spec.fetch ? null : magnet,
    size_bytes: Math.round(spec.gb * GB),
    seeders: spec.seeders,
    leechers: spec.leechers,
    published_at: iso(spec.days),
    indexer: spec.indexer,
    attrs,
    score: spec.score,
    score_reasons: spec.reasons ?? [],
    notes: spec.notes ?? [],
    warnings: spec.warnings ?? [],
    flags: [...flags],
  };
  const entry: MockRelease = { release, covers: spec.covers ?? [], ...(spec.fetch ? { resolves: { magnet_uri: magnet, info_hash: hash } } : {}) };
  known.set(id, entry);
  return entry;
}

/** A release an earlier search found (resolve and inspect take its id for an hour). */
export const findMockRelease = (id: unknown): MockRelease | undefined => (typeof id === 'string' ? known.get(id) : undefined);

const S1 = (from: number, to: number): Coverage[] => [{ season: 1, from, to }];
const episodePack = (label: string, seasons: number[], from: number, to: number) => ({ type: 'episode' as const, label, seasons, episodes: { from, to } });
const single = (n: number, label = String(n)) => episodePack(label, [], n, n);
const run = (from: number, to: number) => ({ type: 'multi_episode' as const, label: `Episodes ${from}–${to}`, seasons: [], episodes: { from, to } });
const season = (n: number, label = `Season ${n}`) => ({ type: 'season' as const, label, seasons: [n], episodes: null });

// ---- Frieren ------------------------------------------------------------------------------

const F = 'Frieren.Beyond.Journeys.End';
const HAS_FILES = 'Includes 28 episodes that already have a file: adding replaces them, and the mapping review asks first.';

/** Ids rel-0…: rel-0 is the best match for S01E29. */
const FRIEREN_SPECS: Spec[] = [
  {
    raw: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]',
    gb: 13.2, seeders: 412, leechers: 18, days: 205, indexer: 'Nyaa',
    attrs: { source: 'WEB', group: 'SubsPlease', pack: run(29, 38) },
    score: 96,
    reasons: ['Covers all 10 missing episodes', '1080p WEB from SubsPlease', '412 seeders', '1.3 GB per episode fits your profile'],
    covers: S1(29, 38),
  },
  {
    raw: `${F}.S02.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG`,
    gb: 14.1, seeders: 188, leechers: 9, days: 203, indexer: 'TorrentLeech',
    attrs: { audio: ['AAC'], channels: '2.0', group: 'VARYG', pack: season(2) },
    score: 91,
    reasons: ['All 10 missing episodes', '1080p WEB-DL from VARYG, a group you prefer', '188 seeders'],
    notes: ['Scene "Season 2" = TMDB S01E29–E38; mapped automatically'],
    covers: S1(29, 38),
  },
  {
    raw: '[Erai-raws] Sousou no Frieren - 29 [1080p][Multiple Subtitle][7B1C0F1E].mkv',
    gb: 1.4, seeders: 96, leechers: 2, days: 271, indexer: 'Nyaa',
    attrs: { source: 'WEB', codec: 'unknown', group: 'Erai-raws', subs: 'multi', pack: single(29) },
    score: 84, reasons: ['Exact episode', '96 seeders'], covers: S1(29, 29),
  },
  {
    raw: `${F}.S01.COMPLETE.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.5.1-ANiHLS`,
    gb: 186, seeders: 31, leechers: 44, days: 60, indexer: 'TorrentLeech',
    attrs: { resolution: '2160p', source: 'REMUX', codec: 'x265', audio: ['DTS-HD'], channels: '5.1', group: 'ANiHLS', pack: season(1) },
    score: 62, flags: ['size_outlier', 'over_size_limit'],
    notes: [HAS_FILES], warnings: ['Over your 8 GB/hour limit'], covers: S1(1, 38),
  },
  {
    raw: '[ASW] Sousou no Frieren - 29 [1080p HEVC x265 10Bit][AAC]',
    gb: 0.312, seeders: 64, leechers: 1, days: 270, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, audio: ['AAC'], group: 'ASW', pack: single(29) },
    score: 80, reasons: ['Exact episode', 'x265, small'], covers: S1(29, 29),
  },
  {
    raw: 'Frieren - 29 [720p] [Judas]',
    gb: 0.22, seeders: 0, leechers: 0, days: 245, indexer: 'The Pirate Bay',
    attrs: { resolution: '720p', source: 'unknown', codec: 'x265', group: 'Judas', pack: single(29) },
    score: 21, warnings: ['No seeders · it may never start'], covers: S1(29, 29),
  },
  {
    raw: '[Judas] Sousou no Frieren - S01E29 [1080p][HEVC x265 10bit][Multi-Subs]',
    gb: 0.402, seeders: 41, leechers: 2, days: 268, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, group: 'Judas', subs: 'multi', pack: single(29, 'S01E29') },
    score: 74, covers: S1(29, 29),
  },
  {
    raw: '[EMBER] Frieren (2023) (Season 1) [1080p] [HEVC WEBRip DDP] (Sousou no Frieren)',
    gb: 19.4, seeders: 66, leechers: 7, days: 150, indexer: 'Nyaa',
    attrs: { source: 'WEBRip', codec: 'x265', audio: ['DD+'], group: 'EMBER', pack: season(1) },
    score: 77, notes: [HAS_FILES], covers: S1(1, 38),
  },
  {
    raw: '[SubsPlease] Sousou no Frieren - 29 (1080p) [9E3C5D21].mkv',
    gb: 1.4, seeders: 302, leechers: 11, days: 271, indexer: 'Nyaa',
    attrs: { source: 'WEB', codec: 'unknown', group: 'SubsPlease', pack: single(29) },
    score: 88, reasons: ['Exact episode', '1080p WEB from SubsPlease', '302 seeders'], covers: S1(29, 29),
  },
  {
    raw: '[SubsPlease] Sousou no Frieren - 29 (720p) [4F2A7B10].mkv',
    gb: 0.73, seeders: 120, leechers: 4, days: 271, indexer: 'Nyaa',
    attrs: { resolution: '720p', source: 'WEB', codec: 'unknown', group: 'SubsPlease', pack: single(29) },
    score: 72, covers: S1(29, 29),
  },
  {
    raw: '[SubsPlease] Sousou no Frieren - 29 (480p) [C81D2E07].mkv',
    gb: 0.36, seeders: 33, leechers: 1, days: 271, indexer: 'Nyaa',
    attrs: { resolution: '480p', source: 'WEB', codec: 'unknown', group: 'SubsPlease', pack: single(29) },
    score: 51, covers: S1(29, 29),
  },
  {
    raw: `${F}.S02E01.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG`,
    gb: 1.4, seeders: 140, leechers: 6, days: 270, indexer: 'TorrentLeech',
    attrs: { audio: ['AAC'], channels: '2.0', group: 'VARYG', pack: episodePack('S02E01', [2], 1, 1) },
    score: 86, notes: ['Scene "S02E01" = TMDB S01E29'], covers: S1(29, 29),
  },
  {
    raw: `${F}.S01E29.Lets.Go.Somewhere.We.Cant.Reach.1080p.CR.WEB-DL.DUAL.AAC2.0.H.264-VARYG`,
    gb: 1.5, seeders: 57, leechers: 3, days: 269, indexer: 'TorrentLeech',
    attrs: { audio: ['AAC'], channels: '2.0', languages: ['dual'], group: 'VARYG', pack: single(29, 'S01E29') },
    score: 82, covers: S1(29, 29), fetch: true,
  },
  {
    raw: '[Tsundere-Raws] Sousou no Frieren - 29 VOSTFR (CR) [WEB 1080p x264 AAC]',
    gb: 1.3, seeders: 25, leechers: 2, days: 270, indexer: 'Nyaa',
    attrs: { source: 'WEB', audio: ['AAC'], group: 'Tsundere-Raws', pack: single(29) },
    score: 58, covers: S1(29, 29),
  },
  {
    raw: '[Yameii] Frieren - Beyond Journey’s End - S01E29 [English Dub] [CR WEB-DL 1080p] [F1A2B3C4]',
    gb: 1.4, seeders: 48, leechers: 5, days: 240, indexer: 'Nyaa',
    attrs: { codec: 'unknown', languages: ['en'], group: 'Yameii', pack: single(29, 'S01E29') },
    score: 70, covers: S1(29, 29),
  },
  {
    raw: '[DKB] Sousou no Frieren - S01E29 [1080p][HEVC x265 10bit][Multi-Subs]',
    gb: 0.45, seeders: 38, leechers: 1, days: 266, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, subs: 'multi', group: 'DKB', pack: single(29, 'S01E29') },
    score: 69, covers: S1(29, 29),
  },
  {
    raw: '[NanakoRaws] Sousou no Frieren - 29 (NTV 1920x1080 x264 AAC).mkv',
    gb: 2.1, seeders: 12, leechers: 0, days: 271, indexer: 'Nyaa',
    attrs: { source: 'HDTV', audio: ['AAC'], group: 'NanakoRaws', pack: single(29) },
    score: 45, covers: S1(29, 29),
  },
  {
    raw: '[LostYears] Frieren - Beyond Journey’s End - S01E29 (WEB 1080p HEVC AAC) [Dual-Audio]',
    gb: 0.6, seeders: 29, leechers: 2, days: 230, indexer: 'Nyaa',
    attrs: { source: 'WEB', codec: 'x265', audio: ['AAC'], languages: ['dual'], group: 'LostYears', pack: single(29, 'S01E29') },
    score: 73, covers: S1(29, 29),
  },
  {
    raw: '[MiniFreeza] Frieren - 29 [480p HEVC]',
    gb: 0.12, seeders: 9, leechers: 0, days: 265, indexer: 'The Pirate Bay',
    attrs: { resolution: '480p', source: 'unknown', codec: 'x265', group: 'MiniFreeza', pack: single(29) },
    score: 34, covers: S1(29, 29),
  },
  {
    raw: `${F}.S01E29.2160p.CR.WEB-DL.AAC2.0.H.265-NanDesuKa`,
    gb: 3.2, seeders: 22, leechers: 3, days: 268, indexer: 'TorrentLeech',
    attrs: { resolution: '2160p', codec: 'x265', audio: ['AAC'], channels: '2.0', group: 'NanDesuKa', pack: single(29, 'S01E29') },
    score: 79, covers: S1(29, 29),
  },
  {
    raw: '[Breeze] Sousou no Frieren - S01E29 [1080p AV1][Dual Audio]',
    gb: 0.5, seeders: 15, leechers: 1, days: 220, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'AV1', languages: ['dual'], group: 'Breeze', pack: single(29, 'S01E29') },
    score: 66, covers: S1(29, 29),
  },
  {
    raw: '[Judas] Sousou no Frieren (Season 1) [1080p][HEVC x265 10bit][Multi-Subs] (Batch)',
    gb: 16.8, seeders: 54, leechers: 6, days: 180, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, subs: 'multi', group: 'Judas', pack: season(1) },
    score: 75, notes: [HAS_FILES], covers: S1(1, 38),
  },
  {
    raw: '[ASW] Sousou no Frieren - 29-38 [1080p HEVC][Batch]',
    gb: 3.1, seeders: 47, leechers: 2, days: 205, indexer: 'Nyaa',
    attrs: { source: 'unknown', codec: 'x265', group: 'ASW', pack: run(29, 38) },
    score: 81, covers: S1(29, 38),
  },
  {
    raw: `${F}.S01.2160p.CR.WEB-DL.DUAL.AAC2.0.H.265-NanDesuKa`,
    gb: 92.4, seeders: 19, leechers: 8, days: 190, indexer: 'TorrentLeech',
    attrs: { resolution: '2160p', codec: 'x265', audio: ['AAC'], channels: '2.0', languages: ['dual'], group: 'NanDesuKa', pack: season(1) },
    score: 71, notes: [HAS_FILES], covers: S1(1, 38),
  },
  {
    raw: '[Anime Time] Sousou no Frieren (Season 1) [1080p][HEVC 10bit x265][AAC][Multi Sub]',
    gb: 12.5, seeders: 36, leechers: 3, days: 170, indexer: 'The Pirate Bay',
    attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, audio: ['AAC'], subs: 'multi', group: 'Anime Time', pack: season(1) },
    score: 68, notes: [HAS_FILES], covers: S1(1, 38),
  },
  {
    raw: `${F}.S01.Part.3.1080p.NF.WEB-DL.DDP5.1.H.264-VARYG`,
    gb: 15, seeders: 61, leechers: 4, days: 160, indexer: 'TorrentLeech',
    attrs: { audio: ['DD+'], channels: '5.1', group: 'VARYG', pack: { type: 'multi_episode', label: 'Season 1 part 3', seasons: [1], episodes: { from: 29, to: 38 } } },
    score: 78, covers: S1(29, 38),
  },
  {
    raw: 'Frieren.Complete.Series.1080p.WEB-DL.AAC2.0.H.264',
    gb: 52, seeders: 23, leechers: 9, days: 140, indexer: 'The Pirate Bay',
    attrs: { audio: ['AAC'], channels: '2.0', pack: { type: 'complete', label: 'Complete series', seasons: [1], episodes: null } },
    score: 60, notes: ['Covers the whole show, 38 episodes', HAS_FILES], covers: S1(1, 38),
  },
  {
    raw: '[SubsPlease] Sousou no Frieren (01-28) (1080p) [Batch]',
    gb: 36.8, seeders: 520, leechers: 22, days: 420, indexer: 'Nyaa',
    attrs: { source: 'WEB', group: 'SubsPlease', pack: run(1, 28) },
    score: 90, notes: [HAS_FILES], covers: S1(1, 28),
  },
  {
    raw: '[Judas] Sousou no Frieren - Part 1 (01-16) [BD 1080p][HEVC x265 10bit][Dual-Audio]',
    gb: 9.8, seeders: 44, leechers: 2, days: 300, indexer: 'Nyaa',
    attrs: { source: 'BluRay', codec: 'x265', bit_depth: 10, languages: ['dual'], group: 'Judas', pack: run(1, 16) },
    score: 70, covers: S1(1, 16),
  },
  {
    raw: '[Erai-raws] Sousou no Frieren - 01 ~ 28 [1080p][Multiple Subtitle]',
    gb: 38.2, seeders: 210, leechers: 15, days: 410, indexer: 'Nyaa',
    attrs: { source: 'WEB', codec: 'unknown', group: 'Erai-raws', subs: 'multi', pack: run(1, 28) },
    score: 85, notes: [HAS_FILES], covers: S1(1, 28),
  },
  // Other episodes: only "Any" keeps them
  ...[27, 28, 30, 31, 32, 33, 34].map(
    (n): Spec => ({
      raw: `[SubsPlease] Sousou no Frieren - ${n} (1080p) [${hashOf(`sp${n}`).slice(0, 8).toUpperCase()}].mkv`,
      gb: 1.4, seeders: 280 - n * 3, leechers: 8, days: 271 - (n - 29) * 7, indexer: 'Nyaa',
      attrs: { source: 'WEB', codec: 'unknown', group: 'SubsPlease', pack: single(n) },
      score: 60, covers: S1(n, n),
    })
  ),
  ...[28, 30, 31, 32].map(
    (n): Spec => ({
      raw: `[Erai-raws] Sousou no Frieren - ${n} [1080p][Multiple Subtitle].mkv`,
      gb: 1.4, seeders: 90 - n, leechers: 2, days: 271 - (n - 29) * 7, indexer: 'Nyaa',
      attrs: { source: 'WEB', codec: 'unknown', group: 'Erai-raws', subs: 'multi', pack: single(n) },
      score: 58, covers: S1(n, n),
    })
  ),
  ...[2, 3].map(
    (n): Spec => ({
      raw: `${F}.S02E${pad(n)}.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG`,
      gb: 1.4, seeders: 130 - n * 5, leechers: 5, days: 270 - n * 7, indexer: 'TorrentLeech',
      attrs: { audio: ['AAC'], channels: '2.0', group: 'VARYG', pack: episodePack(`S02E${pad(n)}`, [2], n, n) },
      score: 57, notes: [`Scene "S02E${pad(n)}" = TMDB S01E${28 + n}`], covers: S1(28 + n, 28 + n),
    })
  ),
  ...[30, 31].map(
    (n): Spec => ({
      raw: `[ASW] Sousou no Frieren - ${n} [1080p HEVC x265 10Bit][AAC]`,
      gb: 0.31, seeders: 60 - n, leechers: 1, days: 271 - (n - 29) * 7, indexer: 'Nyaa',
      attrs: { source: 'unknown', codec: 'x265', bit_depth: 10, audio: ['AAC'], group: 'ASW', pack: single(n) },
      score: 56, covers: S1(n, n),
    })
  ),
];

const FRIEREN_RELEASES = FRIEREN_SPECS.map((spec, i) => build(`rel-${i}`, spec));

function frierenQueries(seasonNumber: number | undefined, episode: number | undefined): SearchQuery[] {
  if (seasonNumber === 1 && episode !== undefined) {
    return [
      { q: `Frieren ${episode}`, label: null },
      { q: `Sousou no Frieren ${episode}`, label: 'original title' },
      ...(episode > 28 ? [{ q: `Frieren S02E${pad(episode - 28)}`, label: 'scene numbering' as const }] : []),
    ];
  }
  const primary = seasonNumber !== undefined ? `Frieren Beyond Journeys End S${pad(seasonNumber)}` : 'Frieren Beyond Journeys End';
  return [
    { q: primary, label: null },
    { q: 'Sousou no Frieren', label: 'original title' },
  ];
}

// ---- Other shows ----------------------------------------------------------------------------

/** "The Good Place" → "The.Good.Place", "Who Is Alive?" → "Who.Is.Alive". */
const dotted = (text: string) => normaliseTitle(text.replace(/&/g, 'and')).replace(/\s+/g, '.');

/** Season packs for the season (or every season for the whole show), plus, for an episode, singles named after it. */
function showSpecs(entry: CatalogEntry, show: ShowSeed, seasonNumber: number | undefined, episode: number | undefined): Spec[] {
  const T = dotted(entry.title);
  const service = show.networks.includes('Apple TV+') ? 'ATVP.' : '';
  const regular = show.seasons.filter((s) => s.n > 0);
  const last = regular.at(-1);
  const allEpisodes = regular.reduce((sum, s) => sum + s.episodes.length, 0);
  const specs: Spec[] = [];
  if (regular.length > 1 && last) {
    specs.push({
      raw: `${T}.S01-S${pad(last.n)}.COMPLETE.1080p.BluRay.x265.10bit.AAC5.1-YAWNiX`,
      gb: 0.45 * allEpisodes, seeders: 88, leechers: 14, days: 1100, indexer: 'The Pirate Bay',
      attrs: { source: 'BluRay', codec: 'x265', bit_depth: 10, audio: ['AAC'], channels: '5.1', group: 'YAWNiX', pack: { type: 'complete', label: `Seasons 1–${last.n}`, seasons: regular.map((s) => s.n), episodes: null } },
      score: 89, notes: [`Covers all ${regular.length} seasons, ${allEpisodes} episodes`],
      covers: regular.map((s) => ({ season: s.n, from: 1, to: s.episodes.length })),
    });
  }
  for (const s of seasonNumber !== undefined ? regular.filter((r) => r.n === seasonNumber) : regular) {
    const count = s.episodes.length;
    const covers = [{ season: s.n, from: 1, to: count }];
    specs.push(
      {
        raw: `${T}.S${pad(s.n)}.1080p.${service}WEB-DL.DDP5.1.H.264-NTb`,
        gb: 1.1 * count, seeders: 141 - s.n * 4, leechers: 6, days: 400 + 30 * (regular.length - s.n), indexer: 'TorrentLeech',
        attrs: { audio: ['DD+'], channels: '5.1', group: 'NTb', pack: season(s.n) },
        score: 94, reasons: [`Whole season ${s.n}`, '1080p WEB-DL from NTb, a group you prefer', `${141 - s.n * 4} seeders`, '1.1 GB per episode'],
        covers,
      },
      {
        raw: `${T} Season ${s.n} Complete 720p WEB x264 [i_c]`,
        gb: 0.38 * count, seeders: 37, leechers: 2, days: 700, indexer: 'The Pirate Bay',
        attrs: { resolution: '720p', source: 'WEB', group: 'i_c', pack: season(s.n) },
        score: 70, covers,
      },
      {
        raw: `${T}.S${pad(s.n)}.2160p.${service}WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX`,
        gb: 4.2 * count, seeders: 198, leechers: 30, days: 380, indexer: 'TorrentLeech',
        attrs: { resolution: '2160p', codec: 'x265', hdr: ['DV', 'HDR10'], audio: ['DD+', 'Atmos'], channels: '5.1', group: 'FLUX', pack: season(s.n) },
        score: 90, reasons: [`Whole season ${s.n}`, '2160p WEB-DL', 'Dolby Vision', '198 seeders'],
        notes: s.episodes.some((e) => e.state !== 'missing' && e.state !== 'upcoming')
          ? ['Includes episodes that already have a file: adding replaces them, and the mapping review asks first.']
          : [],
        covers,
      },
      {
        raw: `${T}.S${pad(s.n)}.2160p.HDR.WEB.H265-CAKES`,
        gb: 2.4 * count, seeders: 4, leechers: 1, days: 600, indexer: 'TorrentLeech',
        attrs: { resolution: '2160p', source: 'WEB', codec: 'x265', hdr: ['HDR10'], group: 'CAKES', pack: season(s.n) },
        score: 55, warnings: ['Only 4 seeders · may be slow'], covers,
      }
    );
  }
  const episodeSeed = seasonNumber !== undefined && episode !== undefined ? regular.find((s) => s.n === seasonNumber)?.episodes.find((e) => e.n === episode) : undefined;
  if (seasonNumber === undefined || episode === undefined || !episodeSeed) return specs;
  const code = `S${pad(seasonNumber)}E${pad(episode)}`;
  const named = `${code}.${dotted(episodeSeed.name)}`;
  const one = [{ season: seasonNumber, from: episode, to: episode }];
  const pack = episodePack(code, [seasonNumber], episode, episode);
  return [
    ...specs,
    {
      raw: `${T}.${named}.2160p.${service}WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX`,
      gb: 4.1, seeders: 312, leechers: 21, days: 240, indexer: 'TorrentLeech',
      attrs: { resolution: '2160p', codec: 'x265', hdr: ['DV', 'HDR10'], audio: ['DD+', 'Atmos'], channels: '5.1', group: 'FLUX', pack },
      score: 97, reasons: ['2160p WEB-DL', 'Dolby Vision', '4.1 GB', '312 seeders', 'matches your quality profile'], covers: one,
    },
    {
      raw: `${T}.${named}.1080p.${service}WEB-DL.DDP5.1.H.264-NTb`,
      gb: 2.6, seeders: 540, leechers: 12, days: 240, indexer: 'TorrentLeech',
      attrs: { audio: ['DD+'], channels: '5.1', group: 'NTb', pack },
      score: 88, reasons: ['1080p WEB-DL from NTb, a group you prefer', '540 seeders'], covers: one,
    },
    {
      raw: `${T} ${code} 720p HEVC x265-MeGusta`,
      gb: 0.41, seeders: 77, leechers: 3, days: 239, indexer: 'EZTV',
      attrs: { resolution: '720p', source: 'unknown', codec: 'x265', group: 'MeGusta', pack }, score: 61, covers: one,
    },
    {
      raw: `${T}.${code}.1080p.WEB.H264-SuccessfulCrab`,
      gb: 1.9, seeders: 210, leechers: 9, days: 240, indexer: 'EZTV',
      attrs: { source: 'WEB', group: 'SuccessfulCrab', pack }, score: 80, covers: one,
    },
    {
      raw: `${T}.${code}.720p.WEB.H264-SuccessfulCrab`,
      gb: 0.9, seeders: 95, leechers: 4, days: 240, indexer: 'EZTV',
      attrs: { resolution: '720p', source: 'WEB', group: 'SuccessfulCrab', pack }, score: 66, covers: one,
    },
    {
      raw: `${T}.${code}.REPACK.2160p.WEB.H265-SuccessfulCrab`,
      gb: 5.2, seeders: 45, leechers: 6, days: 238, indexer: 'TorrentLeech',
      attrs: { resolution: '2160p', source: 'WEB', codec: 'x265', group: 'SuccessfulCrab', repack: true, pack }, score: 83, covers: one, fetch: true,
    },
    {
      raw: `${T}.${code}.1080p.x265-ELiTE`,
      gb: 0.8, seeders: 133, leechers: 7, days: 239, indexer: 'The Pirate Bay',
      attrs: { source: 'unknown', codec: 'x265', group: 'ELiTE', pack }, score: 72, covers: one,
    },
    {
      raw: `${T}.${code}.1080p.HEVC.x265-MeGusta[EZTVx.to]`,
      gb: 0.6, seeders: 64, leechers: 2, days: 239, indexer: 'The Pirate Bay',
      attrs: { source: 'unknown', codec: 'x265', group: 'MeGusta', pack }, score: 63, covers: one,
    },
  ];
}

// ---- Movies -----------------------------------------------------------------------------------

/** The prototype's movie set, named after the movie. The HDCAM is hidden by the profile's avoid terms. */
function movieSpecs(entry: CatalogEntry): Spec[] {
  const n = dotted(entry.title);
  const y = entry.year;
  const hours = (entry.runtime ?? 120) / 60;
  return [
    {
      raw: `${n}.${y}.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX`,
      gb: 15.6, seeders: 286, leechers: 19, days: 300, indexer: 'TorrentLeech',
      attrs: { resolution: '2160p', codec: 'x265', hdr: ['DV', 'HDR10'], audio: ['DD+', 'Atmos'], channels: '5.1', group: 'FLUX' },
      score: 96, reasons: ['2160p WEB-DL', 'Dolby Vision', `15.6 GB (${(15.6 / hours).toFixed(1)} GB/h)`, '286 seeders', 'matches your quality profile'],
    },
    {
      raw: `${n}.${y}.1080p.BluRay.x264-SPARKS`,
      gb: 9.8, seeders: 410, leechers: 8, days: 290, indexer: 'The Pirate Bay',
      attrs: { source: 'BluRay', group: 'SPARKS' }, score: 85, reasons: ['1080p BluRay', '410 seeders'],
    },
    {
      raw: `${n}.${y}.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.Atmos.7.1-FGT`,
      gb: 61.2, seeders: 52, leechers: 30, days: 280, indexer: 'TorrentLeech',
      attrs: { resolution: '2160p', source: 'REMUX', codec: 'x265', hdr: ['DV', 'HDR10'], audio: ['TrueHD', 'Atmos'], channels: '7.1', group: 'FGT' },
      score: 64, flags: ['size_outlier', 'over_size_limit'], warnings: ['Over your 8 GB/hour limit'],
    },
    {
      raw: `${n} ${y} CAMRip x264 AAC`,
      gb: 1.2, seeders: 900, leechers: 45, days: 320, indexer: 'The Pirate Bay',
      attrs: { resolution: 'unknown', source: 'CAM', audio: ['AAC'] }, score: 4, warnings: ['CAM recording · filmed in a cinema'],
    },
    {
      raw: `${n}.${y}.HDCAM.x264-AOC`,
      gb: 1.4, seeders: 640, leechers: 30, days: 330, indexer: 'The Pirate Bay',
      attrs: { resolution: 'unknown', source: 'CAM', group: 'AOC' }, score: 3, warnings: ['CAM recording · filmed in a cinema'],
    },
    {
      raw: `${n}.${y}.1080p.WEB-DL.DDP5.1.H.264-FLUX`,
      gb: 7.1, seeders: 198, leechers: 11, days: 300, indexer: 'TorrentLeech',
      attrs: { audio: ['DD+'], channels: '5.1', group: 'FLUX' }, score: 88, reasons: ['1080p WEB-DL from FLUX, a group you prefer', '198 seeders'], fetch: true,
    },
    {
      raw: `${n}.${y}.1080p.WEBRip.x265.10bit.AAC5.1-YTS`,
      gb: 2.3, seeders: 1500, leechers: 80, days: 295, indexer: '1337x',
      attrs: { source: 'WEBRip', codec: 'x265', bit_depth: 10, audio: ['AAC'], channels: '5.1', group: 'YTS' }, score: 78,
    },
    {
      raw: `${n}.${y}.720p.WEBRip.x264.AAC-YTS`,
      gb: 1.1, seeders: 820, leechers: 40, days: 295, indexer: '1337x',
      attrs: { resolution: '720p', source: 'WEBRip', audio: ['AAC'], group: 'YTS' }, score: 62,
    },
    {
      raw: `${n}.${y}.2160p.WEB-DL.DDP5.1.H.265-NTb`,
      gb: 12.9, seeders: 74, leechers: 6, days: 298, indexer: 'TorrentLeech',
      attrs: { resolution: '2160p', codec: 'x265', audio: ['DD+'], channels: '5.1', group: 'NTb' }, score: 90,
    },
    {
      raw: `${n}.${y}.MULTi.1080p.WEB.H264-FW`,
      gb: 6.5, seeders: 31, leechers: 3, days: 297, indexer: '1337x',
      attrs: { source: 'WEB', languages: ['multi'], group: 'FW' }, score: 67,
    },
    {
      raw: `${n}.${y}.REPACK.1080p.AMZN.WEB-DL.DDP5.1.Atmos.H.264-FLUX`,
      gb: 8.4, seeders: 122, leechers: 9, days: 296, indexer: 'TorrentLeech',
      attrs: { audio: ['DD+', 'Atmos'], channels: '5.1', group: 'FLUX', repack: true }, score: 87,
    },
    {
      raw: `${n}.${y}.1080p.BluRay.DD+7.1.x264-DON`,
      gb: 14.8, seeders: 0, leechers: 2, days: 285, indexer: 'The Pirate Bay',
      attrs: { source: 'BluRay', audio: ['DD+'], channels: '7.1', group: 'DON' }, score: 30, warnings: ['No seeders · it may never start'],
    },
  ];
}

// ---- Per context ------------------------------------------------------------------------------

const FRIEREN = 209867;
const SEVERANCE = 95396;

const releasesOf = (tmdbId: number, specs: Spec[]) => specs.map((spec) => build(`rel-${tmdbId}-${hashOf(spec.raw).slice(0, 10)}`, spec));

/** What the indexers find for a title (and its season and episode); undefined for a title TMDB doesn't know. */
export function searchFixture(mediaType: MediaType, tmdbId: number, context: { season?: number; episode?: number } = {}): SearchFixture | undefined {
  const entry = findTitle(mediaType, tmdbId);
  if (!entry) return undefined;
  if (entry.media_type === 'movie') {
    const name = normaliseTitle(entry.title);
    return {
      queries: [{ q: `${name} ${entry.year}`, label: null }],
      indexers: ['TorrentLeech', 'The Pirate Bay', '1337x'],
      releases: releasesOf(tmdbId, movieSpecs(entry)),
    };
  }
  if (tmdbId === FRIEREN) {
    return { queries: frierenQueries(context.season, context.episode), indexers: ['Nyaa', 'TorrentLeech', 'The Pirate Bay'], releases: FRIEREN_RELEASES };
  }
  const show = SHOWS[tmdbId];
  const name = normaliseTitle(entry.title);
  const { season: s, episode: e } = context;
  const q = s !== undefined && e !== undefined ? `${name} S${pad(s)}E${pad(e)}` : s !== undefined ? `${name} S${pad(s)}` : name;
  return {
    queries: [{ q, label: null }],
    indexers: ['TorrentLeech', 'EZTV', 'The Pirate Bay'],
    flaky: tmdbId === SEVERANCE ? 'The Pirate Bay' : undefined,
    releases: show ? releasesOf(tmdbId, showSpecs(entry, show, s, e)) : [],
  };
}
