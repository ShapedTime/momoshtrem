'use client';

import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, CheckCheck, ChevronLeft, Clock, HardDrive, Hourglass, Pause, TriangleAlert, Tv, Users, type LucideIcon } from 'lucide-react';
import { Banner, Button, ButtonLink, EmptyState, LoadingRegion, Media, Progress, ReleaseName, Skeleton, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { isApiError } from '@/lib/api/client';
import type { TorrentDetail } from '@/lib/api/contract';
import { torrentDetailQuery } from '@/lib/api/endpoints';
import { holdScopeLabel, wantedProgress } from '@/lib/activity/downloads';
import { formatRate, formatTimeOfDay, sessionBanner, torrentLabel } from '@/lib/activity/format';
import { readersByPath } from '@/lib/activity/files';
import { type HealthKind, healthFor, isWeak } from '@/lib/activity/health';
import { isActive } from '@/lib/activity/sections';
import { formatBytes, plural } from '@/lib/title/format';
import { keepReasonPill } from '@/lib/title/keep';
import { cx } from '@/lib/utils/cx';
import { AdvancedPanel } from './AdvancedPanel';
import { DetailActions } from './DetailActions';
import { FilesPanel } from './FilesPanel';
import { HoldPills, TorrentStatePill } from './TorrentCard';
import { useTorrentActions } from './useTorrentActions';

const HEALTH_ICON: Record<HealthKind, LucideIcon> = { healthy: CheckCheck, complete: CheckCheck, slow: TriangleAlert, paused: Pause, waiting: Hourglass, idle: Clock };

const BackLink = () => (
  <ButtonLink href="/activity" variant="ghost" size="sm" className="td-back">
    <ChevronLeft size={18} aria-hidden="true" /> Activity
  </ButtonLink>
);

function DetailHeader({ detail }: { detail: TorrentDetail }) {
  const label = torrentLabel(detail);
  const title = detail.titles[0];
  const active = isActive(detail);
  // What its downloads want counts (#77); without downloads, anacrolix's progress for the whole torrent
  const wanted = wantedProgress(detail);
  const pct = detail.wanted === null && detail.progress !== null ? detail.progress * 100 : null;
  const sized = detail.size_bytes !== null && detail.bytes_completed !== null && detail.progress !== null && detail.progress < 1;
  const stats: [LucideIcon, string, string][] = [];
  if (active) {
    stats.push([ArrowDown, 'Down', formatRate(detail.download_bps)], [ArrowUp, 'Up', formatRate(detail.upload_bps)]);
  }
  if (detail.swarm && detail.state !== 'idle') {
    const { connected_peers: peers, connected_seeders: seeders, total_seeders: total } = detail.swarm;
    stats.push([Users, 'Peers', total !== null ? `${peers} of ${plural(total, 'seeder')}` : `${plural(peers, 'peer')}, ${plural(seeders, 'seeder')}`]);
  }
  if (detail.size_bytes !== null) {
    stats.push([HardDrive, 'Size', sized ? `${formatBytes(detail.bytes_completed ?? 0)} of ${formatBytes(detail.size_bytes)}` : formatBytes(detail.size_bytes)]);
  }

  return (
    <header className="td-head">
      {label.href ? (
        <Link href={label.href} className="td-poster" aria-label={title?.title ?? label.text}>
          <Media shape="poster" src={buildImageUrl(title?.poster_path ?? null, 'poster', 'medium')} sizes="(min-width: 1024px) 136px, (min-width: 640px) 112px, 64px" />
        </Link>
      ) : (
        <div className="td-poster">
          <Media shape="poster" />
        </div>
      )}
      <div className="flex min-w-0 flex-col gap-3">
        <div className="flex flex-wrap gap-2">
          <TorrentStatePill torrent={detail} size="lg" />
          <HoldPills holds={detail.holds} size="lg" className="contents" />
          {isWeak(detail) && <StatusPill status="few_peers" size="lg" />}
        </div>
        <h1>
          {label.href ? (
            <Link href={label.href} className="td-title-link">
              {label.text}
            </Link>
          ) : (
            'A torrent not in your library'
          )}
        </h1>
        <ReleaseName name={detail.name} className="td-release" />
        {wanted && (
          <div className={cx('td-prog', detail.state === 'paused' && 'is-paused')}>
            <Progress value={wanted.pct} size="lg" tone={detail.state === 'streaming' ? 'accent' : 'downloading'} label={`${label.text} progress`} />
            <span className="t-small text-fg-2 tabular-nums">
              {wanted.pct}% · {wanted.detail}
            </span>
          </div>
        )}
        {pct !== null && pct < 100 && (
          <div className={cx('td-prog', detail.state === 'paused' && 'is-paused')}>
            <Progress value={pct} size="lg" tone={detail.state === 'streaming' ? 'accent' : 'downloading'} label={`${label.text} progress`} />
            <span className="t-small text-fg-2 tabular-nums">
              {Math.floor(pct)}%{sized && ` · ${formatBytes(detail.bytes_completed ?? 0)} of ${formatBytes(detail.size_bytes ?? 0)}`}
            </span>
          </div>
        )}
        {stats.length > 0 && (
          <dl className="td-stats">
            {stats.map(([Icon, name, value]) => (
              <div key={name}>
                <dt>
                  <Icon size={14} aria-hidden="true" />
                  {name}
                </dt>
                <dd>{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </header>
  );
}

/** The downloads that keep it on disk (#77): Kept or Smart, and what each is on. */
function HoldsPanel({ detail }: { detail: TorrentDetail }) {
  if (detail.holds.length === 0) return null;
  return (
    <section className="panel panel-pad td-holds" aria-labelledby="td-holds-h">
      <h2 className="t-title-3" id="td-holds-h">
        Kept &amp; smart
      </h2>
      <ul>
        {detail.holds.map((hold) => {
          const pill = keepReasonPill(hold.reason);
          return (
            <li key={hold.download_id}>
              <StatusPill status={pill.status} label={pill.label} />
              <span className="grow">
                <strong>{hold.title}</strong>
                <span>{holdScopeLabel(hold.scope)}</span>
              </span>
            </li>
          );
        })}
      </ul>
      <ButtonLink href="/activity#act-dl-h" variant="ghost" size="sm" className="td-holds-link">
        See your kept downloads
      </ButtonLink>
    </section>
  );
}

function DetailSkeleton() {
  return (
    <LoadingRegion label="Loading the torrent" className="flex flex-col gap-5">
      <div className="td-head">
        <Skeleton shape="poster" className="td-poster" />
        <div className="flex flex-col gap-3">
          <Skeleton width={140} height={30} className="rounded-full" />
          <Skeleton shape="line" width="55%" height={28} />
          <Skeleton shape="line" width="80%" />
          <Skeleton height={6} className="max-w-[560px]" />
        </div>
      </div>
      <div className="td-grid">
        <div className="td-aside">
          <Skeleton height={140} className="rounded-lg" />
          <Skeleton height={180} className="rounded-lg" />
        </div>
        <div className="td-main">
          <Skeleton height={360} className="rounded-lg" />
        </div>
      </div>
    </LoadingRegion>
  );
}

/** "That torrent is gone" (activity.js, no torrent): the library entry stays. */
function TorrentGone() {
  return (
    <EmptyState
      cat="peek"
      title="That torrent is gone"
      headingLevel="h1"
      actions={<ButtonLink href="/activity">Back to Activity</ButtonLink>}
    >
      It may have been removed. Your library entry stays put and shows “Needs a torrent”.
    </EmptyState>
  );
}

function DetailBody({ detail, advanced, stale }: { detail: TorrentDetail; advanced: boolean; stale: number | null }) {
  const router = useRouter();
  const actions = useTorrentActions();
  const label = torrentLabel(detail);
  useDocumentTitle(label.text);

  const health = healthFor(detail, detail.streaming[0] ?? null);
  const HealthIcon = HEALTH_ICON[health.kind];
  const session = detail.state === 'streaming' ? detail.streaming[0] : undefined;
  const banner = session ? sessionBanner(session) : null;

  return (
    <>
      {stale !== null && (
        <Banner tone="error" role="alert" title="Can’t reach momoshtrem">
          Showing what it said at {formatTimeOfDay(stale)}. Infuse can still play ready titles if WebDAV is up.
        </Banner>
      )}
      <DetailHeader detail={detail} />
      <div className="td-grid">
        <aside className="td-aside" aria-label="Health and actions">
          <section className={cx('panel panel-pad td-health', `td-health--${health.kind}`)} aria-labelledby="td-health-h">
            <div className="flex items-center gap-2">
              <span className="td-health-ic" aria-hidden="true">
                <HealthIcon size={18} />
              </span>
              <h2 className="t-title-3" id="td-health-h">
                {health.title}
              </h2>
            </div>
            <p>{health.body}</p>
          </section>
          {banner && (
            <section className="banner banner--accent" aria-label="Streaming now">
              <Tv size={20} aria-hidden="true" />
              <div className="grow">
                <strong>{banner.title}</strong>
                {banner.detail && <span>{banner.detail}</span>}
              </div>
            </section>
          )}
          <HoldsPanel detail={detail} />
          <DetailActions
            detail={detail}
            busy={actions.busy(detail)}
            onPause={() => actions.pause(detail)}
            onResume={() => actions.resume(detail)}
            onRemove={() => void actions.remove(detail, { beforeUndo: () => router.push('/activity') })}
          />
        </aside>
        <div className="td-main">
          <FilesPanel detail={detail} readers={readersByPath(detail)} />
          <AdvancedPanel detail={detail} defaultOpen={advanced} />
        </div>
      </div>
    </>
  );
}

/**
 * /activity/{hash} (activity.js `/activity/:hash`): one torrent, built on
 * `GET /torrents/{hash}` alone, which never loads it (never `/files`, which
 * would). Header, health, actions, files, and a collapsed Advanced section.
 */
export function TorrentDetailView({ hash }: { hash: string }) {
  const searchParams = useSearchParams();
  const detail = useQuery(torrentDetailQuery(hash));
  const gone = isApiError(detail.error) && (detail.error.status === 404 || detail.error.status === 400);

  return (
    <div className="page td-page">
      <BackLink />
      {detail.isPending && <DetailSkeleton />}
      {gone && <TorrentGone />}
      {detail.isError && !gone && !detail.data && (
        <Banner
          tone="error"
          role="alert"
          title="Couldn’t load this torrent"
          action={
            <Button size="sm" onClick={() => void detail.refetch()} loading={detail.isFetching}>
              Try again
            </Button>
          }
        >
          momoshtrem didn’t answer. Infuse still plays what’s ready.
        </Banner>
      )}
      {detail.data && !gone && <DetailBody detail={detail.data} advanced={searchParams.get('advanced') === '1'} stale={detail.isError ? detail.dataUpdatedAt : null} />}
    </div>
  );
}
