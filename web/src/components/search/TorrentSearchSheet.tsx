'use client';

import { type FormEvent, type ReactNode, type Ref, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { History, Magnet, X } from 'lucide-react';
import { SourceSheet, useSourceTitle, useSwitchSource } from '@/components/source';
import { Button, copyText, IconButton, SearchInput, SegmentedControl, Sheet, Skeleton, Spinner, useToast } from '@/components/ui';
import { useAssignTorrent } from '@/hooks/useAssignTorrent';
import { useHDRezkaEnabled } from '@/hooks/useHDRezka';
import { useInspect, useReleaseMagnet, useReleaseSearch } from '@/hooks/useReleaseSearch';
import type { Release, SearchScope, Title } from '@/lib/api/contract';
import { addLabel, alsoSearched, noResultsSuggestions, pagerText, resultsHeading, scopeNoun, searchProblem, statusLine } from '@/lib/search/copy';
import { type SearchFilters, toggleValue } from '@/lib/search/filters';
import { inspectAddLabel } from '@/lib/search/inspect';
import { defaultQuery, defaultScope, SCOPE_LABEL, scopeOptions, searchParams } from '@/lib/search/query';
import { freshDraft, loadDraft, saveDraft, type SearchDraft } from '@/lib/search/session';
import { bestRelease, packBanner, searchViewKind } from '@/lib/search/view';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { legacyScope, type SourceContext, sourceContext } from '@/lib/source/context';
import { legacySearchContext } from '@/lib/title/legacy-search';
import { FiltersSheet } from './FiltersSheet';
import { InspectSheet } from './InspectSheet';
import { ReleaseCard } from './ReleaseCard';
import { SearchChips } from './SearchChips';
import { FilteredEmpty, NoResults, PackBanner, PartialBanner, ReleaseSkeletons, SearchErrorBanner, SearchingState } from './SearchStates';
import { SortSheet } from './SortSheet';

/** Typing searches after this pause; Enter searches at once. */
const TYPING_DEBOUNCE_MS = 700;

const messageOf = (error: unknown) => (error instanceof Error ? error.message : undefined);

interface SearchHeadProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onClear: () => void;
  inputRef: Ref<HTMLInputElement>;
  scopes: SearchScope[];
  scope: SearchScope | null;
  onScope: (scope: SearchScope) => void;
  /** Under the scope: the filter and sort chips. */
  chips?: ReactNode;
}

/** Under the tabs: the query (Enter or a pause searches), the scope switch, then the chips. */
function SearchHead({ value, onChange, onSubmit, onClear, inputRef, scopes, scope, onScope, chips }: SearchHeadProps) {
  return (
    <>
      <form role="search" aria-label="Search torrents" onSubmit={onSubmit}>
        <SearchInput
          ref={inputRef}
          aria-label="Search releases"
          autoComplete="off"
          enterKeyHint="search"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          end={
            value ? (
              <IconButton size="sm" aria-label="Clear search" onClick={onClear}>
                <X size={16} aria-hidden="true" />
              </IconButton>
            ) : undefined
          }
        />
      </form>
      {scope && scopes.length > 0 && (
        <SegmentedControl
          block
          aria-label="Search scope"
          options={scopes.map((option) => ({ value: option, label: SCOPE_LABEL[option] }))}
          value={scope}
          onChange={onScope}
        />
      )}
      {chips}
    </>
  );
}

interface TorrentSearchProps {
  context: SourceContext;
  title: Title;
  close: () => void;
}

/**
 * The Torrents tab of the source sheet (HANDOFF §5.2): the query pre-filled
 * from the context (then the server's primary query), the scope, and results
 * to act on, polled while the search runs. Query, scope, sort and filters stay
 * per context for the session (`lib/search/session.ts`). Filters, Sort and
 * Inspect open nested sheets over it (#93).
 */
function TorrentSearch({ context, title, close }: TorrentSearchProps) {
  const [restored] = useState(() => loadDraft(context.key) !== null);
  const [draft, setDraft] = useState<SearchDraft>(() => loadDraft(context.key) ?? freshDraft());
  /** What `q` sends: only text the user typed and submitted. */
  const [q, setQ] = useState(() => (draft.dirty ? draft.query.trim() : ''));
  const [addingId, setAddingId] = useState<string | null>(null);
  const [copyingId, setCopyingId] = useState<string | null>(null);
  /** The nested sheet on top, if any; the Filters sheet mounts afresh each time it opens, so its draft starts over. */
  const [nested, setNested] = useState<'filters' | 'sort' | null>(null);
  const [filtersOpened, setFiltersOpened] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const focusIndex = useRef<number | null>(null);
  const { toast } = useToast();
  const hdrezka = useHDRezkaEnabled();
  const switchTo = useSwitchSource(context, close, title);

  /** Every change is kept for the session (only changes: just opening and closing isn't "where you left off"). */
  const update = (patch: Partial<SearchDraft>) => {
    const next = { ...draft, ...patch };
    setDraft(next);
    saveDraft(context.key, next);
  };

  const scope = draft.scope ?? defaultScope(context);
  const paramsFor = (filters: SearchFilters) => searchParams(context, { scope, sort: draft.sort, filters, q });
  const params = paramsFor(draft.filters);
  const { query, first, last, items, retry } = useReleaseSearch(params);
  const guess = defaultQuery(context, title, scope);
  const primary = first?.queries[0]?.q;
  const shownQuery = q || primary || guess;

  // A pause in typing searches what's there
  useEffect(() => {
    const text = draft.query.trim();
    if (!draft.dirty || !text || text === q) return;
    const timer = setTimeout(() => setQ(text), TYPING_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [draft.dirty, draft.query, q]);

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!draft.dirty) return;
    const text = draft.query.trim();
    // An emptied field goes back to the generated queries
    if (!text) update({ query: '', dirty: false });
    setQ(text);
  };
  const searchFor = (text: string) => {
    update({ query: text, dirty: true });
    setQ(text);
  };

  // Adding: resolve a magnet first when the release has none, then the mapping job (a show's review, or a movie's wait)
  const assignContext = useMemo(() => legacySearchContext(title, legacyScope(context)), [title, context]);
  const inspect = useInspect(context);
  // Navigating to the review drops `?sheet`, which closes the sheet (Back from the review reopens it);
  // a movie assigned from Inspect closes Inspect, back to its card saying Added
  const { add, isAdding, isAdded } = useAssignTorrent(assignContext, { onLeave: () => {}, onApplied: inspect.close });
  const magnets = useReleaseMagnet();
  const onAdd = async (release: Release) => {
    setAddingId(release.id);
    try {
      add(await magnets.magnetFor(release), release.indexer);
    } catch (error) {
      toast({ tone: 'error', title: 'Couldn’t get the magnet link', body: messageOf(error) });
    } finally {
      setAddingId(null);
    }
  };
  const onCopyMagnet = async (release: Release) => {
    setCopyingId(release.id);
    try {
      const copied = await copyText(await magnets.magnetFor(release));
      toast(
        copied
          ? { tone: 'ready', icon: Magnet, title: 'Magnet link copied' }
          : { tone: 'error', title: 'Couldn’t copy the magnet link', body: 'The browser blocked the clipboard. Try again.' }
      );
    } catch (error) {
      toast({ tone: 'error', title: 'Couldn’t get the magnet link', body: messageOf(error) });
    } finally {
      setCopyingId(null);
    }
  };
  const onRetry = () =>
    retry.mutate(undefined, { onError: (error) => toast({ tone: 'error', title: 'Couldn’t search again', body: messageOf(error) }) });

  // Load more appends below, so the scroll stays put; focus moves to the first new card
  const loadMore = () => {
    focusIndex.current = items.length;
    void query.fetchNextPage();
  };
  const focusNewCard = useCallback((card: HTMLElement | null) => {
    if (card && focusIndex.current !== null && card.dataset.index === String(focusIndex.current)) {
      focusIndex.current = null;
      card.focus({ preventScroll: true });
    }
  }, []);

  const view = searchViewKind(first, items.length, query.isError);
  const best = bestRelease(first, draft.sort);
  const rest = best ? items.slice(1) : items;
  const missing = packBanner(first, scope);
  const also = first ? alsoSearched(first.queries) : null;
  const label = addLabel(title);

  const adding = (release: Release) => {
    const magnet = magnets.knownMagnet(release);
    return addingId === release.id || (magnet !== null && isAdding(magnet));
  };

  const card = (release: Release, index: number) => {
    const magnet = magnets.knownMagnet(release);
    return (
      <ReleaseCard
        key={release.id}
        ref={focusNewCard}
        index={index}
        release={release}
        best={release === best}
        addLabel={label}
        adding={adding(release)}
        added={magnet !== null && isAdded(magnet)}
        copying={copyingId === release.id}
        onAdd={() => void onAdd(release)}
        onCopyMagnet={() => void onCopyMagnet(release)}
        onInspect={() => inspect.open(release)}
      />
    );
  };

  return (
    <SourceSheet
      tab="torrent"
      context={context}
      close={close}
      head={
        <SearchHead
          value={draft.dirty ? draft.query : shownQuery}
          onChange={(value) => update({ query: value, dirty: true })}
          onSubmit={onSubmit}
          onClear={() => {
            update({ query: '', dirty: true });
            inputRef.current?.focus();
          }}
          inputRef={inputRef}
          scopes={scopeOptions(context)}
          scope={scope}
          onScope={(next) => update({ scope: next })}
          chips={
            <SearchChips
              facets={first?.facets}
              filters={draft.filters}
              sort={draft.sort}
              onToggle={(key, value) => update({ filters: toggleValue(draft.filters, key, value) })}
              onOpenFilters={() => {
                setFiltersOpened((n) => n + 1);
                setNested('filters');
              }}
              onOpenSort={() => setNested('sort')}
            />
          }
        />
      }
    >
      {restored && (
        <p className="t-small flex items-center gap-1.5 text-fg-3">
          <History size={14} aria-hidden="true" /> Picked up where you left off
        </p>
      )}
      {view === 'searching' && <SearchingState q={shownQuery} indexers={first?.indexers ?? []} queries={first?.queries ?? []} />}
      {query.isError && (
        <SearchErrorBanner problem={searchProblem(query.error)} retrying={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {first?.complete && view !== 'searching' && view !== 'error' && (
        <PartialBanner indexers={first.indexers} retrying={retry.isPending} onRetry={onRetry} />
      )}
      {first && view === 'results' && (
        <div className="ts-status t-small text-fg-3">
          <p className="flex items-center gap-2">
            {statusLine(first)}
            {!first.complete && <Spinner size={14} label="Still searching" />}
          </p>
          {also && <p>{also}</p>}
          {first.warnings.map((warning) => (
            <p key={warning} className="text-warning">
              {warning}
            </p>
          ))}
        </div>
      )}
      {view === 'none' && first && (
        <NoResults
          q={shownQuery}
          indexers={first.indexers.length}
          hidden={first.hidden}
          suggestions={noResultsSuggestions({ q: shownQuery, title, scope, episode: context.kind === 'show' ? context.episode : null, queries: first.queries })}
          onSearch={searchFor}
          onScope={(next) => update({ scope: next })}
          onTryHDRezka={hdrezka ? () => switchTo('hdrezka') : undefined}
        />
      )}
      {view === 'filtered-empty' && first && (
        <FilteredEmpty
          total={first.total_unfiltered}
          hidden={first.hidden}
          filters={draft.filters}
          noun={scopeNoun(scope)}
          onChange={(filters) => update({ filters })}
        />
      )}
      {view === 'results' && first && (
        <>
          {missing && (
            <PackBanner
              missing={missing}
              absolute={title.media_type === 'tv' && title.episode_ordering === 'absolute'}
              onSeasonPacks={() => update({ scope: 'season' })}
            />
          )}
          {best && card(best, 0)}
          {rest.length > 0 && <h3 className="t-micro ts-sub text-fg-3">{resultsHeading(best !== null, draft.sort)}</h3>}
          <div className="flex flex-col gap-3">{rest.map((release, i) => card(release, i + (best ? 1 : 0)))}</div>
          {query.isFetchingNextPage && <ReleaseSkeletons count={2} />}
          <div className="ts-pager">
            <span className="t-small t-num text-fg-3">{pagerText(items.length, last?.total ?? first.total)}</span>
            {query.hasNextPage && (
              <Button loading={query.isFetchingNextPage} onClick={loadMore}>
                Load more
              </Button>
            )}
          </div>
        </>
      )}
      {/* Nested over the search: inside its tree, so a phone's drawer stacks on it */}
      <FiltersSheet
        key={filtersOpened}
        open={nested === 'filters'}
        onOpenChange={(open) => !open && setNested(null)}
        mediaType={title.media_type}
        filters={draft.filters}
        paramsFor={paramsFor}
        current={first}
        onApply={(filters) => {
          update({ filters });
          setNested(null);
        }}
      />
      <SortSheet
        open={nested === 'sort'}
        onOpenChange={(open) => !open && setNested(null)}
        sort={draft.sort}
        onPick={(sort) => {
          update({ sort });
          setNested(null);
        }}
      />
      <InspectSheet
        open={inspect.isOpen}
        onOpenChange={(open) => !open && inspect.close()}
        release={inspect.release}
        starting={inspect.start.isPending}
        startError={inspect.start.error}
        detail={inspect.job?.data}
        readError={inspect.job?.error ?? null}
        onRetry={inspect.retry}
        addLabel={inspectAddLabel(title)}
        adding={inspect.release !== null && adding(inspect.release)}
        onAdd={() => {
          if (inspect.release) void onAdd(inspect.release);
        }}
      />
    </SourceSheet>
  );
}

/** Until the title is here (a deep link): the head's place and skeletons, at full height. */
function SearchSkeleton({ context, close }: { context: SourceContext; close: () => void }) {
  return (
    <SourceSheet tab="torrent" context={context} close={close} head={<Skeleton height={44} />}>
      <ReleaseSkeletons />
    </SourceSheet>
  );
}

function TorrentSearchContext({ context, close }: { context: SourceContext; close: () => void }) {
  const { title } = useSourceTitle(context);
  if (!title) return <SearchSkeleton context={context} close={close} />;
  return <TorrentSearch context={context} title={title} close={close} />;
}

/**
 * The Torrents tab of the source sheet (`?sheet=torrent&movie=<tmdb>`, or
 * `&show=<tmdb>&season=n&se=S01E29`; #92). Nothing opens it until
 * `TORRENT_SHEET_LIVE` (#95): the deep link works, the entry points still
 * open the legacy search.
 */
export function TorrentSearchSheet({ params, close }: SheetComponentProps) {
  const context = sourceContext(params);
  if (!context) {
    return (
      <Sheet open onOpenChange={(open) => !open && close()} title="Find a source" size="narrow">
        <p className="text-fg-2">This link doesn’t say which movie or show to find a torrent for. Open it from a title’s page.</p>
      </Sheet>
    );
  }
  return <TorrentSearchContext key={context.key} context={context} close={close} />;
}
