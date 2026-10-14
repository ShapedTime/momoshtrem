'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { useOverArt } from '@/components/shell/ShellContext';
import { NotFoundView } from '@/components/system/NotFoundView';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { openLegacySearch } from '@/hooks/useLegacySearch';
import { DOWNLOAD_POLL_MS, useTitle, useTitleActions } from '@/hooks/useTitle';
import type { EpisodeSummary, ShowTitle } from '@/lib/api/contract';
import { seasonQuery } from '@/lib/api/endpoints';
import { useSheet } from '@/lib/sheets/useSheet';
import { selectedSeason } from '@/lib/title/episodes';
import { titleFacts } from '@/lib/title/facts';
import { legacySearchContext } from '@/lib/title/legacy-search';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import { ActionDock } from './ActionDock';
import { CastRow } from './CastRow';
import { DetailsAside } from './DetailsAside';
import { LegacySearchHost } from './LegacySearchHost';
import { MoreLikeThis } from './MoreLikeThis';
import { PrimaryPanel } from './PrimaryPanel';
import { EpisodesSection, episodeSheetParams, type SeasonState } from './show';
import { TitleHero } from './TitleHero';
import { TitleMoreMenu } from './TitleMoreMenu';
import { isTitleNotFound, TitleDetailSkeleton, TitleLoadError } from './TitleStates';
import { TrailerCard } from './TrailerCard';
import { OnTraktCard } from './WatchState';

/** The selected season, polled while one of its episodes downloads. */
function useSeasonState(show: ShowTitle, seasonNumber: number | null): SeasonState {
  const query = useQuery({
    ...seasonQuery(show.tmdb_id, seasonNumber ?? 0),
    enabled: show.library.in_library && seasonNumber !== null,
    refetchInterval: (q) => (q.state.data?.episodes.some((episode) => episode.state === 'downloading') ? DOWNLOAD_POLL_MS : false),
  });
  if (query.isPending) return { status: 'pending' };
  if (query.isError) return { status: 'error', message: query.error.message, retry: () => void query.refetch() };
  return { status: 'success', season: query.data };
}

function ShowDetailView({ show }: { show: ShowTitle }) {
  const actions = useTitleActions(show);
  const sheets = useSheet();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const seasonNumber = selectedSeason(searchParams.get('season'), show);
  const season = useSeasonState(show, seasonNumber);
  const copy = primaryActionFor(show);
  const { library } = show;
  useDocumentTitle(show.title);

  const changeSeason = (next: number) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set('season', String(next));
    router.replace(`${pathname}?${params}`, { scroll: false });
  };
  const findEpisode = (episode: EpisodeSummary) =>
    openLegacySearch(legacySearchContext(show, { season: episode.season_number, episode: episode.episode_number }));

  const onAction = (intent: StageIntent) => {
    if (intent === 'add') actions.add();
    else if (intent === 'how_to_play') sheets.open('infuse-setup', infuseSetupParams({ path: library.webdav_path, url: library.webdav_url }));
    else if (intent === 'find') {
      // One missing episode: search for it; a run: its season; nothing yet: the show
      const { missing } = library;
      const scope = !library.episodes_ready || !missing ? {} : missing.count === 1 ? { season: missing.season_number, episode: missing.from } : { season: missing.season_number };
      openLegacySearch(legacySearchContext(show, scope));
    }
  };

  return (
    <>
      <TitleHero
        title={show}
        panel={<PrimaryPanel copy={copy} onAction={onAction} busy={actions.adding ? 'add' : null} titleName={show.title} />}
        renderMore={(placement) => <TitleMoreMenu title={show} placement={placement} onRefresh={actions.refresh} onRemove={actions.remove} />}
      />
      <div className="page detail-body">
        <div className="page-sections">
          <EpisodesSection
            show={show}
            seasonNumber={seasonNumber}
            onSeasonChange={changeSeason}
            season={season}
            onOpenSettings={() => sheets.open('show-settings', { tmdb: String(show.tmdb_id) })}
            onFindSeason={(number) => openLegacySearch(legacySearchContext(show, { season: number }))}
            onFindEpisode={findEpisode}
            onOpenEpisode={(episode) => sheets.open('episode', episodeSheetParams(show, episode))}
          />
          <CastRow cast={show.cast} />
          <TrailerCard youtubeKey={show.trailer_youtube_key} backdropPath={show.backdrop_path} titleName={show.title} />
          <MoreLikeThis items={show.similar} />
        </div>
        <DetailsAside facts={titleFacts(show)}>
          <OnTraktCard state={show.watch_state} />
        </DetailsAside>
      </div>
      <ActionDock copy={copy} onAction={onAction} busy={actions.adding ? 'add' : null} />
      <LegacySearchHost onAdded={() => void actions.torrentChanged()} onClose={() => void actions.refetch()} />
    </>
  );
}

/**
 * A show's page (PLAN F2): where it stands (the same four stages as a
 * movie, with "Find episodes 29–38" when a run is missing), then its
 * episodes season by season, cast, trailer and more like it.
 */
export function ShowDetail({ tmdbId }: { tmdbId: number }) {
  const query = useTitle('tv', tmdbId);
  const show = query.data?.media_type === 'tv' ? query.data : undefined;
  useOverArt(query.isPending || show !== undefined);

  if (show) return <ShowDetailView show={show} />;
  if (query.isPending) return <TitleDetailSkeleton />;
  if (query.data || isTitleNotFound(query.error)) return <NotFoundView />;
  return <TitleLoadError error={query.error} noun="show" onRetry={() => void query.refetch()} />;
}
