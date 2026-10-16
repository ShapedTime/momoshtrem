import Link from 'next/link';
import { ArrowDown, ArrowUp, ChevronRight, HardDrive, Layers, Tv } from 'lucide-react';
import type { StreamingSession, TorrentListItem, TorrentsSummary, TorrentState } from '@/lib/api/contract';
import { nowTile, splitBytes, splitSpeed } from '@/lib/activity/format';
import { formatBytes, formatNumber, plural } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';
import { Sparkline } from './Sparkline';

/** "Streaming now": what plays, where, how far in and how far ahead; quiet when nothing does. */
function NowTile({ session }: { session: StreamingSession | null }) {
  if (!session) {
    return (
      <div className="act-now is-quiet">
        <span className="ic" aria-hidden="true">
          <Tv size={22} />
        </span>
        <span className="grow">
          <span className="kicker">Streaming now</span>
          <strong>Nothing playing</strong>
          <span className="detail">Infuse shows up here when it opens a file</span>
        </span>
      </div>
    );
  }
  const tile = nowTile(session);
  const body = (
    <>
      <span className="ic" aria-hidden="true">
        <Tv size={22} />
      </span>
      <span className="grow">
        <span className="kicker">Streaming now</span>
        <strong>{tile.title}</strong>
        {tile.detail && <span className="detail">{tile.detail}</span>}
      </span>
    </>
  );
  return tile.href ? (
    <Link href={tile.href} className="act-now">
      {body}
      <ChevronRight size={18} className="chev" aria-hidden="true" />
    </Link>
  ) : (
    <div className="act-now">{body}</div>
  );
}

function Value({ value, unit }: { value: string; unit: string }) {
  return (
    <span className="val">
      <b>{value}</b>
      {unit}
    </span>
  );
}

interface SummaryStripProps {
  summary: TorrentsSummary | undefined;
  /** The visible torrents (pending removals left out), so counts follow a pause at once. */
  items: readonly TorrentListItem[];
  now: StreamingSession | null;
  /** Offline: the last known numbers, greyed. */
  stale?: boolean;
}

/**
 * The summary strip (activity.js `summary`): Streaming now, then Download with
 * its sparkline, Upload, Torrents and Disk. Phone: Streaming now full width
 * over a 2×2 grid; 640+: four columns under it; 1024+: one row.
 */
export function SummaryStrip({ summary, items, now, stale }: SummaryStripProps) {
  const counts = items.reduce<Record<TorrentState, number>>((sum, item) => ({ ...sum, [item.state]: sum[item.state] + 1 }), {
    streaming: 0,
    downloading: 0,
    paused: 0,
    idle: 0,
  });
  const down = summary?.download_bps ?? 0;
  const up = summary?.upload_bps ?? 0;
  const history = summary?.history.download_bps ?? [];
  const disk = summary?.disk ?? null;
  const other = disk ? Math.max(0, disk.total_bytes - disk.free_bytes - disk.torrents_bytes) : 0;
  const share = (bytes: number) => (disk && disk.total_bytes > 0 ? `${((bytes / disk.total_bytes) * 100).toFixed(1)}%` : '0%');
  const free = disk ? splitBytes(disk.free_bytes) : null;

  return (
    <section className={cx('act-summary', stale && 'is-stale')} aria-label="Summary">
      <NowTile session={now} />
      <div className="act-tile">
        <span className="label">
          <ArrowDown size={15} aria-hidden="true" />
          Download
        </span>
        <Value {...splitSpeed(down)} />
        <Sparkline samples={history} intervalSeconds={summary?.history.interval_seconds ?? 2} flat={stale || history.every((value) => value === 0)} />
      </div>
      <div className="act-tile">
        <span className="label">
          <ArrowUp size={15} aria-hidden="true" />
          Upload
        </span>
        <Value {...splitSpeed(up)} />
        <span className="sub">{up > 0 && summary ? `To ${plural(summary.peers, 'peer')}` : 'Nothing seeding'}</span>
      </div>
      <div className="act-tile">
        <span className="label">
          <Layers size={15} aria-hidden="true" />
          Torrents
        </span>
        <Value value={formatNumber(counts.streaming + counts.downloading)} unit="active" />
        <span className="sub">
          {formatNumber(counts.paused)} paused · {formatNumber(counts.idle)} idle
        </span>
      </div>
      <div className="act-tile">
        <span className="label">
          <HardDrive size={15} aria-hidden="true" />
          Disk
        </span>
        {disk && free ? (
          <>
            <Value value={free.value} unit={`${free.unit} free`} />
            <span
              className="act-disk"
              role="img"
              aria-label={`Disk: ${formatBytes(disk.torrents_bytes)} in torrents, ${formatBytes(other)} other, ${formatBytes(disk.free_bytes)} free of ${formatBytes(disk.total_bytes)}`}
            >
              <i style={{ width: share(disk.torrents_bytes) }} />
              <i style={{ width: share(other) }} />
            </span>
            <span className="sub">
              {formatBytes(disk.torrents_bytes)} torrents · {formatBytes(disk.total_bytes)} disk
            </span>
          </>
        ) : (
          <>
            <Value value="—" unit="" />
            <span className="sub">Disk use unknown</span>
          </>
        )}
      </div>
    </section>
  );
}
