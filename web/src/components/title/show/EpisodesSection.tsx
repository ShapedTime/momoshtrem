'use client';

import { type Ref, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { List, RefreshCw, Rows3, Settings } from 'lucide-react';
import { Banner, Button, EmptyState, LoadingRegion, SegmentedControl, Skeleton } from '@/components/ui';
import type { EpisodeSummary, SeasonDetail, SeasonSummary, ShowTitle } from '@/lib/api/contract';
import {
  defaultRangeIndex,
  earlierWatched,
  episodesSummary,
  type JumpDestination,
  LONG_SEASON,
  nextUpEpisodeId,
  rangeIndexFor,
  resolveJump,
  seasonLabel,
} from '@/lib/title/episodes';
import { keepOf } from '@/lib/title/keep';
import { downloadsAvailable } from '@/lib/title/keep-sheet';
import { BulkFind, type SeasonKeep } from './BulkFind';
import { EarlierWatchedRow } from './EarlierWatchedRow';
import { EpisodeList, type EpisodeListHandle, type ScrollToEpisodeOptions } from './EpisodeList';
import { JumpToEpisode } from './JumpToEpisode';
import { NeedsReviewBanner } from './NeedsReviewBanner';
import { RangeChips } from './RangeChips';
import { SeasonTabs } from './SeasonTabs';

/** The selected season's episodes, as the page's query has them. */
export type SeasonState =
  | { status: 'pending' }
  | { status: 'error'; message: string; retry: () => void }
  | { status: 'success'; season: SeasonDetail };

/** Compact 60 px rows, or rows with stills (`?view=rich`). */
export type EpisodeView = 'compact' | 'rich';

interface EpisodesSectionProps {
  show: ShowTitle;
  /** The selected season (`?season=`); null when the show has none. */
  seasonNumber: number | null;
  onSeasonChange: (seasonNumber: number) => void;
  /** Only read while the show is in the library. */
  season: SeasonState;
  onOpenSettings: () => void;
  onFindSeason: (seasonNumber: number) => void;
  /** Opens HDRezka on a season; left out while HDRezka is off. */
  onStreamSeason?: (seasonNumber: number) => void;
  /** Opens the Keep sheet (#206) on a season, or on the show when its own download holds every season. */
  onKeep: (scope: { season?: number }) => void;
  onFindEpisode: (episode: EpisodeSummary) => void;
  onOpenEpisode: (episode: EpisodeSummary) => void;
  view: EpisodeView;
  onViewChange: (view: EpisodeView) => void;
  /** Jump to episode found it in another season: switch there (`?season=&ep=` or `&abs=`). */
  onJumpToSeason: (destination: JumpDestination) => void;
  /** Where to scroll once the selected season shows (`?ep=` or `?abs=`). */
  initialTarget: JumpDestination | null;
  /** Scroll to an episode of the selected season and flash it. */
  ref?: Ref<EpisodeListHandle>;
}

const PANEL_ID = 'season-panel';

const VIEWS = [
  { value: 'compact' as const, label: <List size={16} aria-hidden="true" />, ariaLabel: 'Compact' },
  { value: 'rich' as const, label: <Rows3 size={16} aria-hidden="true" />, ariaLabel: 'With stills' },
];

interface SeasonBodyProps extends Omit<EpisodesSectionProps, 'season' | 'seasonNumber' | 'onSeasonChange' | 'onOpenSettings'> {
  season: SeasonDetail;
  summary: SeasonSummary;
}

/**
 * One loaded season: range chips, jump to episode and density, the
 * season-wide finds, the collapsed watched run and the rows. Keyed by season,
 * so its state (expanded, pressed chip) starts over on another tab.
 */
function SeasonBody(props: SeasonBodyProps) {
  const { show, season, summary, view, onViewChange, onJumpToSeason, initialTarget, onFindSeason, onStreamSeason, onKeep, onFindEpisode, onOpenEpisode, ref } = props;
  const { episodes, ranges, season_number: number } = season;
  const list = useRef<EpisodeListHandle>(null);
  // Trakt's next episode, else the frontier's first; the watched run never folds it away
  const nextUpId = nextUpEpisodeId(show, number, episodes);
  const collapsed = earlierWatched(episodes, nextUpId);
  const initialEpisode = initialTarget ? resolveJump(initialTarget, episodes) : null;

  const [expanded, setExpanded] = useState(() => initialEpisode !== null && collapsed !== null && initialEpisode <= collapsed.to);
  const [active, setActive] = useState(() => (initialEpisode !== null ? rangeIndexFor(ranges, initialEpisode) : defaultRangeIndex(ranges, show, number)));
  // Scroll and focus after the render that shows the row (expanding the watched run adds rows)
  const [request, setRequest] = useState<{ episode: number; options?: ScrollToEpisodeOptions; at: number } | null>(() =>
    initialEpisode !== null ? { episode: initialEpisode, at: 0 } : null
  );
  useEffect(() => {
    if (request) list.current?.scrollToEpisode(request.episode, request.options);
  }, [request]);

  const jumpTo = (episodeNumber: number, options?: ScrollToEpisodeOptions): boolean => {
    if (!episodes.some((episode) => episode.episode_number === episodeNumber)) return false;
    if (collapsed && episodeNumber <= collapsed.to) setExpanded(true);
    setActive(rangeIndexFor(ranges, episodeNumber));
    setRequest({ episode: episodeNumber, options, at: Date.now() });
    return true;
  };
  useImperativeHandle(ref, () => ({ scrollToEpisode: jumpTo }));

  /** Jump to episode: this season resolves the row (absolute numbers by lookup); another season goes through the URL. */
  const jump = (destination: JumpDestination): string | null => {
    if (destination.season !== number) {
      onJumpToSeason(destination);
      return null;
    }
    const episodeNumber = resolveJump(destination, episodes);
    if (episodeNumber !== null && jumpTo(episodeNumber)) return null;
    return `${seasonLabel(season)} has no episode #${destination.absolute ?? destination.episode}.`;
  };

  const visible = collapsed && !expanded ? episodes.slice(collapsed.count) : episodes;
  const firstMissing = episodes.find((episode) => episode.state === 'missing');
  const reviewJob = show.library.review_job_id;
  const long = episodes.length > LONG_SEASON || show.episode_ordering === 'absolute';
  // The season's own keep; else a show keep holds it; else Download & keep makes a season one.
  // A backend without downloads (no `keep` on the season) offers none.
  const ownKeep = keepOf(season.keep);
  const showKeep = keepOf(show.library.keep);
  const keepScope = !ownKeep && showKeep?.reason === 'kept' ? 'show' : 'season';
  const seasonKeep: SeasonKeep | undefined = downloadsAvailable(season)
    ? { keep: keepScope === 'show' ? showKeep : ownKeep, scope: keepScope, onKeep: () => onKeep(keepScope === 'show' ? {} : { season: number }) }
    : undefined;

  return (
    <>
      <div className="ep-tools">
        <RangeChips ranges={ranges} ordering={show.episode_ordering} active={active} onSelect={(range) => jumpTo(range.from)} />
        <div className="ep-tools-right">
          {long && (
            <JumpToEpisode
              context={{ ordering: show.episode_ordering, seasons: show.seasons, currentSeason: number }}
              onJump={jump}
            />
          )}
          <SegmentedControl aria-label="Episode list density" options={VIEWS} value={view} onChange={onViewChange} />
        </div>
      </div>
      <BulkFind
        missing={summary.missing}
        upcoming={summary.upcoming}
        onFindSeason={() => onFindSeason(number)}
        onFindMissing={() => (firstMissing ? onFindEpisode(firstMissing) : onFindSeason(number))}
        onStreamSeason={onStreamSeason && (() => onStreamSeason(number))}
        keep={seasonKeep}
      />
      {episodes.length === 0 ? (
        <p className="t-small text-fg-3">TMDB lists no episodes for this season yet.</p>
      ) : (
        <div className="flex flex-col gap-0.5">
          {collapsed && !expanded && (
            // The button goes away: focus the first row it brought back, where it was
            <EarlierWatchedRow from={collapsed.from} to={collapsed.to} onExpand={() => jumpTo(collapsed.from, { scroll: false, flash: false })} />
          )}
          <EpisodeList
            ref={list}
            label={`${seasonLabel(season)} episodes`}
            episodes={visible}
            ordering={show.episode_ordering}
            nextUpId={nextUpId}
            reviewHref={reviewJob === null ? null : `/mapping/${reviewJob}`}
            rich={view === 'rich'}
            onFind={onFindEpisode}
            onOpen={onOpenEpisode}
          />
        </div>
      )}
    </>
  );
}

function SeasonContent({ season, ...props }: Omit<SeasonBodyProps, 'season'> & { season: SeasonState }) {
  if (season.status === 'pending') {
    // Same boxes as the loaded season: the tools row, the finds and rows of this density
    return (
      <LoadingRegion label="Loading episodes" className="flex flex-col gap-4">
        <Skeleton height={44} className="rounded-md" />
        <Skeleton height={36} width={280} className="max-w-full rounded-sm" />
        <div className="flex flex-col gap-0.5">
          {Array.from({ length: 8 }, (_, i) => (
            <Skeleton key={i} height={props.view === 'rich' ? 92 : 60} className="rounded-md" />
          ))}
        </div>
      </LoadingRegion>
    );
  }
  if (season.status === 'error') {
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load these episodes"
        action={
          <Button size="sm" onClick={season.retry}>
            <RefreshCw size={15} aria-hidden="true" /> Try again
          </Button>
        }
      >
        {season.message} The rest of the page is fine.
      </Banner>
    );
  }
  return <SeasonBody key={season.season.season_number} season={season.season} {...props} />;
}

/**
 * Episodes on the show page: what's ready, the review banner, season tabs
 * (Specials last), long-season tools, season-wide finds and the episode rows.
 * Before the show is in the library: the tabs and "Add it to see what's ready".
 */
export function EpisodesSection({ seasonNumber, onSeasonChange, season, onOpenSettings, ...props }: EpisodesSectionProps) {
  const { show } = props;
  const inLibrary = show.library.in_library;
  const summary = inLibrary ? episodesSummary(show) : null;
  const selected = show.seasons.find((s) => s.season_number === seasonNumber);

  return (
    <section className="section" aria-labelledby="episodes-h">
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-2" id="episodes-h">
            Episodes
          </h2>
          {summary && <p className="t-small text-fg-3">{summary}</p>}
        </div>
        {inLibrary && (
          <div className="actions">
            <Button variant="ghost" size="sm" onClick={onOpenSettings} aria-haspopup="dialog">
              <Settings size={16} aria-hidden="true" />
              <span className="max-sm:sr-only">Show settings</span>
            </Button>
          </div>
        )}
      </div>
      {inLibrary && <NeedsReviewBanner count={show.library.needs_review} jobId={show.library.review_job_id} />}
      {selected && <SeasonTabs seasons={show.seasons} value={selected.season_number} onChange={onSeasonChange} controls={inLibrary ? PANEL_ID : undefined} />}
      {!inLibrary ? (
        <EmptyState cat="peek" title="Add it to see what’s ready">
          Once it’s in your library, each episode shows whether it’s ready, downloading or missing.
        </EmptyState>
      ) : selected ? (
        <div role="tabpanel" id={PANEL_ID} aria-label={seasonLabel(selected)} className="flex flex-col gap-4">
          <SeasonContent season={season} summary={selected} {...props} />
        </div>
      ) : (
        <p className="t-small text-fg-3">TMDB lists no seasons for this show yet.</p>
      )}
    </section>
  );
}
