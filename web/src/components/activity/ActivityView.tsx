'use client';

import { useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CheckCheck, Pause, Tv } from 'lucide-react';
import { MappingJobsBanner } from '@/components/mapping';
import { Button, ButtonLink, EmptyState } from '@/components/ui';
import { usePendingRemovals } from '@/hooks/useUndoableRemoval';
import type { StreamingSession, TorrentListItem } from '@/lib/api/contract';
import { torrentsListQuery, torrentsSummaryQuery } from '@/lib/api/endpoints';
import { formatTimeOfDay } from '@/lib/activity/format';
import { torrentRemovalKey } from '@/lib/activity/remove';
import { groupTorrents, isActive, isQuiet, streamingNow } from '@/lib/activity/sections';
import { statusLine } from '@/lib/activity/status';
import { useSheet } from '@/lib/sheets/useSheet';
import { cx } from '@/lib/utils/cx';
import { ActivityLoading, ActivitySkeletonBody } from './ActivitySkeleton';
import { OfflineBanner } from './OfflineBanner';
import { SummaryStrip } from './SummaryStrip';
import { type CardHandlers, TorrentSection } from './TorrentSection';
import { useTorrentActions } from './useTorrentActions';

/** Speeds sample every 2 s on the server; the list and the strip follow every 5 s while the tab is visible. */
export const ACTIVITY_POLL_MS = 5000;

/**
 * /activity (PLAN F7, #102; screens/activity): is everything working? A status
 * line, the summary strip, then the torrents in four sections: Streaming now,
 * Downloading, Paused and Idle (a side column on desktop). Offline, it keeps
 * what momoshtrem last said, greyed and inert, and retries on a countdown.
 */
export function ActivityView() {
  const pending = usePendingRemovals();
  const actions = useTorrentActions();
  const sheets = useSheet();

  // A failed poll hands the retries to the offline banner's countdown
  const list = useQuery({ ...torrentsListQuery(), refetchInterval: (query) => (query.state.status === 'error' ? false : ACTIVITY_POLL_MS) });
  const offline = list.isError;
  const summary = useQuery({ ...torrentsSummaryQuery(), refetchInterval: offline ? false : ACTIVITY_POLL_MS });

  // Torrents waiting out their Undo are hidden everywhere on the page
  const items = useMemo(() => (list.data?.items ?? []).filter((item) => !pending.has(torrentRemovalKey(item.info_hash))), [list.data, pending]);
  const sections = groupTorrents(items);
  const now = streamingNow(summary.data?.streaming ?? [], items);
  // The page shows once both have answered, so "Nothing playing" is never a guess
  const ready = list.data !== undefined && (summary.data !== undefined || summary.isError);
  const quiet = isQuiet(items);
  const canPauseAll = !offline && items.some(isActive);

  const { refetch: refetchList } = list;
  const { refetch: refetchSummary } = summary;
  const retry = useCallback(() => Promise.all([refetchList(), refetchSummary()]), [refetchList, refetchSummary]);

  const handlers: CardHandlers = { onPause: actions.pause, onResume: actions.resume, onRemove: (torrent) => void actions.remove(torrent), busy: actions.busy };

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
              {quiet ? (
                <>
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
                  <div className="act-groups is-wide">
                    <TorrentSection state="idle" items={sections.idle} {...handlers} />
                  </div>
                </>
              ) : (
                <div className={cx('act-groups', sections.idle.length > 0 && 'has-side')}>
                  <div className="act-col-main">
                    <TorrentSection state="streaming" items={sections.streaming} {...handlers} />
                    <TorrentSection state="downloading" items={sections.downloading} {...handlers} />
                    <TorrentSection state="paused" items={sections.paused} {...handlers} />
                  </div>
                  {sections.idle.length > 0 && (
                    <div className="act-col-side">
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
