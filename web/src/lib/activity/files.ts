import type { StreamingSession, TorrentDetail, TorrentDetailFile } from '@/lib/api/contract';
import { formatNumber } from '@/lib/title/format';

/**
 * The detail page's file list (activity.js `fileRow`): what each file plays
 * as, its size and progress, and the one Infuse is reading.
 */

/** Past this many files the list shows the first `FILES_PREVIEW` and "Show all N files". */
export const FILES_COLLAPSE_OVER = 14;
export const FILES_PREVIEW = 10;

/** The folder every file sits in ("Name/" in a multi-file torrent), or null for a single file. */
export function filesRoot(files: readonly Pick<TorrentDetailFile, 'path'>[]): string | null {
  const first = files[0]?.path;
  const slash = first?.indexOf('/') ?? -1;
  if (!first || slash <= 0) return null;
  const root = first.slice(0, slash + 1);
  return files.every((file) => file.path.startsWith(root)) ? root : null;
}

/** A file's path inside the root, so rows don't repeat the torrent's folder. */
export const relativePath = (path: string, root: string | null) => (root && path.startsWith(root) ? path.slice(root.length) : path);

/** "10 mapped · 3 ignored · in {root}": the parts of the line under "Files N". */
export function filesSummary(files: readonly TorrentDetailFile[]): { mapped: number; ignored: number; root: string | null; text: string } {
  const mapped = files.filter((file) => file.plays_as.length > 0).length;
  const ignored = files.filter((file) => file.plays_as.length === 0 && file.ignored !== null).length;
  const root = filesRoot(files);
  const text = [`${formatNumber(mapped)} mapped`, ignored ? `${formatNumber(ignored)} ignored` : null].filter(Boolean).join(' · ');
  return { mapped, ignored, root, text };
}

const SUBTITLE = /\.(srt|ass|ssa|vtt|sub|idx|sup)$/i;

/**
 * What the "Plays as" column says for a file with no target and not ignored:
 * a subtitle that follows its video, or a file the mapping hasn't placed (yet,
 * while a job is open).
 */
export function unmappedLabel(path: string, openJob: boolean): string {
  if (SUBTITLE.test(path)) return 'Subtitle';
  return openJob ? 'Not mapped yet' : 'Not mapped';
}

export type FileProgress = { kind: 'ready' } | { kind: 'partial'; pct: number } | { kind: 'skipped' } | { kind: 'unknown' };

/** "✓ Ready", a bar and %, "Skipped" for an ignored file, or "—" while the torrent isn't loaded. */
export function fileProgress(file: Pick<TorrentDetailFile, 'progress' | 'ignored' | 'plays_as'>): FileProgress {
  if (file.plays_as.length === 0 && file.ignored) return { kind: 'skipped' };
  if (file.progress === null) return { kind: 'unknown' };
  if (file.progress >= 1) return { kind: 'ready' };
  return { kind: 'partial', pct: Math.floor(file.progress * 100) };
}

/** The files sessions read now, by path (the newest session first wins); a paused torrent's lingering sessions don't count. */
export function readersByPath(detail: Pick<TorrentDetail, 'state' | 'streaming'>): Map<string, StreamingSession> {
  const readers = new Map<string, StreamingSession>();
  if (detail.state === 'paused') return readers;
  for (const session of detail.streaming) {
    if (session.file_path !== null && !readers.has(session.file_path)) readers.set(session.file_path, session);
  }
  return readers;
}
