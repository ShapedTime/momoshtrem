'use client';

import {
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  type RefObject,
  useCallback,
  useEffectEvent,
  useId,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { defaultRangeExtractor, type Range, useVirtualizer } from '@tanstack/react-virtual';
import { Check, EyeOff, Info, Search } from 'lucide-react';
import { Banner, Button, FilePath, FilterChip, SearchInput, SegmentedControl, Sheet, Skeleton } from '@/components/ui';
import type { Proposal, ShowEpisodesSeason } from '@/lib/api/contract';
import {
  episodeCount,
  episodeNumberLabel,
  initialSeason,
  PICKER_VIRTUALIZE_ABOVE,
  type PickerEpisode,
  type PickerRow,
  pickerRows,
  type PickerTag,
  seasonShortLabel,
} from '@/lib/mapping/picker';
import { episodeCode } from '@/lib/title/episodes';
import { cx } from '@/lib/utils/cx';

/** The catalog's load state (GET /shows/{id}/episodes). */
export type PickerCatalog =
  | { status: 'pending' }
  | { status: 'error'; message: string; retry: () => void }
  | { status: 'success'; seasons: ShowEpisodesSeason[] };

type EpisodeRow = Extract<PickerRow, { type: 'episode' }>;
type HeaderRow = Extract<PickerRow, { type: 'header' }>;

const isEpisode = (row: PickerRow | undefined): row is EpisodeRow => row?.type === 'episode';
const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

// ---- Rows --------------------------------------------------------------------------------

function TagView({ tag }: { tag: PickerTag }) {
  if (!tag) return null;
  if (tag.kind === 'current') {
    return (
      <span className="pk-tag pk-tag--cur">
        <Check size={13} strokeWidth={2.4} aria-hidden="true" />
        Current
      </span>
    );
  }
  if (tag.kind === 'taken') {
    return (
      <span className="pk-tag pk-tag--batch" title={`Taken by ${tag.by}`}>
        Taken · {tag.by}
      </span>
    );
  }
  return <span className="pk-tag">In library</span>;
}

interface PlacedProps {
  style?: CSSProperties;
  measure?: (node: Element | null) => void;
  index?: number;
}

interface OptionProps extends PlacedProps {
  row: EpisodeRow;
  id: string;
  active: boolean;
  disabled: boolean;
  onPick: (episode: PickerEpisode) => void;
  onActivate: (episodeId: number) => void;
}

function Option({ row, id, active, disabled, onPick, onActivate, style, measure, index }: OptionProps) {
  const { episode, tag } = row;
  const current = tag?.kind === 'current';
  return (
    <div
      ref={measure}
      data-index={index}
      id={id}
      role="option"
      aria-selected={current}
      aria-disabled={disabled || undefined}
      className={cx('pk-ep', current && 'is-current', tag && !current && 'is-taken', active && 'is-active')}
      style={style}
      onClick={() => {
        onActivate(episode.id);
        if (!disabled) onPick(episode);
      }}
    >
      <span className="no t-num" aria-hidden="true">
        {episodeNumberLabel(episode)}
      </span>
      <span className="nm">
        <span className="t">
          <span className="sr-only">{episodeCode(episode.season, episode.episode)} </span>
          {episode.name}
        </span>
        {episode.absolute !== null && episode.absolute !== episode.episode && <span className="abs t-num">Absolute #{episode.absolute}</span>}
      </span>
      <TagView tag={tag} />
    </div>
  );
}

function Header({ row, style, measure, index }: PlacedProps & { row: HeaderRow }) {
  return (
    <div ref={measure} data-index={index} data-season={row.season} className="pk-shead" aria-hidden="true" style={style}>
      <span>{row.label}</span>
      <span className="t-num">{row.count} ep</span>
    </div>
  );
}

function groupBySeason(rows: PickerRow[]): Array<{ header: HeaderRow; episodes: EpisodeRow[] }> {
  const groups: Array<{ header: HeaderRow; episodes: EpisodeRow[] }> = [];
  for (const row of rows) {
    if (row.type === 'header') groups.push({ header: row, episodes: [] });
    else groups.at(-1)?.episodes.push(row);
  }
  return groups;
}

// ---- The list ------------------------------------------------------------------------------

interface ListHandle {
  scrollToSeason: (season: number) => void;
  focus: () => void;
}

interface PickerListProps {
  rows: PickerRow[];
  virtual: boolean;
  /** Nothing to assign to yet (desktop, no file selected). */
  disabled: boolean;
  query: string;
  /** Where the list opens when the file has no episode yet. */
  startSeason: number | null;
  onPick: (episode: PickerEpisode) => void;
  ref?: Ref<ListHandle>;
}

/**
 * A listbox of episodes under sticky season headers. Focus stays on the list
 * (aria-activedescendant): ↑ ↓ Home End PageUp PageDown move, Enter or Space
 * picks. Over 60 episodes the rows are virtualised.
 */
function PickerList({ rows, virtual, disabled, query, startSeason, onPick, ref }: PickerListProps) {
  const listId = useId();
  const listRef = useRef<HTMLDivElement>(null);
  const optionId = useCallback((episodeId: number) => `${listId}-ep-${episodeId}`, [listId]);
  const episodeIndexes = useMemo(() => rows.flatMap((row, i) => (isEpisode(row) ? [i] : [])), [rows]);
  const headerIndexes = useMemo(() => rows.flatMap((row, i) => (row.type === 'header' ? [i] : [])), [rows]);
  const [activeId, setActiveId] = useState<number | null>(() => rows.find((row): row is EpisodeRow => isEpisode(row) && row.tag?.kind === 'current')?.episode.id ?? null);
  const activeIndex = rows.findIndex((row) => isEpisode(row) && row.episode.id === activeId);
  const shownActive = activeIndex >= 0 ? activeIndex : (episodeIndexes[0] ?? -1);
  const shownRow = rows[shownActive];
  const shownActiveId = isEpisode(shownRow) ? shownRow.episode.id : null;

  // Virtual: the season header above the first visible row stays rendered, and sticks
  const stickyFor = useCallback((start: number) => [...headerIndexes].reverse().find((index) => start >= index) ?? 0, [headerIndexes]);
  const rangeExtractor = useCallback(
    (range: Range) => [...new Set([stickyFor(range.startIndex), ...defaultRangeExtractor(range)])].sort((a, b) => a - b),
    [stickyFor]
  );
  // No React Compiler in this app; the list re-renders as it scrolls anyway
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: virtual ? rows.length : 0,
    // Only a virtual list: on attach the virtualiser scrolls its element to its own offset
    getScrollElement: () => (virtual ? listRef.current : null),
    estimateSize: (index) => (rows[index]?.type === 'header' ? 38 : 50),
    overscan: 10,
    rangeExtractor,
  });

  /** A plain row's top inside the list (a header: its season block, which doesn't stick). */
  const plainTop = useCallback(
    (index: number): number | null => {
      const list = listRef.current;
      const row = rows[index];
      if (!list || !row) return null;
      const node = isEpisode(row)
        ? document.getElementById(optionId(row.episode.id))
        : list.querySelector<HTMLElement>(`[data-season="${row.season}"]`)?.parentElement;
      if (!node) return null;
      return node.getBoundingClientRect().top - list.getBoundingClientRect().top + list.scrollTop;
    },
    [rows, optionId]
  );

  const scrollToIndex = useCallback(
    (index: number, align: 'start' | 'center' | 'auto', smooth = false) => {
      const list = listRef.current;
      if (!list || index < 0) return;
      if (virtual) {
        virtualizer.scrollToIndex(index, { align });
        return;
      }
      const row = rows[index];
      if (align === 'auto') {
        if (isEpisode(row)) document.getElementById(optionId(row.episode.id))?.scrollIntoView({ block: 'nearest' });
        return;
      }
      const top = plainTop(index);
      if (top === null) return;
      const offset = align === 'center' ? Math.min(160, list.clientHeight / 3) : 0;
      list.scrollTo({ top: Math.max(0, top - offset), behavior: smooth && !prefersReducedMotion() ? 'smooth' : 'auto' });
    },
    [virtual, virtualizer, rows, optionId, plainTop]
  );

  useImperativeHandle(
    ref,
    () => ({
      scrollToSeason: (season) => scrollToIndex(rows.findIndex((row) => row.type === 'header' && row.season === season), 'start', true),
      focus: () => listRef.current?.focus({ preventScroll: true }),
    }),
    [rows, scrollToIndex]
  );

  // First show: the current episode a third of the way down, else the file's season at the top.
  // Once per mount (twice under StrictMode, after the virtualiser has attached), never as the search narrows the rows.
  const scrollToStart = useEffectEvent(() => {
    const current = rows.findIndex((row) => isEpisode(row) && row.tag?.kind === 'current');
    if (current >= 0) scrollToIndex(current, 'center');
    else if (startSeason !== null) scrollToIndex(rows.findIndex((row) => row.type === 'header' && row.season === startSeason), 'start');
  });
  useLayoutEffect(() => {
    scrollToStart();
  }, []);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (episodeIndexes.length === 0) return;
    const at = Math.max(0, episodeIndexes.indexOf(shownActive));
    const last = episodeIndexes.length - 1;
    const steps: Record<string, number> = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0, End: last, PageDown: at + 8, PageUp: at - 8 };
    if (event.key in steps) {
      event.preventDefault();
      const next = episodeIndexes[Math.min(last, Math.max(0, steps[event.key]))];
      const row = rows[next];
      if (isEpisode(row)) setActiveId(row.episode.id);
      scrollToIndex(next, 'auto');
    } else if ((event.key === 'Enter' || event.key === ' ') && !disabled && isEpisode(shownRow)) {
      event.preventDefault();
      onPick(shownRow.episode);
    }
  };

  if (rows.length === 0) {
    return (
      <div className="pk-list">
        <div className="pk-empty" role="status">
          <Search size={20} aria-hidden="true" />
          <p>No episode matches “{query}”.</p>
          <p className="t-small text-fg-3">Try a number like 35, an absolute number, or S00E03.</p>
        </div>
      </div>
    );
  }

  const optionProps = (row: EpisodeRow) => ({
    row,
    id: optionId(row.episode.id),
    active: row.episode.id === shownActiveId,
    disabled,
    onPick,
    onActivate: setActiveId,
  });
  // getVirtualItems() computes this render's range first; the sticky header follows it
  const items = virtual ? virtualizer.getVirtualItems() : [];
  const sticky = stickyFor(virtualizer.range?.startIndex ?? 0);

  return (
    <div
      ref={listRef}
      role="listbox"
      tabIndex={0}
      aria-label="Episodes"
      aria-activedescendant={shownActiveId !== null ? optionId(shownActiveId) : undefined}
      className="pk-list"
      onKeyDown={onKeyDown}
    >
      {virtual ? (
        <div className="pk-virtual" style={{ height: virtualizer.getTotalSize() }}>
          {items.map((item) => {
            const row = rows[item.index];
            const style: CSSProperties =
              row.type === 'header' && item.index === sticky
                ? { position: 'sticky', top: 0, zIndex: 2 }
                : { position: 'absolute', top: 0, transform: `translateY(${item.start}px)` };
            return isEpisode(row) ? (
              <Option key={item.key} {...optionProps(row)} style={style} measure={virtualizer.measureElement} index={item.index} />
            ) : (
              <Header key={item.key} row={row} style={style} measure={virtualizer.measureElement} index={item.index} />
            );
          })}
        </div>
      ) : (
        groupBySeason(rows).map(({ header, episodes }) => (
          <div key={header.season} role="presentation">
            <Header row={header} />
            <div className="pk-eps" role="presentation">
              {episodes.map((row) => (
                <Option key={row.episode.id} {...optionProps(row)} />
              ))}
            </div>
          </div>
        ))
      )}
    </div>
  );
}

// ---- Search, tools and list ------------------------------------------------------------------

interface PickerProps {
  catalog: PickerCatalog;
  /** The file being mapped; null on desktop until one is selected. */
  proposal: Proposal | null;
  /** Its path as shown (without the torrent's folder). */
  path: string | null;
  /** Episodes other files in this job use, with that file's short name. */
  taken: ReadonlyMap<number, string>;
  hideTaken: boolean;
  onHideTaken: (hide: boolean) => void;
  onPick: (episode: PickerEpisode) => void;
}

interface PickerParts {
  head: ReactNode;
  /** The list once the catalog is in; its loading or error state until then. */
  body: ReactNode;
  loaded: boolean;
}

const NO_SEASONS: ShowEpisodesSeason[] = [];

/**
 * Search, season jump, Hide taken and the list, sharing one query. The parts
 * stay mounted while the catalog loads; only the list waits for it, and it
 * opens on the file's episode when it first shows.
 */
function usePickerParts({
  catalog,
  proposal,
  taken,
  hideTaken,
  onHideTaken,
  onPick,
  searchRef,
}: Omit<PickerProps, 'path'> & { searchRef?: RefObject<HTMLInputElement | null> }): PickerParts {
  const seasons = catalog.status === 'success' ? catalog.seasons : NO_SEASONS;
  const listRef = useRef<ListHandle>(null);
  const [query, setQuery] = useState('');
  const current = useMemo(() => new Set(proposal?.targets.map((t) => t.episode_id) ?? []), [proposal]);
  const rows = useMemo(() => pickerRows(seasons, { query, hideTaken, current, taken }), [seasons, query, hideTaken, current, taken]);
  // The file's season until you jump to another
  const [chosenSeason, setChosenSeason] = useState<number | null>(null);
  const startSeason = initialSeason(seasons, proposal);
  const season = chosenSeason ?? startSeason;

  const head = (
    <>
      <SearchInput
        ref={searchRef}
        aria-label="Search episodes"
        placeholder="Number, absolute # or title"
        autoComplete="off"
        enterKeyHint="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown') {
            event.preventDefault();
            listRef.current?.focus();
          }
        }}
      />
      <div className="pk-tools">
        {seasons.length > 1 ? (
          <SegmentedControl
            aria-label="Jump to season"
            options={seasons.map((s) => ({ value: String(s.season_number), label: seasonShortLabel(s) }))}
            value={season === null ? '' : String(season)}
            onChange={(value) => {
              setChosenSeason(Number(value));
              listRef.current?.scrollToSeason(Number(value));
            }}
          />
        ) : (
          <span />
        )}
        <FilterChip pressed={hideTaken} onPressedChange={onHideTaken}>
          Hide taken
        </FilterChip>
      </div>
    </>
  );
  const body =
    catalog.status === 'success' ? (
      <PickerList
        ref={listRef}
        rows={rows}
        virtual={episodeCount(seasons) > PICKER_VIRTUALIZE_ABOVE}
        disabled={proposal === null}
        query={query}
        startSeason={startSeason}
        onPick={onPick}
      />
    ) : (
      <CatalogState catalog={catalog} />
    );
  return { head, body, loaded: catalog.status === 'success' };
}

function CatalogState({ catalog }: { catalog: Exclude<PickerCatalog, { status: 'success' }> }) {
  if (catalog.status === 'error') {
    return (
      <div className="pk-state">
        <Banner
          tone="error"
          title="Couldn’t load the episodes"
          action={
            <Button size="sm" onClick={catalog.retry}>
              Try again
            </Button>
          }
        >
          {catalog.message}
        </Banner>
      </div>
    );
  }
  return (
    <div className="pk-state" role="status">
      <span className="sr-only">Loading episodes…</span>
      {Array.from({ length: 6 }, (_, i) => (
        <Skeleton key={i} height={44} />
      ))}
    </div>
  );
}

function PickerFor({ proposal, path, label }: { proposal: Proposal | null; path: string | null; label: string }) {
  return (
    <div className="pk-for">
      <span className="t-micro text-fg-3">{proposal ? label : 'Episodes'}</span>
      {proposal && path ? (
        <p className="pk-file">
          <FilePath path={path} />
        </p>
      ) : (
        <p className="t-small text-fg-2">Pick a file on the left to change its episode.</p>
      )}
    </div>
  );
}

// ---- Pane (desktop) --------------------------------------------------------------------------

/**
 * Desktop (≥ 1024 px): the sticky right-hand pane, linked to the selected
 * row. Key it by the selected proposal, so a new selection starts on its episode.
 */
export function EpisodePickerPane({ path, ...props }: PickerProps & { searchRef: RefObject<HTMLInputElement | null> }) {
  const { head, body } = usePickerParts(props);
  return (
    <aside className="map-pane" aria-label="Episode picker">
      <div className="picker">
        <div className="pk-head">
          <PickerFor proposal={props.proposal} path={path} label="Episode for" />
          {head}
        </div>
        {body}
        <p className="pk-legend t-small text-fg-3">
          <Info size={14} aria-hidden="true" />
          Click an episode to assign it to the selected file.
        </p>
      </div>
    </aside>
  );
}

// ---- Sheet (phone, tablet) -------------------------------------------------------------------

interface SheetProps extends PickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onIgnore: () => void;
}

/**
 * Phone and tablet: a tall sheet with a stable height. A plain sheet, not
 * nested: this page has no parent sheet. One sheet from loading to loaded
 * (only its body changes). Key it per opening, so the query starts empty and
 * the list opens on the file's episode.
 */
export function EpisodePickerSheet({ open, onOpenChange, onIgnore, path, ...props }: SheetProps) {
  const { head, body, loaded } = usePickerParts(props);
  const bodyHost = useRef<HTMLDivElement>(null);
  // Focus goes to the current option, not the search, so the keyboard doesn't cover the list
  // (once the list is there; not if you've started typing meanwhile)
  useLayoutEffect(() => {
    if (!open || !loaded) return;
    const timer = window.setTimeout(() => {
      if (document.activeElement instanceof HTMLInputElement) return;
      bodyHost.current?.querySelector<HTMLElement>('[role="listbox"]')?.focus({ preventScroll: true });
    }, 60);
    return () => window.clearTimeout(timer);
  }, [open, loaded]);
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title="Choose an episode"
      size="full"
      className="map-sheet"
      head={
        <>
          <PickerFor proposal={props.proposal} path={path} label="For" />
          {head}
        </>
      }
      footer={
        <>
          <Button variant="ghost" onClick={onIgnore}>
            <EyeOff size={17} aria-hidden="true" />
            Ignore file
          </Button>
          <span className="grow" />
          <Button onClick={() => onOpenChange(false)}>{props.proposal?.targets.length ? 'Keep current' : 'Not now'}</Button>
        </>
      }
    >
      <div ref={bodyHost} className="contents">
        {body}
      </div>
    </Sheet>
  );
}
