'use client';

import Link from 'next/link';
import type { CSSProperties } from 'react';
import { Plus, Search, Sparkle, Star } from 'lucide-react';
import { FeedbackRow, useRecFeedback } from '@/components/recommendations';
import { Button, ButtonLink, Media, Skeleton, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { AskItem, AvailabilityStage } from '@/lib/api/contract';
import { askItemFacts, askRating } from '@/lib/ask/copy';
import type { StatusKey } from '@/lib/status';
import { titleHref } from '@/lib/title/href';
import { cx } from '@/lib/utils/cx';
import type { AskLibrary, AskLibraryState } from './useAskLibrary';

/** The library pill on a card (as LibraryPill, off the artwork). */
const STAGE_PILL: Record<AvailabilityStage, { status: StatusKey; label?: string }> = {
  none: { status: 'added', label: 'In library' },
  needs_torrent: { status: 'needs_torrent', label: 'No torrent' },
  downloading: { status: 'downloading' },
  ready: { status: 'ready' },
};

/** The card's main action by library state (ask.js `cardActions`). */
function PrimaryAction({ item, state, library }: { item: AskItem; state: AskLibraryState; library: AskLibrary }) {
  if (state.kind === 'added') {
    return (
      <span className="ask-inlib">
        <StatusPill status="added" label="In library" />
        <Button size="sm" onClick={() => library.findTorrent(item)}>
          <Search size={15} aria-hidden="true" />
          Find a torrent
        </Button>
      </span>
    );
  }
  if (state.kind === 'in') {
    const pill = STAGE_PILL[state.stage];
    return (
      <span className="ask-inlib">
        <StatusPill status={pill.status} label={pill.label} />
        <ButtonLink size="sm" href={titleHref(item.media_type, item.tmdb_id)}>
          Open
        </ButtonLink>
      </span>
    );
  }
  return (
    <Button variant="primary" size="sm" loading={library.adding(item)} onClick={() => library.add(item)}>
      <Plus size={16} aria-hidden="true" />
      Add to library
    </Button>
  );
}

interface AskCardProps {
  item: AskItem;
  index: number;
  library: AskLibrary;
  /** Arrived while you watched: it rises in. */
  isNew?: boolean;
}

/**
 * One pick (ask.js `card`): poster, match tag, title, facts, the reason,
 * what you can do with it, and feedback (thumbs and "…", #155's). Negative
 * feedback dims it at once, with Undo; the next read of the thread leaves it out.
 */
export function AskCard({ item, index, library, isNew }: AskCardProps) {
  const feedback = useRecFeedback(item, 'ask');
  const state = library.stateOf(item);
  const href = titleHref(item.media_type, item.tmdb_id);
  const rating = askRating(item.vote_average);
  return (
    <article className={cx('ask-card', isNew && 'is-new', feedback.dismissed && 'is-dimmed')} style={{ '--i': index } as CSSProperties}>
      <Link className="ask-poster" href={href} tabIndex={-1} aria-hidden="true">
        <Media shape="poster" src={buildImageUrl(item.poster_path, 'poster', 'small')} sizes="(min-width: 1024px) 112px, 84px" />
      </Link>
      <div className="ask-card-body">
        <span className="ask-match">
          <Sparkle size={13} aria-hidden="true" />
          <span>{item.match_tag}</span>
        </span>
        <h3 className="t-title-3">
          <Link href={href}>{item.title}</Link>
        </h3>
        <p className="ask-meta">
          {askItemFacts(item).map((fact) => (
            <span key={fact}>{fact}</span>
          ))}
          {rating && (
            <span className="star">
              <Star size={12} fill="currentColor" aria-hidden="true" />
              {rating}
              <span className="sr-only"> on TMDB</span>
            </span>
          )}
        </p>
        <p className="ask-reason">{item.reason}</p>
        <div className="ask-actions">
          <PrimaryAction item={item} state={state} library={library} />
          <ButtonLink variant="ghost" size="sm" href={href}>
            Details
          </ButtonLink>
          <FeedbackRow feedback={feedback} inLibrary={state.kind !== 'none'} className="ask-fb" />
        </div>
      </div>
    </article>
  );
}

/** A card still coming: the size of a real one. */
export function AskCardSkeleton() {
  return (
    <div className="ask-card is-skel" aria-hidden="true">
      <Skeleton shape="poster" className="skel-poster" />
      <div className="skel-lines">
        <Skeleton shape="line" width="34%" />
        <Skeleton shape="line" width="62%" height={18} />
        <Skeleton shape="line" width="90%" />
        <Skeleton shape="line" width="76%" />
      </div>
    </div>
  );
}
