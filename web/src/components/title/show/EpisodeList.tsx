'use client';

import { type Ref, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState } from 'react';
import { useWindowVirtualizer } from '@tanstack/react-virtual';
import type { EpisodeOrdering, EpisodeSummary } from '@/lib/api/contract';
import { shouldVirtualize } from '@/lib/title/episodes';
import { EpisodeRow, type EpisodeRowProps, episodeRowId } from './EpisodeRow';

export interface ScrollToEpisodeOptions {
  /** Scroll it to the middle of the screen (default). Off when it's already where the user looks. */
  scroll?: boolean;
  /** A short amber flash (default). */
  flash?: boolean;
}

/** What the section can ask of the list. */
export interface EpisodeListHandle {
  /**
   * Bring an in-season episode into view, flash it and move focus to it, so
   * Tab carries on from that row; false when it isn't in the list.
   */
  scrollToEpisode: (episodeNumber: number, options?: ScrollToEpisodeOptions) => boolean;
}

interface EpisodeListProps extends Pick<EpisodeRowProps, 'onFind' | 'onOpen'> {
  /** Names the list, e.g. "Season 1 episodes". */
  label: string;
  episodes: EpisodeSummary[];
  ordering: EpisodeOrdering;
  nextUpId: number | null;
  reviewHref: string | null;
  /** "With stills" rows (92 px) instead of compact ones (60 px). */
  rich: boolean;
  ref?: Ref<EpisodeListHandle>;
}

const ROW_GAP = 2;
const FLASH_MS = 1600;

type RowProps = Omit<EpisodeListProps, 'label' | 'episodes' | 'ref'>;

function rowProps(props: RowProps, episode: EpisodeSummary, flashId: number | null) {
  return {
    episode,
    ordering: props.ordering,
    isNext: episode.id === props.nextUpId,
    reviewHref: props.reviewHref,
    onFind: props.onFind,
    onOpen: props.onOpen,
    rich: props.rich,
    flash: episode.id === flashId,
  };
}

const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

/**
 * Focus a row without scrolling to it again. A virtualised row may only
 * render a few frames after the scroll, so keep trying briefly.
 */
function focusRow(episode: EpisodeSummary, frames = 12) {
  const row = document.getElementById(episodeRowId(episode));
  if (row) row.focus({ preventScroll: true });
  else if (frames > 0) window.requestAnimationFrame(() => focusRow(episode, frames - 1));
}

/**
 * Rows above `VIRTUALIZE_ABOVE` (60) go through the window virtualiser: only
 * the rows near the viewport are in the DOM (with aria-setsize/posinset), so
 * a 1,000-episode season stays light. Rows have ids only while rendered.
 */
interface VirtualRowsProps extends RowProps {
  episodes: EpisodeSummary[];
  label: string;
  flashId: number | null;
  onFlash: (episodeId: number) => void;
  listRef: Ref<EpisodeListHandle> | undefined;
}

function VirtualRows({ episodes, label, flashId, onFlash, listRef, ...props }: VirtualRowsProps) {
  const ol = useRef<HTMLOListElement>(null);
  const [offset, setOffset] = useState(0);

  // The list's distance from the top of the page, which the window virtualiser needs
  useLayoutEffect(() => {
    const measure = () => {
      const top = (ol.current?.getBoundingClientRect().top ?? 0) + window.scrollY;
      setOffset((current) => (Math.abs(current - top) > 1 ? top : current));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, []);

  const virtualizer = useWindowVirtualizer({
    count: episodes.length,
    estimateSize: () => (props.rich ? 92 : 60),
    gap: ROW_GAP,
    overscan: 8,
    scrollMargin: offset,
  });

  useImperativeHandle(listRef, () => ({
    scrollToEpisode: (episodeNumber, { scroll = true, flash = true } = {}) => {
      const index = episodes.findIndex((episode) => episode.episode_number === episodeNumber);
      if (index < 0) return false;
      // Next frame: the list's offset (scrollMargin) is measured after the first render.
      // Instant, because smooth scrolling can't land precisely on rows of measured height.
      window.requestAnimationFrame(() => {
        if (scroll) virtualizer.scrollToIndex(index, { align: 'center' });
        focusRow(episodes[index]);
      });
      if (flash) onFlash(episodes[index].id);
      return true;
    },
  }));

  const items = virtualizer.getVirtualItems();
  return (
    <ol ref={ol} className="ep-list ep-list--virtual" aria-label={label} style={{ height: virtualizer.getTotalSize() }}>
      {items.map((item) => {
        const episode = episodes[item.index];
        return (
          <EpisodeRow
            key={episode.id}
            {...rowProps(props, episode, flashId)}
            ref={virtualizer.measureElement}
            position={{ index: item.index, of: episodes.length }}
            style={{ transform: `translateY(${item.start - virtualizer.options.scrollMargin}px)` }}
          />
        );
      })}
    </ol>
  );
}

/**
 * A season's episodes as rows (compact or with stills). Up to 60 rows render
 * plainly; longer seasons are virtualised. `scrollToEpisode` works either way.
 */
export function EpisodeList({ label, episodes, ref, ...props }: EpisodeListProps) {
  const [flashId, setFlashId] = useState<number | null>(null);
  useEffect(() => {
    if (flashId === null) return;
    const timer = window.setTimeout(() => setFlashId(null), FLASH_MS);
    return () => window.clearTimeout(timer);
  }, [flashId]);
  const flash = useCallback((id: number) => {
    // Restart the animation when the same row is flashed twice
    setFlashId(null);
    window.requestAnimationFrame(() => setFlashId(id));
  }, []);

  const virtual = shouldVirtualize(episodes.length);
  useImperativeHandle(
    virtual ? undefined : ref,
    () => ({
      scrollToEpisode: (episodeNumber, { scroll = true, flash: flashRow = true } = {}) => {
        const episode = episodes.find((e) => e.episode_number === episodeNumber);
        const row = episode && document.getElementById(episodeRowId(episode));
        if (!episode || !row) return false;
        if (scroll) row.scrollIntoView({ block: 'center', behavior: prefersReducedMotion() ? 'auto' : 'smooth' });
        row.focus({ preventScroll: true });
        if (flashRow) flash(episode.id);
        return true;
      },
    }),
    [episodes, flash]
  );

  if (virtual) return <VirtualRows episodes={episodes} label={label} flashId={flashId} onFlash={flash} listRef={ref} {...props} />;
  return (
    <ol className="ep-list" aria-label={label}>
      {episodes.map((episode) => (
        <EpisodeRow key={episode.id} {...rowProps(props, episode, flashId)} />
      ))}
    </ol>
  );
}
