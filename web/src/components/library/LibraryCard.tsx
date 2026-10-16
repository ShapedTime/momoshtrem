import { PosterCard, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { LibraryItem } from '@/lib/api/contract';
import { posterBadge, watchedPct, watchLine } from '@/lib/library/display';
import { titleHref } from '@/lib/title/href';
import { cx } from '@/lib/utils/cx';
import { LibraryItemMenu } from './LibraryItemMenu';
import { LibraryStatusLine } from './LibraryStatusLine';

interface LibraryCardProps {
  item: LibraryItem;
  onRemove: (item: LibraryItem) => void;
  /** Trakt is linked, so the watch line means something (a show's would read "Not started" otherwise). */
  trakt: boolean;
}

/**
 * A library title in the grid (library.js `card`): the pill only when
 * something wants attention, the "…" menu, the watched bar, and the status
 * and watch lines. Everything is visible without hover.
 */
export function LibraryCard({ item, onRemove, trakt }: LibraryCardProps) {
  const badge = posterBadge(item);
  const watched = watchedPct(item);
  const sub = watchLine(item, { trakt });
  return (
    <PosterCard
      href={titleHref(item.media_type, item.tmdb_id)}
      title={item.title}
      posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
      className={cx('libcard', item.needs_review > 0 && 'has-review')}
      badge={badge ? <StatusPill status={badge.status} label={badge.label} pct={badge.pct} onArt /> : undefined}
      watchedPct={watched > 0 ? watched : undefined}
      action={<LibraryItemMenu item={item} onRemove={onRemove} variant="card" />}
      lines={
        <>
          <p className="status">
            <LibraryStatusLine item={item} />
          </p>
          {sub && <p className="sub">{sub}</p>}
        </>
      }
    />
  );
}
