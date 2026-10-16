import Link from 'next/link';
import { Media, Progress, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { LibraryItem } from '@/lib/api/contract';
import { kindLine, lastWatchedLabel, librarySize, watchedPct, watchLine } from '@/lib/library/display';
import { titleHref } from '@/lib/title/href';
import { LibraryItemMenu } from './LibraryItemMenu';
import { LibraryStatusLine } from './LibraryStatusLine';

/** The list view's column heads (desktop only; phone rows stack). */
export function LibraryListHead() {
  return (
    <div className="lrows-head" aria-hidden="true">
      <span />
      <span>Title</span>
      <span>Status</span>
      <span>Watched</span>
      <span>Size</span>
      <span>Last watched</span>
      <span />
    </div>
  );
}

interface LibraryRowProps {
  item: LibraryItem;
  onRemove: (item: LibraryItem) => void;
  /** Trakt is linked: without it nothing says what was watched, so Watched and Last watched stay blank. */
  trakt: boolean;
}

/**
 * A library title in the list (library.js `listRow`): thumbnail, title and
 * kind, status with its pills, watched, size, last watched and ⋮.
 */
export function LibraryRow({ item, onRemove, trakt }: LibraryRowProps) {
  const href = titleHref(item.media_type, item.tmdb_id);
  const watched = watchedPct(item);
  const watch = watchLine(item, { trakt });
  const last = lastWatchedLabel(item);
  const showDownload = item.media_type === 'tv' && item.download !== null && !item.streaming;
  const pills = item.needs_review > 0 || item.streaming || showDownload;
  return (
    <li className="lrow">
      <Link href={href} className="lrow-thumb" aria-hidden="true" tabIndex={-1}>
        <Media shape="poster" src={buildImageUrl(item.poster_path, 'poster', 'small')} sizes="44px" />
      </Link>
      <div className="lrow-title">
        <Link href={href} className="name">
          {item.title}
        </Link>
        <span className="sub">
          {kindLine(item)}
          {item.size_bytes > 0 && <span className="xs-inline"> · {librarySize(item.size_bytes)}</span>}
        </span>
      </div>
      <div className="lrow-status">
        <LibraryStatusLine item={item} />
        {pills && (
          <span className="lrow-pills">
            {item.needs_review > 0 && <StatusPill status="needs_review" label={`${item.needs_review} to review`} />}
            {item.streaming && <StatusPill status="streaming" />}
            {showDownload && item.download && <StatusPill status="downloading" label={`${item.download.pct}%`} pct={item.download.pct} />}
          </span>
        )}
      </div>
      <div className="lrow-watch">
        {watch && <span className="t-small text-fg-2">{watch}</span>}
        {watched > 0 && <Progress value={watched} tone="watched" label={`${item.title}: ${watched}% watched`} />}
      </div>
      <span className="lrow-size">{librarySize(item.size_bytes)}</span>
      <span className="lrow-last">
        {last ?? (
          <span className="text-fg-3" aria-hidden={!trakt || undefined}>
            {trakt ? 'Never' : '—'}
          </span>
        )}
      </span>
      <LibraryItemMenu item={item} onRemove={onRemove} variant="row" />
    </li>
  );
}
