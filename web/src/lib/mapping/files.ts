/**
 * What a file in a torrent is by its extension, as the backend classifies it
 * (`identify.ClassOf`): only videos, and subtitles no video claims, can be
 * given episodes. Fonts, info files, images and archives can't (422).
 */

export type FileClass = 'video' | 'subtitle' | 'font' | 'image' | 'info' | 'archive' | 'other';

const CLASSES: Record<Exclude<FileClass, 'other'>, string[]> = {
  video: ['mkv', 'mp4', 'avi', 'wmv', 'mov', 'm4v', 'webm', 'ts', 'm2ts', 'vob', 'flv', 'divx', 'mpg', 'mpeg', 'ogm'],
  subtitle: ['srt', 'ass', 'ssa', 'sub', 'idx', 'vtt', 'smi', 'sup'],
  font: ['ttf', 'otf', 'ttc', 'woff', 'woff2'],
  image: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'tbn'],
  info: ['nfo', 'txt', 'md', 'url', 'html', 'htm', 'sfv', 'md5', 'sha1', 'srr'],
  archive: ['zip', 'rar', '7z', 'tar', 'gz'],
};

const BY_EXTENSION = new Map<string, FileClass>(
  Object.entries(CLASSES).flatMap(([fileClass, extensions]) => extensions.map((extension) => [extension, fileClass as FileClass]))
);

export function fileClass(path: string): FileClass {
  const extension = /\.([a-z0-9]{1,5})$/i.exec(path)?.[1]?.toLowerCase();
  if (!extension) return 'other';
  if (/^r\d\d$/.test(extension)) return 'archive';
  return BY_EXTENSION.get(extension) ?? 'other';
}

/**
 * The files that can be given an episode (Include on an extra, the picker), as the backend's
 * `takesEpisodes`: a video, or a subtitle file listed as a subtitle.
 */
export function canHaveEpisodes(p: { file_path: string; kind: string }): boolean {
  const kind = fileClass(p.file_path);
  return kind === 'video' || (kind === 'subtitle' && p.kind === 'subtitle');
}
