import type { TorrentListItem, TorrentState } from '@/lib/api/contract';
import { formatNumber } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';
import { TorrentCard } from './TorrentCard';

export const SECTION_COPY: Record<TorrentState, { title: string; sub?: string }> = {
  streaming: { title: 'Streaming now' },
  downloading: { title: 'Downloading' },
  paused: { title: 'Paused' },
  // Torrents fetch on demand: idle ones play the moment Infuse asks (HANDOFF §6)
  idle: { title: 'Idle', sub: 'Ready in Infuse · fetches when you play' },
};

export interface CardHandlers {
  onPause: (torrent: TorrentListItem) => void;
  onResume: (torrent: TorrentListItem) => void;
  onRemove: (torrent: TorrentListItem) => void;
  busy: (torrent: TorrentListItem) => boolean;
}

interface TorrentSectionProps extends CardHandlers {
  state: TorrentState;
  items: readonly TorrentListItem[];
  compact?: boolean;
  /** Replaces the section's line: Downloading says what its downloads still want (#77). */
  sub?: string;
}

/** One of Activity's sections (activity.js `group`); hidden when empty. */
export function TorrentSection({ state, items, compact, sub: line, busy, ...handlers }: TorrentSectionProps) {
  if (items.length === 0) return null;
  const { title } = SECTION_COPY[state];
  const sub = line ?? SECTION_COPY[state].sub;
  const headingId = `act-${state}-h`;
  return (
    <section className={cx('act-group', `act-group--${state}`)} aria-labelledby={headingId}>
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-3" id={headingId}>
            {title} <span className="count">{formatNumber(items.length)}</span>
          </h2>
          {sub && <p className="t-small text-fg-3">{sub}</p>}
        </div>
      </div>
      <div className="act-list">
        {items.map((torrent) => (
          <TorrentCard key={torrent.info_hash} torrent={torrent} compact={compact} busy={busy(torrent)} {...handlers} />
        ))}
      </div>
    </section>
  );
}
