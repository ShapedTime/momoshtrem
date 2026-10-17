'use client';

import { useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CheckCheck, Pause, Tv } from 'lucide-react';
import { MappingJobsBanner } from '@/components/mapping';
import { Button, ButtonLink, EmptyState } from '@/components/ui';
import { usePendingRemovals } from '@/hooks/useUndoableRemoval';
import { isApiError } from '@/lib/api/client';
import type { StreamingSession, TorrentListItem } from '@/lib/api/contract';
import { downloadsQuery, torrentsListQuery, torrentsSummaryQuery } from '@/lib/api/endpoints';
import { downloadRemovalKey, downloadingLine, withoutHolds } from '@/lib/activity/downloads';
import { formatTimeOfDay } from '@/lib/activity/format';
import { torrentRemovalKey } from '@/lib/activity/remove';
import { groupTorrents, isActive, isQuiet, onlyIdle, streamingNow } from '@/lib/activity/sections';
import { statusLine } from '@/lib/activity/status';
import { useSheet } from '@/lib/sheets/useSheet';
import { cx } from '@/lib/utils/cx';
import { ActivityLoading, ActivitySkeletonBody } from './ActivitySkeleton';
import { DownloadsSection } from './DownloadsSection';
import { OfflineBanner } from './OfflineBanner';
import { SummaryStrip } from './SummaryStrip';
import { type CardHandlers, TorrentSection } from './TorrentSection';
import { useTorrentActions } from './useTorrentActions';

/** Speeds sample every 2 s on the server; the list and the strip follow every 5 s while the tab is visible. */
export const ACTIVITY_POLL_MS = 5000;

/** A backend without downloads (#77's P4 not deployed) has no /downloads: no "Kept & smart" list, and no error. */
const noDownloads = (error: unknown) => isApiError(error) && error.status === 404;

/**
 * /activity (PLAN F7, #102; screens/activity): is everything working? A status
 * line, the summary strip, then the torrents in four sections: Streaming now,
 * Downloading, Paused and Idle, with the kept and smart downloads (#77) and the
 * idle torrents in a side column on desktop. Offline, it keeps what momoshtrem
 * last said, greyed and inert, and retries on a countdown.
 */
export function ActivityView() {
  const pending = usePendingRemovals();
  const actions = useTorrentActions();
  const sheets = useSheet();

  // A failed poll hands the retries to the offline banner's countdown
  const list = useQuery({ ...torrentsListQuery(), refetchInterval: (query) => (query.state.status === 'error' ? false : ACTIVITY_POLL_MS) });
  const offline = list.isError;
  const summary = useQuery({ ...torrentsSummaryQuery(), refetchInterval: offline ? false : ACTIVITY_POLL_MS });
  // Downloads move with the torrents (a smart pick appears, a wait ends), so they follow the page's poll;
  // a failed fetch keeps polling (its banner or note says so), and only a backend without /downloads stops it
  const downloads = useQuery({ ...downloadsQuery(), refetchInterval: (query) => (offline || noDownloads(query.state.error) ? false : ACTIVITY_POLL_MS) });
  const downloadsAbsent = noDownloads(downloads.error);

  // Torrents and downloads waiting out their Undo are hidden everywhere on the page, a released download's hold too
  const items = useMemo(
    () =>
      (list.data?.items ?? [])
        .filter((item) => !pending.has(torrentRemovalKey(item.info_hash)))
        .map((item) => withoutHolds(item, (id) => pending.has(downloadRemovalKey(id)))),
    [list.data, pending]
  );
  const kept = useMemo(
    () => (downloadsAbsent ? [] : (downloads.data?.items ?? []).filter((download) => !pending.has(downloadRemovalKey(download.id)))),
    [downloads.data, downloadsAbsent, pending]
  );
  const sections = groupTorrents(items);
  const now = streamingNow(summary.data?.streaming ?? [], items);
  // The page shows once both have answered, so "Nothing playing" is never a guess; the kept downloads join when they answer
  const ready = list.data !== undefined && (summary.data !== undefined || summary.isError);
  const quiet = isQuiet(items);
  const wide = onlyIdle(items);
  const canPauseAll = !offline && items.some(isActive);

  const { refetch: refetchList } = list;
  const { refetch: refetchSummary } = summary;
  const { refetch: refetchDownloads } = downloads;
  const retry = useCallback(() => Promise.all([refetchList(), refetchSummary(), refetchDownloads()]), [refetchList, refetchSummary, refetchDownloads]);

  const handlers: CardHandlers = { onPause: actions.pause, onResume: actions.resume, onRemove: (torrent) => void actions.remove(torrent), busy: actions.busy };
  const showDownloads = kept.length > 0 || (downloads.isError && !downloadsAbsent && !offline);
  const keptSection = showDownloads ? (
    <DownloadsSection
      list={downloads.data}
      items={kept}
      failed={downloads.isError && !downloadsAbsent && !offline}
      updatedAt={downloads.dataUpdatedAt}
      retrying={downloads.isFetching}
      onRetry={() => void refetchDownloads()}
    />
  ) : null;
  const hasSide = sections.idle.length > 0 || showDownloads;

  return (
    <div className="act-page">
      <header className="page act-head">
        <h1 className="t-display">Activity</h1>
        {canPauseAll && (
          <Button variant="ghost" size="sm" onClick={actions.pauseAll} loading={actions.pausingAll}>
            <Pause size={15} aria-hidden="true" /> Pause all
          </Button>
        )}
      </header>

      <div className="page act-body">
        {!ready && !offline && <ActivityLoading />}

        {offline && (
          <>
            <OfflineBanner since={list.errorUpdatedAt} lastKnown={list.data ? list.dataUpdatedAt : null} onRetry={retry} />
            {list.data ? (
              <p className="act-lastknown">Last known · {formatTimeOfDay(list.dataUpdatedAt)}</p>
            ) : (
              <div className="flex flex-col gap-8">
                <ActivitySkeletonBody />
              </div>
            )}
          </>
        )}

        {list.data && (ready || offline) && (
          <>
            {!offline && <StatusLine items={items} now={now} />}
            <SummaryStrip summary={summary.data} items={items} now={now} stale={offline} />
            <div className={cx('flex flex-col gap-8', offline && 'is-stale')} inert={offline || undefined} aria-label={offline ? 'Last known torrents (offline)' : undefined} role={offline ? 'group' : undefined}>
              {!offline && <MappingJobsBanner />}
              {wide ? (
                <>
                  {quiet && (
                    <EmptyState
                      cat="sleep"
                      title="Nothing downloading."
                      className="act-quiet"
                      headingLevel="h2"
                      actions={
                        <>
                          <ButtonLink href="/library?filter=ready">See what’s ready</ButtonLink>
                          <Button variant="ghost" onClick={() => sheets.open('infuse-setup')}>
                            <Tv size={18} aria-hidden="true" /> How to play
                          </Button>
                        </>
                      }
                    >
                      Infuse can still play everything that’s ready.
                    </EmptyState>
                  )}
                  <div className="act-groups is-wide">
                    {keptSection}
                    <TorrentSection state="idle" items={sections.idle} {...handlers} />
                  </div>
                </>
              ) : (
                <div className={cx('act-groups', hasSide && 'has-side')}>
                  <div className="act-col-main">
                    <TorrentSection state="streaming" items={sections.streaming} {...handlers} />
                    <TorrentSection state="downloading" items={sections.downloading} sub={downloadingLine(sections.downloading)} {...handlers} />
                    <TorrentSection state="paused" items={sections.paused} {...handlers} />
                  </div>
                  {hasSide && (
                    <div className="act-col-side">
                      {keptSection}
                      <TorrentSection state="idle" items={sections.idle} compact {...handlers} />
                    </div>
                  )}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/** "Everything's working. Infuse is streaming smoothly." (activity.js `verdict`) */
function StatusLine({ items, now }: { items: readonly TorrentListItem[]; now: StreamingSession | null }) {
  const line = statusLine(items, now);
  return (
    <p className="act-verdict">
      <CheckCheck size={18} className="ok" aria-hidden="true" />
      <span>
        {line.lead && <strong>{line.lead}</strong>}
        {line.lead && line.text ? ' ' : ''}
        {line.text}
      </span>
    </p>
  );
}
