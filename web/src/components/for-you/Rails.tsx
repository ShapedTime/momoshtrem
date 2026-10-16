'use client';

import { type ReactNode, useId } from 'react';
import { libraryBadge } from '@/components/library/LibraryPill';
import { Banner, Button, LoadingRegion, PosterCard, PosterSkeletons, Rail, RailItem, WideSkeletons } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useLibraryLookup } from '@/hooks/useLibraryLookup';
import type { MediaType } from '@/lib/api/contract';
import type { EpisodeCardModel } from '@/lib/for-you/episode-card';
import { titleHref } from '@/lib/title/href';
import { cx } from '@/lib/utils/cx';
import { ForYouEpisodeCard } from './ForYouEpisodeCard';

/** What a query gives a rail: its items once loaded, and how to try again. */
export interface RailQuery<T> {
  items: T[] | undefined;
  pending: boolean;
  failed: boolean;
  retry: () => void;
  retrying: boolean;
}

/** The parts of a TanStack query result a rail reads. */
interface QueryLike<TData> {
  data: TData | undefined;
  isPending: boolean;
  isError: boolean;
  isRefetching: boolean;
  refetch: () => unknown;
}

/** A query as a rail's items; `enabled: false` reads as loaded and empty. */
export function railQuery<TData, T>(query: QueryLike<TData>, select: (data: TData) => T[], enabled = true): RailQuery<T> {
  return {
    items: !enabled ? [] : query.data === undefined ? undefined : select(query.data),
    pending: enabled && query.isPending,
    failed: enabled && query.isError,
    retry: () => void query.refetch(),
    retrying: query.isRefetching,
  };
}

/**
 * A rail's heading over something that isn't the rail yet: skeletons the
 * size of its cards, a failure, or a note. (A <Rail> is a list, so these
 * can't go inside one.)
 */
export function RailFrame({ title, subtitle, children, className }: { title: string; subtitle?: string; children: ReactNode; className?: string }) {
  const headingId = useId();
  return (
    <section className={cx('section rail-wrap', className)} aria-labelledby={headingId}>
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-2" id={headingId}>
            {title}
          </h2>
          {subtitle && <p className="t-small text-fg-3">{subtitle}</p>}
        </div>
      </div>
      {children}
    </section>
  );
}

/** A rail that couldn't load: what failed, that the rest is fine, and Try again. */
export function RailError({ title, children, onRetry, retrying }: { title: string; children: ReactNode; onRetry: () => void; retrying: boolean }) {
  return (
    <Banner
      tone="error"
      title={title}
      action={
        <Button size="sm" onClick={onRetry} loading={retrying}>
          Try again
        </Button>
      }
    >
      {children}
    </Banner>
  );
}

interface EpisodeRailProps {
  title: string;
  subtitle?: string;
  query: RailQuery<EpisodeCardModel>;
  /** "Couldn’t load Up next". */
  errorTitle: string;
  /** What shows when it loads empty; nothing at all without one. */
  empty?: ReactNode;
  /** Skeletons stay while this is true and nothing has come in yet (the first sync). */
  filling?: boolean;
}

/** Up next and New episodes: landscape cards, each with its availability's action. */
export function EpisodeRail({ title, subtitle, query, errorTitle, empty, filling }: EpisodeRailProps) {
  const { items } = query;
  if (items && items.length > 0) {
    return (
      <Rail title={title} subtitle={subtitle} kind="wide">
        {items.map((card) => (
          <RailItem key={card.key}>
            <ForYouEpisodeCard card={card} />
          </RailItem>
        ))}
      </Rail>
    );
  }
  if (query.failed) {
    return (
      <RailFrame title={title} subtitle={subtitle}>
        <RailError title={errorTitle} onRetry={query.retry} retrying={query.retrying}>
          momoshtrem didn’t answer. The rest of the page still works.
        </RailError>
      </RailFrame>
    );
  }
  if (query.pending || filling) {
    return (
      <RailFrame title={title} subtitle={subtitle}>
        <LoadingRegion label={`Loading ${title}`} className="rail rail--wide overflow-hidden">
          <WideSkeletons count={4} />
        </LoadingRegion>
      </RailFrame>
    );
  }
  return empty ? (
    <RailFrame title={title} subtitle={subtitle}>
      {empty}
    </RailFrame>
  ) : null;
}

/** A title on a poster rail (trending, Discover, the watchlist). */
export interface PosterTitle {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
}

const posterSubtitle = (item: PosterTitle) => [item.year, item.media_type === 'tv' ? 'Series' : 'Movie'].filter(Boolean).join(' · ');

interface TitleRailProps {
  title: string;
  subtitle?: string;
  query: RailQuery<PosterTitle>;
  errorTitle: string;
  errorDetail?: string;
  /** Library pills on the posters (off for the first-time page: the library is empty). */
  library?: boolean;
  /** At most this many posters. */
  limit?: number;
}

/** A poster rail of TMDB titles, with where each stands in your library. Hidden when it loads empty. */
export function TitleRail({ title, subtitle, query, errorTitle, errorDetail, library = true, limit = 20 }: TitleRailProps) {
  const items = (query.items ?? []).slice(0, limit);
  const lookup = useLibraryLookup(library ? items : []);

  if (items.length > 0) {
    return (
      <Rail title={title} subtitle={subtitle}>
        {items.map((item) => (
          <RailItem key={`${item.media_type}:${item.tmdb_id}`}>
            <PosterCard
              href={titleHref(item.media_type, item.tmdb_id)}
              title={item.title}
              subtitle={posterSubtitle(item)}
              posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
              badge={library ? libraryBadge(lookup.get(item.media_type, item.tmdb_id)) : undefined}
            />
          </RailItem>
        ))}
      </Rail>
    );
  }
  if (query.failed) {
    return (
      <RailFrame title={title} subtitle={subtitle}>
        <RailError title={errorTitle} onRetry={query.retry} retrying={query.retrying}>
          {errorDetail ?? 'TMDB didn’t answer. The rest of the page still works.'}
        </RailError>
      </RailFrame>
    );
  }
  if (query.pending) {
    return (
      <RailFrame title={title} subtitle={subtitle}>
        <LoadingRegion label={`Loading ${title}`} className="rail rail--posters overflow-hidden">
          <PosterSkeletons count={8} />
        </LoadingRegion>
      </RailFrame>
    );
  }
  return null;
}
