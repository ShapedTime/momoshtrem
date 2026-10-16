'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Banner, Button, LoadingRegion, PosterGrid, PosterSkeletons, Skeleton } from '@/components/ui';
import { useLibraryRemove } from '@/hooks/useLibraryRemove';
import { DOWNLOAD_POLL_MS } from '@/hooks/useTitle';
import { useTraktIntegration } from '@/hooks/useTraktIntegration';
import { usePendingRemovals } from '@/hooks/useUndoableRemoval';
import type { LibraryPage } from '@/lib/api/contract';
import { libraryPagesQuery } from '@/lib/api/endpoints';
import { traktLinked } from '@/lib/for-you/state';
import { totalsLine } from '@/lib/library/display';
import { pendingLibraryItems, visibleItems } from '@/lib/library/pending';
import { libraryParams, libraryViewQuery, type LibraryViewState, parseLibraryView, withoutPending } from '@/lib/library/view';
import { formatNumber, plural } from '@/lib/title/format';
import { LibraryCard } from './LibraryCard';
import { LibraryFilteredEmpty, LibraryFirstEmpty } from './LibraryEmpty';
import { LibraryListHead, LibraryRow } from './LibraryRow';
import { LibraryToolbar } from './LibraryToolbar';

/** Titles per request; "Load more" fetches the next page. */
const PAGE_SIZE = 100;
/** Typing settles this long before the URL and the query follow. */
const SEARCH_DEBOUNCE_MS = 250;

const anyDownloading = (pages: LibraryPage[] | undefined) => Boolean(pages?.some((page) => page.items.some((item) => item.download)));

/**
 * /library (PLAN F7, #97; screens/library): the whole library as a fluid
 * poster grid or a list, with filter chips and their counts, sort and search,
 * all in the URL (`filter`, `sort`, `view`, `q`). Removing a title hides it at
 * once and sends the DELETE when its 10 s Undo closes.
 */
export function LibraryView() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const state = parseLibraryView(searchParams);
  const [q, setQ] = useState(state.q);
  const searchTimer = useRef<number | undefined>(undefined);
  const remove = useLibraryRemove();
  const pending = usePendingRemovals();
  // The watch lines only mean something once Trakt is linked (unknown counts as not); a finished sync refetches the library
  const integration = useTraktIntegration();
  const trakt = integration.data ? traktLinked(integration.data) : false;

  // Reads the URL at call time, so a debounced search never undoes a newer change (a chip, a sheet)
  const update = useCallback(
    (patch: Partial<LibraryViewState>) => {
      const current = new URLSearchParams(window.location.search);
      const query = libraryViewQuery({ ...parseLibraryView(current), ...patch }, current);
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router]
  );
  useEffect(() => () => window.clearTimeout(searchTimer.current), []);

  const onQueryChange = (value: string) => {
    setQ(value);
    window.clearTimeout(searchTimer.current);
    searchTimer.current = window.setTimeout(() => update({ q: value }), SEARCH_DEBOUNCE_MS);
  };
  const clearSearch = () => {
    window.clearTimeout(searchTimer.current);
    setQ('');
    update({ q: '' });
  };

  const query = useInfiniteQuery({
    ...libraryPagesQuery({ ...libraryParams(state), page_size: PAGE_SIZE }),
    // Download progress moves on its own while anything downloads (paused in hidden tabs)
    refetchInterval: (current) => (anyDownloading(current.state.data?.pages) ? DOWNLOAD_POLL_MS : false),
  });

  const pages = query.data?.pages;
  const first = pages?.[0];
  const items = visibleItems(pages?.flatMap((page) => page.items) ?? [], pending);
  const adjusted = first ? withoutPending(first, pendingLibraryItems(pending), state) : null;
  const firstTime = adjusted !== null && adjusted.summary.total === 0;

  const totals = !adjusted ? (query.isError ? '' : 'Loading…') : firstTime ? 'Nothing here yet' : totalsLine(adjusted.summary);

  return (
    <div className="lib-page">
      <header className="page lib-head">
        <h1 className="t-display">Library</h1>
        {totals && <p className="t-small text-fg-3">{totals}</p>}
      </header>

      <div className="page lib-body">
        {!adjusted && query.isPending && <LibrarySkeleton />}

        {!adjusted && query.isError && (
          <Banner
            tone="error"
            role="alert"
            title="Couldn’t load your library"
            action={
              <Button size="sm" onClick={() => void query.refetch()} loading={query.isRefetching}>
                Try again
              </Button>
            }
          >
            momoshtrem didn’t answer. Infuse still plays what’s ready.
          </Banner>
        )}

        {firstTime && <LibraryFirstEmpty />}

        {adjusted && !firstTime && (
          <>
            <LibraryToolbar
              q={q}
              onQueryChange={onQueryChange}
              onClearSearch={clearSearch}
              filter={state.filter}
              onFilterChange={(filter) => update({ filter })}
              counts={adjusted.counts}
              total={adjusted.total}
              sort={state.sort}
              onSortChange={(sort) => update({ sort })}
              view={state.view}
              onViewChange={(view) => update({ view })}
            />
            <p className="sr-only" role="status">
              {plural(adjusted.total, 'title')}
            </p>
            <div className="lib-results" aria-busy={query.isPlaceholderData || undefined}>
              {items.length === 0 ? (
                <LibraryFilteredEmpty q={state.q} filter={state.filter} trakt={trakt} onClearSearch={clearSearch} onShowAll={() => update({ filter: 'all' })} />
              ) : state.view === 'list' ? (
                <>
                  <LibraryListHead />
                  <ul className="lrows" aria-label="Your library">
                    {items.map((item) => (
                      <LibraryRow key={`${item.media_type}-${item.id}`} item={item} onRemove={remove} trakt={trakt} />
                    ))}
                  </ul>
                </>
              ) : (
                <PosterGrid className="lib-grid">
                  {items.map((item) => (
                    <LibraryCard key={`${item.media_type}-${item.id}`} item={item} onRemove={remove} trakt={trakt} />
                  ))}
                </PosterGrid>
              )}
              {query.hasNextPage && (
                <div className="lib-more">
                  <p className="t-small text-fg-3">
                    Showing {formatNumber(items.length)} of {formatNumber(adjusted.total)}
                  </p>
                  <Button onClick={() => void query.fetchNextPage()} loading={query.isFetchingNextPage}>
                    Load more
                  </Button>
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/** The toolbar and grid in their final sizes, after 150 ms (library.js state `loading`). */
function LibrarySkeleton() {
  return (
    <LoadingRegion label="Loading your library" className="flex flex-col gap-5">
      <div className="lib-tools--skel">
        <Skeleton height={44} className="rounded-sm" />
        <div className="flex gap-2 overflow-hidden">
          {[56, 76, 70, 88, 72, 64, 104].map((width) => (
            <Skeleton key={width} width={width} height={36} className="shrink-0 rounded-full" />
          ))}
        </div>
      </div>
      <PosterGrid className="lib-grid">
        <PosterSkeletons count={12} />
      </PosterGrid>
    </LoadingRegion>
  );
}
