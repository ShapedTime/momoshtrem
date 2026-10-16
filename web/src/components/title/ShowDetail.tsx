'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { useOverArt } from '@/components/shell/ShellContext';
import { NotFoundView } from '@/components/system/NotFoundView';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { useHDRezkaEnabled } from '@/hooks/useHDRezka';
import { DOWNLOAD_POLL_MS, useTitle, useTitleActions } from '@/hooks/useTitle';
import type { EpisodeSummary, ShowTitle } from '@/lib/api/contract';
import { seasonQuery, titleKeys } from '@/lib/api/endpoints';
import { useSheet } from '@/lib/sheets/useSheet';
import { showSourceParams } from '@/lib/source/context';
import { type JumpDestination, selectedSeason } from '@/lib/title/episodes';
import { titleFacts } from '@/lib/title/facts';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import { ActionDock } from './ActionDock';
import { CastRow } from './CastRow';
import { DetailsAside } from './DetailsAside';
import { MoreLikeThis } from './MoreLikeThis';
import { PrimaryPanel } from './PrimaryPanel';
import { type EpisodeView, EpisodesSection, episodeSheetParams, type SeasonState } from './show';
import { TitleHero } from './TitleHero';
import { TitleMoreMenu } from './TitleMoreMenu';
import { isTitleNotFound, TitleDetailSkeleton, TitleLoadError } from './TitleStates';
import { TrailerCard } from './TrailerCard';
import { OnTraktCard } from './WatchState';

/** `?ep=` (an in-season number) or `?abs=` (an absolute one) for the selected season: where a cross-season jump lands. */
function jumpFromUrl(params: URLSearchParams, season: number | null): JumpDestination | null {
  const digits = (key: string) => {
    const value = params.get(key);
    return value !== null && /^\d+$/.test(value) ? Number(value) : null;
  };
  if (season === null) return null;
  const absolute = digits('abs');
  if (absolute !== null) return { season, absolute };
  const episode = digits('ep');
  return episode === null ? null : { season, episode };
}

/**
 * The selected season, polled while one of its episodes downloads. When a
 * poll sees an episode change state (a download finished), the show is
 * refetched too, so the panel, header and tab counts keep up.
 */
function useSeasonState(show: ShowTitle, seasonNumber: number | null): SeasonState {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...seasonQuery(show.tmdb_id, seasonNumber ?? 0),
    enabled: show.library.in_library && seasonNumber !== null,
    refetchInterval: (q) => (q.state.data?.episodes.some((episode) => episode.state === 'downloading') ? DOWNLOAD_POLL_MS : false),
  });
  const states = query.data ? `${query.data.season_number}:${query.data.episodes.map((episode) => episode.state).join(',')}` : null;
  const previous = useRef(states);
  useEffect(() => {
    const before = previous.current;
    previous.current = states;
    if (!before || !states || before === states || before.split(':')[0] !== states.split(':')[0]) return;
    // Exact: the season itself just loaded
    void queryClient.invalidateQueries({ queryKey: titleKeys.detail('tv', show.tmdb_id), exact: true });
  }, [states, queryClient, show.tmdb_id]);
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
  const hdrezka = useHDRezkaEnabled();
  const copy = primaryActionFor(show, { hdrezka });
  const { library } = show;
  useDocumentTitle(show.title);

  const view: EpisodeView = searchParams.get('view') === 'rich' ? 'rich' : 'compact';
  const initialTarget = jumpFromUrl(searchParams, seasonNumber);

  /** Replace some query params (null removes one), keeping the rest and the scroll position. */
  const setParams = useCallback(
    (changes: Record<string, string | null>) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const [key, value] of Object.entries(changes)) {
        if (value === null) params.delete(key);
        else params.set(key, value);
      }
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );
  const changeSeason = (next: number) => setParams({ season: String(next), ep: null, abs: null });

  // `?ep=`/`?abs=` are one-offs: once their season shows (and scrolled there), drop them so a refetch doesn't
  // scroll again. Through the History API, which Next keeps in sync, so no navigation is in flight meanwhile.
  const seasonShown = season.status === 'success';
  const hasTarget = initialTarget !== null;
  useEffect(() => {
    if (!hasTarget || !seasonShown) return;
    const url = new URL(window.location.href);
    url.searchParams.delete('ep');
    url.searchParams.delete('abs');
    window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
  }, [hasTarget, seasonShown]);
  // The source sheet's tabs, on the same params: the show, a season, or one episode
  const findTorrent = (scope: { season?: number; episode?: number }) => sheets.open('torrent', showSourceParams(show.tmdb_id, scope));
  const findEpisode = (episode: EpisodeSummary) => findTorrent({ season: episode.season_number, episode: episode.episode_number });
  const streamFromHDRezka = (scope: { season?: number; episode?: number }) => sheets.open('hdrezka', showSourceParams(show.tmdb_id, scope));
  const onAction = (intent: StageIntent) => {
    // One missing episode: that episode; a run: its season; nothing yet: the show
    const { missing } = library;
    const scope = !library.episodes_ready || !missing ? {} : missing.count === 1 ? { season: missing.season_number, episode: missing.from } : { season: missing.season_number };
    if (intent === 'add') actions.add();
    else if (intent === 'how_to_play') sheets.open('infuse-setup', infuseSetupParams({ path: library.webdav_path, url: library.webdav_url }));
    else if (intent === 'find') findTorrent(scope);
    else if (intent === 'hdrezka') streamFromHDRezka(scope);
  };

  return (
    <>
      <TitleHero
        title={show}
        panel={<PrimaryPanel copy={copy} onAction={onAction} busy={actions.adding ? 'add' : null} titleName={show.title} />}
        renderMore={(placement) => (
          <TitleMoreMenu
            title={show}
            placement={placement}
            onRefresh={actions.refresh}
            onRemove={actions.remove}
            onStreamFromHDRezka={hdrezka ? () => streamFromHDRezka(seasonNumber === null ? {} : { season: seasonNumber }) : undefined}
          />
        )}
      />
      <div className="page detail-body">
        <div className="page-sections">
          <EpisodesSection
            show={show}
            seasonNumber={seasonNumber}
            onSeasonChange={changeSeason}
            season={season}
            onOpenSettings={() => sheets.open('show-settings', { tmdb: String(show.tmdb_id) })}
            onFindSeason={(number) => findTorrent({ season: number })}
            onStreamSeason={hdrezka ? (number) => streamFromHDRezka({ season: number }) : undefined}
            onFindEpisode={findEpisode}
            onOpenEpisode={(episode) => sheets.open('episode', episodeSheetParams(show, episode))}
            view={view}
            onViewChange={(next) => setParams({ view: next === 'rich' ? 'rich' : null })}
            onJumpToSeason={(destination) =>
              setParams({
                season: String(destination.season),
                ep: destination.episode === undefined ? null : String(destination.episode),
                abs: destination.absolute === undefined ? null : String(destination.absolute),
              })
            }
            initialTarget={initialTarget}
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
