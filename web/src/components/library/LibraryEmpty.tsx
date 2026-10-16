import { Check, Compass, Search } from 'lucide-react';
import { Button, ButtonLink, EmptyState } from '@/components/ui';
import { filterDefinition, type LibraryFilter } from '@/lib/library/view';
import { NAV } from '@/lib/nav';

const discoverHref = NAV.find((item) => item.id === 'discover')?.href ?? '/discover';
const searchHref = NAV.find((item) => item.id === 'search')?.href ?? '/search';
const traktSettingsHref = '/settings?section=trakt';

/** Nothing in the library yet (library.js state `empty`). */
export function LibraryFirstEmpty() {
  return (
    <EmptyState
      cat="sleep"
      title="Your library is empty"
      className="lib-first"
      actions={
        <>
          <ButtonLink href={discoverHref} variant="primary">
            <Compass size={18} aria-hidden="true" /> Discover
          </ButtonLink>
          <ButtonLink href={searchHref} variant="secondary">
            <Search size={18} aria-hidden="true" /> Search titles
          </ButtonLink>
        </>
      }
    >
      Add a movie or show and Noir gets it ready for Infuse. Everything you add lives here, with what’s ready and what you’ve watched.
    </EmptyState>
  );
}

interface LibraryFilteredEmptyProps {
  q: string;
  filter: LibraryFilter;
  /** Trakt is linked: without it nothing is ever under Watching. */
  trakt: boolean;
  onClearSearch: () => void;
  onShowAll: () => void;
}

/** The filters or the search match nothing (library.js `resultsHTML`). */
export function LibraryFilteredEmpty({ q, filter, trakt, onClearSearch, onShowAll }: LibraryFilteredEmptyProps) {
  const query = q.trim();
  if (query) {
    return (
      <EmptyState
        title={`No titles match “${query}”`}
        className="lib-empty"
        actions={
          <>
            <Button variant="secondary" onClick={onClearSearch}>
              Clear search
            </Button>
            <ButtonLink href={`${searchHref}?q=${encodeURIComponent(query)}`} variant="ghost">
              <Search size={17} aria-hidden="true" /> Search everywhere
            </ButtonLink>
          </>
        }
      >
        Nothing in your library by that name. It might be out there, though.
      </EmptyState>
    );
  }
  const showAll = (
    <Button variant="secondary" onClick={onShowAll}>
      Show all titles
    </Button>
  );
  if (filter === 'watching' && !trakt) {
    return (
      <EmptyState
        title="Connect Trakt to see what you’re watching"
        className="lib-empty"
        actions={
          <>
            <ButtonLink href={traktSettingsHref} variant="primary">
              Connect Trakt
            </ButtonLink>
            {showAll}
          </>
        }
      >
        Infuse scrobbles your plays to Trakt. Once it’s connected, the shows you’re partway through show up here.
      </EmptyState>
    );
  }
  if (filter === 'review') {
    return (
      <EmptyState title="Nothing needs review" className="lib-empty" badge={<Check size={16} strokeWidth={2.6} />} actions={showAll}>
        Every file in your library has its episode. Noir checks new torrents as they arrive.
      </EmptyState>
    );
  }
  return (
    <EmptyState title={`Nothing under ${filterDefinition(filter).label}`} className="lib-empty" actions={showAll}>
      Try another filter.
    </EmptyState>
  );
}
