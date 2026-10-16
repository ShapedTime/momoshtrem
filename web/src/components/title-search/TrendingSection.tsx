'use client';

import { useQuery } from '@tanstack/react-query';
import { libraryBadge } from '@/components/library/LibraryPill';
import { Banner, Button, LoadingRegion, PosterCard, PosterGrid, PosterSkeletons } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useLibraryLookup } from '@/hooks/useLibraryLookup';
import { trendingTitlesQuery } from '@/lib/api/endpoints';
import { titleHref } from '@/lib/title/href';
import { cx } from '@/lib/utils/cx';

interface TrendingSectionProps {
  title: string;
  subtitle?: string;
  /** How many titles to show. */
  count: number;
  headingId: string;
  className?: string;
}

/**
 * This week's trending titles on TMDB, with library pills: "Trending this
 * week" when idle, "Popular right now" when a search finds nothing.
 */
export function TrendingSection({ title, subtitle, count, headingId, className }: TrendingSectionProps) {
  const query = useQuery(trendingTitlesQuery());
  const titles = (query.data ?? []).slice(0, count);
  const library = useLibraryLookup(titles);

  return (
    <section className={cx('section', className)} aria-labelledby={headingId}>
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-3" id={headingId}>
            {title}
          </h2>
          {subtitle && <p className="t-small text-fg-3">{subtitle}</p>}
        </div>
      </div>
      {query.isPending && (
        <LoadingRegion label="Loading trending titles">
          <PosterGrid>
            <PosterSkeletons count={count} />
          </PosterGrid>
        </LoadingRegion>
      )}
      {query.isError && (
        <Banner
          tone="error"
          title="Couldn’t load trending titles"
          action={
            <Button size="sm" onClick={() => void query.refetch()} loading={query.isRefetching}>
              Try again
            </Button>
          }
        >
          TMDB didn’t answer. Search still works.
        </Banner>
      )}
      {titles.length > 0 && (
        <PosterGrid>
          {titles.map((item) => (
            <PosterCard
              key={`${item.media_type}:${item.tmdb_id}`}
              href={titleHref(item.media_type, item.tmdb_id)}
              title={item.title}
              subtitle={[item.year, item.media_type === 'tv' ? 'Series' : 'Movie'].filter(Boolean).join(' · ')}
              posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
              badge={libraryBadge(library.get(item.media_type, item.tmdb_id))}
            />
          ))}
        </PosterGrid>
      )}
    </section>
  );
}
