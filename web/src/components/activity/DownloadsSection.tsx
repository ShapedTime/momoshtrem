'use client';

import Link from 'next/link';
import { useCallback } from 'react';
import { Clock, Pause, Pin, Sparkles, Tv, type LucideIcon } from 'lucide-react';
import { Banner, Button, ButtonLink, Media, Progress, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useUndoableRemoval } from '@/hooks/useUndoableRemoval';
import { apiKeepalive, isApiError } from '@/lib/api/client';
import type { Download, DownloadList, DownloadsPausedReason } from '@/lib/api/contract';
import { downloadKeys, downloadPath, downloadsApi, libraryKeys, settingsKeys, titleKeys, torrentKeys } from '@/lib/api/endpoints';
import { downloadName, downloadRemovalKey, downloadRow, downloadsUsageLine, pausedBanner, stopKeepingCopy } from '@/lib/activity/downloads';
import { formatNumber } from '@/lib/title/format';
import { STORAGE_SETTINGS_HREF } from '@/lib/title/keep';
import { cx } from '@/lib/utils/cx';

/** What Stop keeping changes: the list, the torrents' holds, the titles' `keep`, and the storage usage. */
const AFFECTED = [downloadKeys.all, torrentKeys.all, titleKeys.all, libraryKeys.all, settingsKeys.storage] as const;

const BANNER_ICON: Record<DownloadsPausedReason, LucideIcon> = { streaming: Tv, window: Clock, disabled: Pause };

/** Stop keeping with a 10 s Undo; the DELETE goes when the toast closes. One already released is what was asked for. */
function useStopKeeping() {
  const removeLater = useUndoableRemoval();
  return useCallback(
    (download: Download) => {
      const copy = stopKeepingCopy(download);
      removeLater({
        key: downloadRemovalKey(download.id),
        label: downloadName(download),
        payload: download,
        commit: () =>
          downloadsApi.remove(download.id).catch((error: unknown) => {
            if (!(isApiError(error) && error.status === 404)) throw error;
          }),
        keepalive: () => apiKeepalive(downloadPath(download.id), 'DELETE'),
        invalidate: AFFECTED,
        copy: { title: copy.title, body: copy.body, failed: copy.failed, icon: download.reason === 'smart' ? Sparkles : Pin },
      });
    },
    [removeLater]
  );
}

function DownloadRowItem({ download, onStop }: { download: Download; onStop: (download: Download) => void }) {
  const row = downloadRow(download);
  const copy = stopKeepingCopy(download);
  const name = downloadName(download);
  const meta = [row.size, row.files, row.retry].filter(Boolean).join(' · ');
  const ReasonIcon = download.reason === 'smart' ? Sparkles : Pin;
  return (
    <li className={cx('dlrow', download.state === 'paused' && 'is-paused')} aria-label={name}>
      <Link href={row.href} className="poster-link" aria-hidden="true" tabIndex={-1}>
        <Media shape="poster" src={buildImageUrl(download.item.poster_path, 'poster', 'small')} sizes="40px" />
      </Link>
      <div className="dl-body">
        <div className="top">
          <div className="dl-id">
            <Link href={row.href} className="label">
              {row.title}
            </Link>
            {row.scope && <span className="scope">{row.scope}</span>}
          </div>
          <StatusPill status={row.pill.status} label={row.pill.label} pct={row.pill.pct} />
        </div>
        {row.pct !== null && <Progress value={row.pct} label={`${name} download progress`} />}
        {meta && <span className="meta">{meta}</span>}
        <div className="foot">
          <span className={cx('expiry', `is-${download.reason}`)}>
            <ReasonIcon size={14} aria-hidden="true" />
            {row.expiry}
          </span>
          <Button variant="ghost" size="sm" onClick={() => onStop(download)} aria-label={`${copy.action}: ${name}`}>
            {copy.action}
          </Button>
        </div>
      </div>
    </li>
  );
}

interface DownloadsSectionProps {
  list: DownloadList | undefined;
  /** The rows to show: the list's, less those waiting out a Stop keeping's Undo. */
  items: readonly Download[];
  /** GET /downloads failed (not a 404: without downloads the section isn't there at all). */
  failed: boolean;
  retrying: boolean;
  onRetry: () => void;
}

/**
 * "Kept & smart" (#77, GET /downloads): every download, its state, size,
 * progress and expiry, with Stop keeping; above them, why background downloads
 * wait now (`paused_reason`). The side column on desktop, below the busy
 * sections on phone. Hidden when there are none.
 */
export function DownloadsSection({ list, items, failed, retrying, onRetry }: DownloadsSectionProps) {
  const stop = useStopKeeping();
  if (!list) {
    return failed ? (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load your kept downloads"
        action={
          <Button size="sm" onClick={onRetry} loading={retrying}>
            Try again
          </Button>
        }
      >
        The torrents on this page are up to date, and what’s on disk still plays in Infuse.
      </Banner>
    ) : null;
  }
  if (items.length === 0) return null;
  const shown = { ...list, items };
  const banner = pausedBanner(shown);
  return (
    <section className="act-group act-dl" aria-labelledby="act-dl-h">
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-3" id="act-dl-h">
            Kept &amp; smart <span className="count">{formatNumber(items.length)}</span>
          </h2>
          <p className="t-small text-fg-3">{downloadsUsageLine(shown)}</p>
        </div>
      </div>
      {banner && list.paused_reason && (
        <Banner
          role="status"
          icon={BANNER_ICON[list.paused_reason]}
          title={banner.title}
          action={
            banner.settings ? (
              <ButtonLink href={STORAGE_SETTINGS_HREF} variant="ghost" size="sm">
                Storage settings
              </ButtonLink>
            ) : undefined
          }
        >
          {banner.body}
        </Banner>
      )}
      <ul className="act-dl-list">
        {items.map((download) => (
          <DownloadRowItem key={download.id} download={download} onStop={stop} />
        ))}
      </ul>
    </section>
  );
}
