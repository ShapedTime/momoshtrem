import Link from 'next/link';
import { ArrowDown, ArrowUp, HardDrive, Pause, Play, Trash2, TriangleAlert, Tv, Users } from 'lucide-react';
import { Button, ButtonLink, IconButton, Media, Progress, ReleaseName, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { TorrentListItem } from '@/lib/api/contract';
import { formatRate, sessionBanner, torrentLabel } from '@/lib/activity/format';
import { isWeak } from '@/lib/activity/health';
import { formatBytes, plural } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

/** The state pill (core.js `tcard`): Streaming now, a ring with %, Paused, Complete (only when it is), or Idle. */
export function TorrentStatePill({ torrent, size = 'md' }: { torrent: TorrentListItem; size?: 'md' | 'lg' }) {
  const pct = torrent.progress !== null ? Math.floor(torrent.progress * 100) : null;
  switch (torrent.state) {
    case 'streaming':
      return <StatusPill status="streaming" label="Streaming now" size={size} />;
    case 'downloading':
      return pct !== null ? (
        <StatusPill status="downloading" label={size === 'lg' ? `Downloading ${pct}%` : `${pct}%`} pct={pct} size={size} />
      ) : (
        <StatusPill status="downloading" size={size} />
      );
    case 'paused':
      return <StatusPill status="paused" size={size} />;
    default:
      return torrent.complete ? (
        <StatusPill status="complete" label={size === 'lg' ? 'Complete · ready in Infuse' : undefined} size={size} />
      ) : (
        <StatusPill status="idle" size={size} />
      );
  }
}

interface TorrentCardProps {
  torrent: TorrentListItem;
  onPause: (torrent: TorrentListItem) => void;
  onResume: (torrent: TorrentListItem) => void;
  onRemove: (torrent: TorrentListItem) => void;
  /** A pause or resume is on its way. */
  busy?: boolean;
  /** The compact side column (Idle on desktop). */
  compact?: boolean;
}

/**
 * One torrent on Activity (core.js `ui.tcard`, HANDOFF §3 TorrentCard): poster
 * and label linking to its title, the release name, a state pill, progress,
 * speeds, peers and size, a few-peers warning, the streaming session, and
 * Pause/Resume, Details and Remove. Not the Jackett result card in components/torrent.
 */
export function TorrentCard({ torrent, onPause, onResume, onRemove, busy, compact }: TorrentCardProps) {
  const label = torrentLabel(torrent);
  const title = torrent.titles[0];
  const streaming = torrent.state === 'streaming';
  const active = streaming || torrent.state === 'downloading';
  const session = streaming ? torrent.streaming[0] : undefined;
  const banner = session ? sessionBanner(session) : null;
  const pct = torrent.progress !== null ? torrent.progress * 100 : null;
  const detailHref = `/activity/${torrent.info_hash}`;

  return (
    <article className={cx('tcard', streaming && 'is-streaming')} aria-label={label.text}>
      {label.href ? (
        <Link href={label.href} className="poster-link" aria-hidden="true" tabIndex={-1}>
          <Media shape="poster" src={buildImageUrl(title?.poster_path ?? null, 'poster', 'small')} sizes={compact ? '44px' : '56px'} />
        </Link>
      ) : (
        <Media shape="poster" className="poster-link" />
      )}
      <div className="flex min-w-0 flex-col gap-2">
        <div className="top">
          <div className="flex min-w-0 flex-col">
            {label.href ? (
              <Link href={label.href} className="label">
                {label.text}
              </Link>
            ) : (
              <span className="t-small font-medium text-fg-2">Not linked to your library</span>
            )}
            <ReleaseName name={torrent.name} className="clamp-2" />
          </div>
          <TorrentStatePill torrent={torrent} />
        </div>
        {pct !== null && pct < 100 && <Progress value={pct} label={`${label.text} progress`} />}
        <div className="stats">
          {active && (
            <>
              <span>
                <ArrowDown size={14} aria-hidden="true" />
                <span className="sr-only">Download </span>
                {formatRate(torrent.download_bps)}
              </span>
              <span>
                <ArrowUp size={14} aria-hidden="true" />
                <span className="sr-only">Upload </span>
                {formatRate(torrent.upload_bps)}
              </span>
            </>
          )}
          {active && torrent.peers !== null && (
            <span>
              <Users size={14} aria-hidden="true" />
              {plural(torrent.peers, 'peer')}
            </span>
          )}
          {torrent.size_bytes !== null && (
            <span>
              <HardDrive size={14} aria-hidden="true" />
              {formatBytes(torrent.size_bytes)}
            </span>
          )}
          {isWeak(torrent) && (
            <span className="warn">
              <TriangleAlert size={14} aria-hidden="true" />
              Few peers
            </span>
          )}
        </div>
      </div>
      {banner && (
        <div className="full banner banner--accent session">
          <Tv size={18} aria-hidden="true" />
          <div className="grow">
            <strong>{banner.title}</strong>
            {banner.detail && <span>{banner.detail}</span>}
          </div>
        </div>
      )}
      <div className="full actions">
        {torrent.state === 'paused' ? (
          <Button size="sm" onClick={() => onResume(torrent)} loading={busy} aria-label={`Resume ${label.text}`}>
            <Play size={15} aria-hidden="true" /> Resume
          </Button>
        ) : active ? (
          <Button size="sm" onClick={() => onPause(torrent)} loading={busy} aria-label={`Pause ${label.text}`}>
            <Pause size={15} aria-hidden="true" /> Pause
          </Button>
        ) : null}
        <ButtonLink href={detailHref} variant="ghost" size="sm" aria-label={`Details: ${label.text}`}>
          Details
        </ButtonLink>
        <IconButton size="sm" className="end" aria-label={`Remove ${label.text} torrent`} onClick={() => onRemove(torrent)}>
          <Trash2 size={16} aria-hidden="true" />
        </IconButton>
      </div>
    </article>
  );
}
