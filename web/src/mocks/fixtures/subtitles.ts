import type { Subtitle, SubtitleCandidate, SubtitleItemType, SubtitleMatch, SubtitleSearchResult } from '@/lib/api/contract';
import { iso } from './catalog';
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

/**
 * Items with an assigned release: its name, and what OpenSubtitles has for it
 * (the exact release, the same source from another group, another cut).
 */
const RELEASES: Record<string, { release: string; names: [string, string, string] }> = {
  [subtitleItemKey('movie', 1)]: {
    release: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    names: [
      'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
      'Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb',
      'Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS',
    ],
  },
  [subtitleItemKey('movie', 2)]: {
    release: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
    names: [
      'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
      'Interstellar.2014.1080p.BluRay.x264-SPARKS',
      'Interstellar.2014.IMAX.1080p.WEB-DL.DD5.1.H.264-NTG',
    ],
  },
  [subtitleItemKey('episode', episodeId(95396, 2, 5))]: {
    release: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    names: [
      'Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
      'Severance.S02E05.1080p.ATVP.WEB-DL.DDP5.1.H.264-NTb',
      'Severance.S02E05.720p.HDTV.x264-SYNCOPY',
    ],
  },
  [subtitleItemKey('episode', episodeId(209867, 1, 28))]: {
    release: '[SubsPlease] Sousou no Frieren (01-28) (1080p) [Batch]',
    names: [
      '[SubsPlease] Sousou no Frieren - 28 (1080p)',
      '[Erai-raws] Sousou no Frieren - 28 [1080p][Multiple Subtitle]',
      'Frieren.S01E28.1080p.BluRay.x264-iKaos',
    ],
  },
};

const RANKS: Array<{ match: SubtitleMatch; score: number; reason: string; downloads: number }> = [
  { match: 'exact_release', score: 100, reason: 'Made for this exact release', downloads: 312 },
  { match: 'same_source', score: 74, reason: 'Same source, different release group', downloads: 1204 },
  { match: 'different_timing', score: 31, reason: 'Different source or cut: the timing may drift', downloads: 88 },
];

/** Stable per language and rank, so a download can find the candidate again. */
const fileId = (code: string, rank: number) => 7_000_000 + code.charCodeAt(0) * 1000 + (code.charCodeAt(1) || 0) * 10 + rank;

/** GET …/subtitles/search: three ranked candidates per language, or unranked ones when nothing is assigned. */
export function subtitleSearch(type: SubtitleItemType, id: number, languages: string[]): SubtitleSearchResult {
  const known = RELEASES[subtitleItemKey(type, id)];
  const names = known?.names ?? ['Release.1080p.WEB-DL.H.264-NTb', 'Release.720p.HDTV.x264-LOL', 'Release.1080p.BluRay.x264-SPARKS'];
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
      match: known ? rank.match : 'unknown',
      match_score: known ? rank.score : 0,
      match_reason: known ? rank.reason : 'Nothing assigned to compare with',
    }))
  );
  return { items, release_name: known?.release ?? null };
}

/** Stored subtitles a page starts with: each kind of source, and one the torrent brought (can't be deleted). */
export function seedSubtitles(): Map<string, Subtitle[]> {
  const stored = (id: number, code: string, source: Subtitle['source'], key: string, daysAgo: number, format = 'srt'): Subtitle => ({
    id,
    language_code: code,
    language_name: languageName(code),
    format,
    source,
    release_name: RELEASES[key]?.release ?? null,
    match: 'exact_release',
    created_at: iso(daysAgo),
  });
  const dune = subtitleItemKey('movie', 1);
  const severance = subtitleItemKey('episode', episodeId(95396, 2, 5));
  const frieren = subtitleItemKey('episode', episodeId(209867, 1, 28));
  return new Map([
    [dune, [stored(1, 'en', 'torrent', dune, 12), stored(2, 'ru', 'opensubtitles', dune, 10)]],
    [severance, [stored(3, 'en', 'torrent', severance, 30)]],
    [frieren, [stored(4, 'en', 'torrent', frieren, 20, 'ass')]],
  ]);
}
