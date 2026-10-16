'use client';

import { useEffect, useMemo, useRef } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Link2 } from 'lucide-react';
import { CatEyes } from '@/components/brand';
import { libraryPill } from '@/components/library/LibraryPill';
import { Banner, Button, LoadingRegion, PosterCard, PosterGrid, PosterSkeletons, Select, StatusPill, copyText, useToast } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useLibraryLookup } from '@/hooks/useLibraryLookup';
import { isApiError } from '@/lib/api/client';
import type { DiscoverItem, DiscoverPage, LibraryItem } from '@/lib/api/contract';
import { discoverPagesQuery } from '@/lib/api/endpoints';
import { activeFilters, clearFilters, countLabel, DISCOVER_SORTS, type DiscoverFilters, type DiscoverSort, SORT_LABELS, withoutFilter } from '@/lib/discover/filters';
import { selectedMood } from '@/lib/discover/moods';
import { posterBadge } from '@/lib/library/display';
import { discoverHref } from '@/lib/discover/url';
import { plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';
import { DiscoverActiveFilters } from './DiscoverChips';
import { DiscoverEmpty } from './DiscoverEmpty';

interface DiscoverResultsProps {
  filters: DiscoverFilters;
  onChange: (next: DiscoverFilters) => void;
}

/**
 * The results column: the count line (count, Copy link on desktop, sort),
 * the active filters, then the poster grid, which loads the next page as
 * its footer scrolls near (discover.js `results`).
 */
export function DiscoverResults({ filters, onChange }: DiscoverResultsProps) {
  const query = useInfiniteQuery(discoverPagesQuery(filters));
  const active = activeFilters(filters);
  const mood = selectedMood(filters);
  const first = query.data?.pages[0];
  const pages = useMemo(() => uniquePages(query.data?.pages ?? []), [query.data]);
  const empty = first !== undefined && pages.every((items) => items.length === 0);

  const heading = first ? countLabel(filters.type, first.total) : query.isPending ? 'Finding titles…' : 'Nothing to show';
  const source = mood ? `${mood.label} picks from TMDB` : 'From TMDB, filtered live';

  return (
    <>
      <div className="disc-rhead">
        <div className="titles">
          <h2 className="t-title-2" id="disc-count">
            {heading}
          </h2>
          <p className="t-small text-fg-3">
            {source}
            {active.length > 0 && ` · ${plural(active.length, 'filter')}`}
          </p>
        </div>
        <div className="disc-rtools">
          <CopyLinkButton filters={filters} />
          <Select aria-label="Sort by" className="disc-sort" value={filters.sort} onChange={(event) => onChange({ ...filters, sort: toSort(event.target.value) })}>
            {DISCOVER_SORTS.map((sort) => (
              <option key={sort} value={sort}>
                {SORT_LABELS[sort]}
              </option>
            ))}
          </Select>
        </div>
      </div>
      {first && (
        <p className="sr-only" role="status">
          {heading}
        </p>
      )}

      {!empty && (
        <DiscoverActiveFilters active={active} onRemove={(filter) => onChange(withoutFilter(filters, filter.key))} onClearAll={() => onChange(clearFilters(filters))} />
      )}

      {query.isPending && (
        <LoadingRegion label="Finding titles">
          <PosterGrid className="disc-grid">
            <PosterSkeletons count={18} />
          </PosterGrid>
        </LoadingRegion>
      )}

      {!first && query.isError && <DiscoverError error={query.error} onRetry={() => void query.refetch()} retrying={query.isRefetching} />}

      {empty && <DiscoverEmpty filters={filters} onChange={onChange} />}

      {first && !empty && (
        <>
          <PosterGrid className="disc-grid">
            {pages.map((items, index) => (
              <DiscoverCards key={index} items={items} />
            ))}
          </PosterGrid>
          {query.isFetchNextPageError ? (
            <DiscoverError error={query.error} onRetry={() => void query.fetchNextPage()} retrying={query.isFetchingNextPage} more />
          ) : (
            query.hasNextPage && <LoadMore pages={pages.length} onLoadMore={() => !query.isFetching && void query.fetchNextPage()} />
          )}
        </>
      )}
    </>
  );
}

const toSort = (value: string): DiscoverSort => DISCOVER_SORTS.find((sort) => sort === value) ?? 'popular';

/**
 * Each title once: TMDB's pages can repeat a title when popularity moves
 * between requests, and a card key must be unique.
 */
function uniquePages(pages: DiscoverPage[]): DiscoverItem[][] {
  const seen = new Set<string>();
  return pages.map((page) =>
    page.items.filter((item) => {
      const key = `${item.media_type}:${item.tmdb_id}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
  );
}

/** One page of cards, with one `/library?ids=` lookup for the page's library pills. */
function DiscoverCards({ items }: { items: DiscoverItem[] }) {
  const library = useLibraryLookup(items);
  return (
    <>
      {items.map((item) => (
        <PosterCard
          key={`${item.media_type}:${item.tmdb_id}`}
          href={titleHref(item.media_type, item.tmdb_id)}
          title={item.title}
          // TMDB discover has no runtimes, so the type stands in (a deviation from the design)
          subtitle={[item.year, item.media_type === 'tv' ? 'Series' : 'Movie'].filter(Boolean).join(' · ')}
          posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
          badge={libraryBadge(library.get(item.media_type, item.tmdb_id))}
        />
      ))}
    </>
  );
}

/**
 * Where a title stands in your library, as the Library's own cards say it
 * (Review, Playing, 42%, No torrent), else Ready. Short words fit the
 * phone's 104 px cards, where "Downloading" would run off the poster.
 */
function libraryBadge(item: LibraryItem | undefined) {
  const badge = item ? posterBadge(item) : null;
  return badge ? <StatusPill status={badge.status} label={badge.label} pct={badge.pct} onArt /> : libraryPill(item);
}

/**
 * The infinite-scroll footer: a row of skeletons and "Loading more…". It
 * asks for the next page when it comes within 200 px, and looks again after
 * each page (a tall screen can still see it).
 */
function LoadMore({ pages, onLoadMore }: { pages: number; onLoadMore: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const load = useRef(onLoadMore);
  useEffect(() => {
    load.current = onLoadMore;
  });

  useEffect(() => {
    const footer = ref.current;
    if (!footer || typeof IntersectionObserver === 'undefined') return;
    const observer = new IntersectionObserver((entries) => entries.some((entry) => entry.isIntersecting) && load.current(), { rootMargin: '200px 0px' });
    observer.observe(footer);
    return () => observer.disconnect();
  }, [pages]);

  return (
    <div ref={ref} className="disc-more">
      <PosterGrid className="disc-skelrow">
        <PosterSkeletons count={10} />
      </PosterGrid>
      <p className="disc-more-label">
        <CatEyes width={30} />
        <span>Loading more…</span>
      </p>
    </div>
  );
}

function DiscoverError({ error, onRetry, retrying, more = false }: { error: unknown; onRetry: () => void; retrying: boolean; more?: boolean }) {
  const code = isApiError(error) ? error.code : '';
  const detail =
    code === 'tmdb_not_configured'
      ? 'TMDB_API_KEY isn’t set. Add it to .env and restart the web app.'
      : code === 'tmdb_auth'
        ? 'TMDB rejected the API key. Check TMDB_API_KEY.'
        : 'TMDB didn’t answer. Your library and Infuse aren’t affected.';
  return (
    <Banner
      tone="error"
      role="alert"
      title={more ? 'Couldn’t load more titles' : 'Couldn’t load Discover'}
      action={
        <Button size="sm" onClick={onRetry} loading={retrying}>
          Try again
        </Button>
      }
    >
      {detail}
    </Banner>
  );
}

/** Desktop only (CSS): the link opens Discover with these filters. */
function CopyLinkButton({ filters }: { filters: DiscoverFilters }) {
  const { toast } = useToast();
  const copy = async () => {
    const ok = await copyText(`${window.location.origin}${discoverHref(filters)}`);
    toast(
      ok
        ? { tone: 'ready', icon: Link2, title: 'Link copied', body: 'It opens Discover with these filters.' }
        : { tone: 'error', title: 'Couldn’t copy the link', body: 'Copy it from the address bar instead.' }
    );
  };
  return (
    <Button variant="ghost" size="sm" className="disc-copy" onClick={() => void copy()}>
      <Link2 size={16} aria-hidden="true" />
      Copy link
    </Button>
  );
}
