import type { EpisodeStream, Subtitle, SubtitleCandidate, SubtitleItemType, SubtitleMatch, SubtitleSearchResult } from '@/lib/api/contract';
import { iso } from './catalog';
import { MOVIES, SHOWS } from './title-seeds';
import { episodeId } from './titles';

const LANGUAGES: Record<string, string> = {
  en: 'English',
  ru: 'Russian',
  tr: 'Turkish',
  az: 'Azerbaijani',
  ja: 'Japanese',
  de: 'German',
  fr: 'French',
  es: 'Spanish',
};

export const languageName = (code: string) => LANGUAGES[code] ?? code.toUpperCase();

export const subtitleItemKey = (type: SubtitleItemType, id: number) => `${type}:${id}`;

const DUNE = subtitleItemKey('movie', 1);
const SEVERANCE_S02E05 = subtitleItemKey('episode', episodeId(95396, 2, 5));
const FRIEREN_S01E28 = subtitleItemKey('episode', episodeId(209867, 1, 28));

/** What OpenSubtitles has for a few items: the exact release, the same source from another group, another cut. */
const NAMES: Record<string, [string, string, string]> = {
  [DUNE]: [
    'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    'Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb',
    'Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS',
  ],
  [subtitleItemKey('movie', 2)]: [
    'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
    'Interstellar.2014.1080p.BluRay.x264-SPARKS',
    'Interstellar.2014.IMAX.1080p.WEB-DL.DD5.1.H.264-NTG',
  ],
  [SEVERANCE_S02E05]: [
    'Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    'Severance.S02E05.1080p.ATVP.WEB-DL.DDP5.1.H.264-NTb',
    'Severance.S02E05.720p.HDTV.x264-SYNCOPY',
  ],
  [FRIEREN_S01E28]: [
    '[SubsPlease] Sousou no Frieren - 28 (1080p)',
    '[Erai-raws] Sousou no Frieren - 28 [1080p][Multiple Subtitle]',
    'Frieren.S01E28.1080p.BluRay.x264-iKaos',
  ],
};

const OTHER_NAMES = ['Release.1080p.WEB-DL.H.264-NTb', 'Release.720p.HDTV.x264-LOL'];

const RANKS: Array<{ match: SubtitleMatch; score: number; reason: string; downloads: number }> = [
  { match: 'exact_release', score: 100, reason: 'Made for this exact release', downloads: 312 },
  { match: 'same_source', score: 74, reason: 'Same source, different release group', downloads: 1204 },
  { match: 'different_timing', score: 31, reason: 'Different source or cut: the timing may drift', downloads: 88 },
];

/** Stable per language and rank, so a download can find the candidate again. */
const fileId = (code: string, rank: number) => 7_000_000 + code.charCodeAt(0) * 1000 + (code.charCodeAt(1) || 0) * 10 + rank;

/**
 * GET …/subtitles/search: three candidates per language, ranked against the
 * item's release (`itemRelease`); unranked while it has no file.
 */
export function subtitleSearch(type: SubtitleItemType, id: number, languages: string[], release: string | null): SubtitleSearchResult {
  const names = NAMES[subtitleItemKey(type, id)] ?? [release ?? 'Release.2160p.WEB-DL.H.265-FLUX', ...OTHER_NAMES];
  const items = languages.flatMap((code) =>
    RANKS.map((rank, i): SubtitleCandidate => ({
      file_id: fileId(code, i),
      language_code: code,
      language_name: languageName(code),
      release_name: names[i],
      file_name: `${names[i]}.${code}.srt`,
      downloads: rank.downloads,
      hearing_impaired: i === 1,
      machine_translated: i === 2,
      match: release ? rank.match : 'unknown',
      match_score: release ? rank.score : 0,
      match_reason: release ? rank.reason : 'Nothing assigned to compare with',
    }))
  );
  return { items, release_name: release };
}

/** The voice-over with the original audio: HDRezka ships it with English and Russian subtitles, the dubs with none (fixtures/hdrezka.ts). */
const ORIGINAL_AUDIO = 'Оригинал (+субтитры)';

/** An HDRezka stream as its subtitles' release name: "HDRezka · Оригинал (+субтитры) · 1080p", as the backend labels it. */
export const streamRelease = (stream: EpisodeStream) => `HDRezka · ${stream.translator} · ${stream.quality}`;

/**
 * The subtitles an HDRezka stream brings (#68): the backend downloads them
 * when the stream is assigned and stores them as SRT. English and Russian
 * for the original audio, none for a dub. Ids count up from `firstId`.
 */
export function hdrezkaSubtitles(stream: EpisodeStream, firstId: number): Subtitle[] {
  if (stream.translator !== ORIGINAL_AUDIO) return [];
  return ['en', 'ru'].map((code, i) => ({
    id: firstId + i,
    language_code: code,
    language_name: languageName(code),
    format: 'srt',
    source: 'hdrezka',
    release_name: streamRelease(stream),
    match: 'exact_release',
    created_at: new Date().toISOString(),
  }));
}

/** Stored subtitles a page starts with: each kind of source, and some the torrent brought (can't be deleted). */
export function seedSubtitles(): Map<string, Subtitle[]> {
  const stored = (id: number, code: string, source: Subtitle['source'], release: string | null, daysAgo: number, format = 'srt'): Subtitle => ({
    id,
    language_code: code,
    language_name: languageName(code),
    format,
    source,
    release_name: release,
    match: 'exact_release',
    created_at: iso(daysAgo),
  });
  const dune = MOVIES[693134].torrent?.release_name ?? null;
  return new Map([
    [DUNE, [stored(1, 'en', 'torrent', dune, 12), stored(2, 'ru', 'opensubtitles', dune, 10)]],
    [SEVERANCE_S02E05, [stored(3, 'en', 'torrent', SHOWS[95396].release ?? null, 30)]],
    [FRIEREN_S01E28, [stored(4, 'en', 'torrent', SHOWS[209867].release ?? null, 20, 'ass')]],
  ]);
}
