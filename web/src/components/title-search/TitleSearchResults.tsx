'use client';

import { Banner, Button, LoadingRegion, PosterCard, PosterGrid, PosterSkeletons, Skeleton, Tabs } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { isApiError } from '@/lib/api/client';
import type { TitleSearchItem } from '@/lib/api/contract';
import { inTab, SEARCH_TABS, type SearchTab, TAB_NOUN, TMDB_HEADING } from '@/lib/title-search/results';
import { titleHref } from '@/lib/title/href';
import { Highlight } from './Highlight';
import { LibraryResultRow } from './LibraryResultRow';
import type { ShownSearch } from './useTitleSearch';

const TAB_LABEL: Record<SearchTab, string> = { all: 'All', tv: 'Shows', movie: 'Movies' };
const PANEL_ID = 'srch-results';

interface TitleSearchResultsProps {
  shown: ShownSearch;
  tab: SearchTab;
  onTab: (tab: SearchTab) => void;
  onRetry: () => void;
  retrying: boolean;
}

/**
 * Results (search.js `resultsBody`): tabs with counts, "In your library"
 * rows with their actions, then the TMDB titles you don't have. An empty
 * tab offers all results.
 */
export function TitleSearchResults({ shown, tab, onTab, onRetry, retrying }: TitleSearchResultsProps) {
  const { query, sections, tmdbError, libraryError } = shown;
  const visible = inTab(sections, tab);
  const empty = visible.library.length === 0 && visible.titles.length === 0;

  return (
    <>
      <div className="srch-tabs">
        <Tabs
          aria-label="Result type"
          value={tab}
          onChange={onTab}
          tabs={SEARCH_TABS.map((value) => ({ value, label: TAB_LABEL[value], count: sections.counts[value], controls: PANEL_ID }))}
        />
      </div>
      <div id={PANEL_ID} role="tabpanel" aria-label={`${TAB_LABEL[tab]} results`} className="srch-sections">
        {tmdbError && <TmdbError error={tmdbError} onRetry={onRetry} retrying={retrying} />}

        {visible.library.length > 0 && (
          <section className="section" aria-labelledby="srch-lib-h">
            <div className="srch-group-h">
              <h2 className="t-title-3" id="srch-lib-h">
                In your library
              </h2>
              <span className="n">{visible.library.length}</span>
            </div>
            <div className="srch-lib">
              {visible.library.map((hit) => (
                <LibraryResultRow key={`${hit.item.media_type}:${hit.item.tmdb_id}`} hit={hit} query={query} />
              ))}
            </div>
          </section>
        )}

        {libraryError && (
          <Banner tone="warning" title="Couldn’t check your library">
            Some of these titles may already be in it.
          </Banner>
        )}

        {visible.titles.length > 0 && (
          <section className="section" aria-labelledby="srch-tmdb-h">
            <div className="srch-group-h">
              <h2 className="t-title-3" id="srch-tmdb-h">
                {TMDB_HEADING[tab]}
              </h2>
              <span className="n">{visible.titles.length} on TMDB</span>
            </div>
            <PosterGrid className="srch-hit">
              {visible.titles.map((item) => (
                <TitleCard key={`${item.media_type}:${item.tmdb_id}`} item={item} query={query} />
              ))}
            </PosterGrid>
          </section>
        )}

        {tab !== 'all' && empty && sections.counts.all > 0 && (
          <p className="srch-tab-empty text-fg-2">
            No {TAB_NOUN[tab]} for “{query}”.
            <Button variant="ghost" size="sm" onClick={() => onTab('all')}>
              {sections.counts.all === 1 ? 'See the 1 result' : `See all ${sections.counts.all} results`}
            </Button>
          </p>
        )}
      </div>
    </>
  );
}

/** A TMDB title: the match marked, and why it matched when it wasn't by its title. */
function TitleCard({ item, query }: { item: TitleSearchItem; query: string }) {
  // TMDB search has no runtimes, so the type stands in (as on Discover)
  const facts = [item.year, item.media_type === 'tv' ? 'Series' : 'Movie'].filter(Boolean).join(' · ');
  return (
    <PosterCard
      href={titleHref(item.media_type, item.tmdb_id)}
      title={item.title}
      titleContent={<Highlight text={item.title} query={query} />}
      subtitle={item.match_reason ?? facts}
      posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
    />
  );
}

/** Rows and posters the size of the results, after 150 ms. */
export function SearchSkeleton() {
  return (
    <LoadingRegion label="Searching" className="srch-sections">
      <Skeleton width={220} height={44} />
      <div className="srch-lib">
        <Skeleton className="srch-row-skel" />
      </div>
      <PosterGrid>
        <PosterSkeletons count={6} />
      </PosterGrid>
    </LoadingRegion>
  );
}

function TmdbError({ error, onRetry, retrying }: { error: Error; onRetry: () => void; retrying: boolean }) {
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
      title="Couldn’t search TMDB"
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
