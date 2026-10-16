'use client';

import { useState } from 'react';
import { Check, Tv } from 'lucide-react';
import { Button, Chip, FilePath, Progress } from '@/components/ui';
import type { StreamingSession, TorrentDetail, TorrentDetailFile } from '@/lib/api/contract';
import { FILES_COLLAPSE_OVER, FILES_PREVIEW, fileProgress, filesSummary, relativePath } from '@/lib/activity/files';
import { formatBytes, formatNumber } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

function FileRow({ file, root, reader }: { file: TorrentDetailFile; root: string | null; reader: StreamingSession | null }) {
  const progress = fileProgress(file);
  const ignored = file.plays_as.length === 0 && file.ignored !== null;
  const [single] = file.plays_as;
  const who = reader?.client && reader.client !== 'Unknown' ? reader.client : 'Infuse';
  return (
    <li className={cx('td-file', ignored && 'is-ignored', reader && 'is-reading')}>
      <div className="f-name">
        {reader && (
          <span className="pill pill--accent f-badge">
            <Tv size={13} aria-hidden="true" />
            {who} is reading
          </span>
        )}
        <FilePath path={relativePath(file.path, root)} />
      </div>
      <div className="f-map">
        {ignored ? (
          <Chip>{file.ignored?.label}</Chip>
        ) : file.plays_as.length > 0 ? (
          <>
            <span className="flex flex-wrap gap-1">
              {file.plays_as.map((target) => (
                <Chip key={`${target.media_type}-${target.item_id}`} tone="hi">
                  {target.label}
                </Chip>
              ))}
            </span>
            {file.plays_as.length === 1 && single.episode_name && <span className="t-small clamp-1 text-fg-3">{single.episode_name}</span>}
          </>
        ) : (
          <span className="t-small text-fg-3">Not mapped yet</span>
        )}
      </div>
      <div className="f-size">{formatBytes(file.size_bytes)}</div>
      {/* On phone, rows skip what needs no attention: Ready, Skipped and unknown */}
      <div className={cx('f-prog', progress.kind !== 'partial' && 'is-quiet')}>
        {progress.kind === 'ready' && (
          <span className="f-done">
            <Check size={14} strokeWidth={2.4} aria-hidden="true" />
            Ready
          </span>
        )}
        {progress.kind === 'partial' && (
          <>
            <Progress value={progress.pct} tone={reader ? 'accent' : 'downloading'} label={`${relativePath(file.path, root)} progress`} />
            <span className="pct">{progress.pct}%</span>
          </>
        )}
        {progress.kind === 'skipped' && <span className="t-small text-fg-3">Skipped</span>}
        {progress.kind === 'unknown' && (
          <span className="text-fg-3">
            <span aria-hidden="true">—</span>
            <span className="sr-only">Progress unknown until the torrent loads</span>
          </span>
        )}
      </div>
    </li>
  );
}

/**
 * Files N (activity.js `fileRow`): what each file plays as, its size and
 * progress, and the one being read with an amber rule. Past 14 files, the
 * first 10 and "Show all N files".
 */
export function FilesPanel({ detail, readers }: { detail: TorrentDetail; readers: ReadonlyMap<string, StreamingSession> }) {
  const [all, setAll] = useState(false);
  const { files } = detail;
  const summary = filesSummary(files);
  const collapsed = files.length > FILES_COLLAPSE_OVER && !all;
  const shown = collapsed ? files.slice(0, FILES_PREVIEW) : files;

  return (
    <section className="panel td-files" aria-labelledby="td-files-h">
      <div className="panel-head td-files-head">
        <div className="flex min-w-0 flex-col">
          <h2 className="t-title-3" id="td-files-h">
            Files <span className="font-medium text-fg-3 tabular-nums">{formatNumber(files.length)}</span>
          </h2>
          <span className="t-small text-fg-3">
            {summary.text}
            {summary.root && (
              <>
                {' · in '}
                <FilePath path={summary.root} />
              </>
            )}
          </span>
        </div>
      </div>
      {files.length === 0 ? (
        <p className="t-small px-4 pb-4 text-fg-3">The file list arrives once the torrent’s metadata does.</p>
      ) : (
        <>
          <div className="td-cols" aria-hidden="true">
            <span>File</span>
            <span>Plays as</span>
            <span>Size</span>
            <span>Progress</span>
          </div>
          <ul className="td-filelist" aria-label="Files">
            {shown.map((file) => (
              <FileRow key={file.path} file={file} root={summary.root} reader={readers.get(file.path) ?? null} />
            ))}
          </ul>
          {collapsed && (
            <div className="td-more">
              <Button variant="ghost" size="sm" onClick={() => setAll(true)}>
                Show all {formatNumber(files.length)} files
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
