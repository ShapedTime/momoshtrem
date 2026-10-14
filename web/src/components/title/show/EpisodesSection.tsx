import type { ReactNode } from 'react';
import { RefreshCw, Settings } from 'lucide-react';
import { Banner, Button, EmptyState, LoadingRegion, Skeleton } from '@/components/ui';
import type { EpisodeSummary, SeasonDetail, ShowTitle } from '@/lib/api/contract';
import { episodesSummary, nextUpEpisodeId, seasonLabel } from '@/lib/title/episodes';
import { BulkFind } from './BulkFind';
import { EpisodeList } from './EpisodeList';
import { NeedsReviewBanner } from './NeedsReviewBanner';
import { SeasonTabs } from './SeasonTabs';

/** The selected season's episodes, as the page's query has them. */
export type SeasonState =
  | { status: 'pending' }
  | { status: 'error'; message: string; retry: () => void }
  | { status: 'success'; season: SeasonDetail };

interface EpisodesSectionProps {
  show: ShowTitle;
  /** The selected season (`?season=`); null when the show has none. */
  seasonNumber: number | null;
  onSeasonChange: (seasonNumber: number) => void;
  /** Only read while the show is in the library. */
  season: SeasonState;
  onOpenSettings: () => void;
  onFindSeason: (seasonNumber: number) => void;
  onFindEpisode: (episode: EpisodeSummary) => void;
  onOpenEpisode: (episode: EpisodeSummary) => void;
  /** Between the tabs and the list: range chips, jump to episode and density (slice 6). */
  tools?: ReactNode;
}

const LIST_ID = 'season-episodes';
const PANEL_ID = 'season-panel';

function SeasonEpisodes({ show, season, onFindEpisode, onOpenEpisode }: Pick<EpisodesSectionProps, 'show' | 'season' | 'onFindEpisode' | 'onOpenEpisode'>) {
  if (season.status === 'pending') {
    return (
      <LoadingRegion label="Loading episodes" className="flex flex-col gap-0.5">
        {Array.from({ length: 8 }, (_, i) => (
          <Skeleton key={i} height={60} className="rounded-md" />
        ))}
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
  const { episodes, season_number: number } = season.season;
  if (!episodes.length) return <p className="t-small text-fg-3">TMDB lists no episodes for this season yet.</p>;
  const reviewJob = show.library.review_job_id;
  return (
    <EpisodeList
      id={LIST_ID}
      label={`${seasonLabel(season.season)} episodes`}
      episodes={episodes}
      ordering={show.episode_ordering}
      nextUpId={nextUpEpisodeId(show, number, episodes)}
      reviewHref={reviewJob === null ? null : `/mapping/${reviewJob}`}
      onFind={onFindEpisode}
      onOpen={onOpenEpisode}
    />
  );
}

/**
 * Episodes on the show page: what's ready, the review banner, season tabs
 * (Specials last), season-wide finds and the episode rows. Before the show
 * is in the library: the tabs and "Add it to see what's ready".
 */
export function EpisodesSection(props: EpisodesSectionProps) {
  const { show, seasonNumber, onSeasonChange, season, onOpenSettings, onFindSeason, onFindEpisode, tools } = props;
  const inLibrary = show.library.in_library;
  const summary = inLibrary ? episodesSummary(show) : null;
  const selected = show.seasons.find((s) => s.season_number === seasonNumber);
  const firstMissing = inLibrary && season.status === 'success' ? season.season.episodes.find((episode) => episode.state === 'missing') : undefined;

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
          {tools}
          <BulkFind
            missing={selected.missing}
            upcoming={selected.upcoming}
            onFindSeason={() => onFindSeason(selected.season_number)}
            onFindMissing={() => (firstMissing ? onFindEpisode(firstMissing) : onFindSeason(selected.season_number))}
          />
          <SeasonEpisodes {...props} />
        </div>
      ) : (
        <p className="t-small text-fg-3">TMDB lists no seasons for this show yet.</p>
      )}
    </section>
  );
}
