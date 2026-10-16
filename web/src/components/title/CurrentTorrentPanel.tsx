import { Play, RefreshCw } from 'lucide-react';
import { ReplaceMenu } from '@/components/source/ReplaceMenu';
import { Button, ButtonLink, Chip, ReleaseName, StatusPill } from '@/components/ui';
import type { TitleTorrent } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';
import { formatBytes, formatNumber, formatShortDate } from '@/lib/title/format';
import { releaseTags } from '@/lib/title/release';
import { cx } from '@/lib/utils/cx';

/** Paused (B6) wins; loaded with seeders: healthy; loaded without: say so; otherwise it loads on the first read. */
function health(torrent: TitleTorrent): StatusKey {
  if (torrent.state === 'paused') return 'paused';
  if (torrent.state !== 'active') return 'idle';
  return torrent.seeders ? 'healthy' : 'no_seeders';
}

interface CurrentTorrentPanelProps {
  torrent: TitleTorrent;
  /** Opens torrent search to pick another release. */
  onReplace: () => void;
  /** With HDRezka set up, Replace becomes a menu: a torrent, or a stream (#63). */
  onStreamFromHDRezka?: () => void;
  /** Resumes a paused torrent (offered only while `torrent.state` is `paused`). */
  onResume?: () => void;
  resuming?: boolean;
}

/** The release Infuse plays from, once a title is ready: name, attributes, facts, Replace. */
export function CurrentTorrentPanel({ torrent, onReplace, onStreamFromHDRezka, onResume, resuming }: CurrentTorrentPanelProps) {
  const source = torrent.source?.toLowerCase() ?? '';
  const chips = [
    torrent.resolution && { label: torrent.resolution, hi: true },
    torrent.source && { label: torrent.source, hi: false },
    // "Remux" is already in a "BluRay REMUX" source
    ...releaseTags(torrent.release_name).filter((tag) => !source.includes(tag.label.toLowerCase())),
  ].filter((chip): chip is { label: string; hi: boolean } => Boolean(chip));
  // Its page on Activity reads what's stored when it isn't loaded, and never loads it
  const statsHref = `/activity/${torrent.info_hash}`;

  return (
    <section className="panel panel-pad flex flex-col gap-3" aria-labelledby="torrent-h">
      <div className="flex items-center justify-between gap-2">
        <h2 className="t-title-3" id="torrent-h">
          Current torrent
        </h2>
        <StatusPill status={health(torrent)} />
      </div>
      <ReleaseName name={torrent.release_name} />
      {chips.length > 0 && (
        <div className="chips">
          {chips.map((chip) => (
            <Chip key={chip.label} tone={chip.hi ? 'hi' : 'default'}>
              {chip.label}
            </Chip>
          ))}
        </div>
      )}
      <dl className="facts-grid">
        <div>
          <dt className="t-micro text-fg-3">Size</dt>
          <dd>{formatBytes(torrent.size_bytes)}</dd>
        </div>
        <div>
          <dt className="t-micro text-fg-3">Seeders</dt>
          <dd className={cx(torrent.seeders ? 'text-ready' : 'text-fg-2')}>
            {torrent.seeders === null ? (
              <>
                <span aria-hidden="true">—</span>
                <span className="sr-only">Unknown until the torrent loads</span>
              </>
            ) : (
              formatNumber(torrent.seeders)
            )}
          </dd>
        </div>
        <div>
          <dt className="t-micro text-fg-3">Added</dt>
          <dd>{formatShortDate(torrent.added_at)}</dd>
        </div>
      </dl>
      <div className="flex flex-wrap gap-2">
        {torrent.state === 'paused' && onResume && (
          <Button size="sm" onClick={onResume} loading={resuming}>
            <Play size={15} aria-hidden="true" /> Resume
          </Button>
        )}
        {onStreamFromHDRezka ? (
          <ReplaceMenu onFindTorrent={onReplace} onStreamFromHDRezka={onStreamFromHDRezka} />
        ) : (
          <Button size="sm" onClick={onReplace}>
            <RefreshCw size={15} aria-hidden="true" /> Replace
          </Button>
        )}
        <ButtonLink href={statsHref} variant="ghost" size="sm">
          Files &amp; stats
        </ButtonLink>
      </div>
    </section>
  );
}
