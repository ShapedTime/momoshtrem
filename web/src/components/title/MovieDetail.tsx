'use client';

import { useSearchParams } from 'next/navigation';
import { useEffect, useState } from 'react';
import { NotFoundView } from '@/components/system/NotFoundView';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { useOverArt } from '@/components/shell/ShellContext';
import { TorrentSearchModal } from '@/components/torrent';
import { useItemSubtitles } from '@/hooks/useItemSubtitles';
import { useTitle, useTitleActions } from '@/hooks/useTitle';
import type { MovieTitle } from '@/lib/api/contract';
import { useSheet } from '@/lib/sheets/useSheet';
import { titleFacts } from '@/lib/title/facts';
import { legacySearchContext } from '@/lib/title/legacy-search';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import type { TorrentSearchContext } from '@/types/jackett';
import { ActionDock } from './ActionDock';
import { CastRow } from './CastRow';
import { CurrentTorrentPanel } from './CurrentTorrentPanel';
import { DetailsAside } from './DetailsAside';
import { MoreLikeThis } from './MoreLikeThis';
import { PrimaryPanel } from './PrimaryPanel';
import { SubtitlesPanel } from './SubtitlesPanel';
import { TitleHero } from './TitleHero';
import { TitleMoreMenu } from './TitleMoreMenu';
import { isTitleNotFound, TitleDetailSkeleton, TitleLoadError } from './TitleStates';
import { TrailerCard } from './TrailerCard';
import { OnTraktCard } from './WatchState';

/**
 * The browser tab names the title (the route's metadata only knows "Movie").
 * Next re-applies that metadata on every navigation, sheets' `?sheet=`
 * included, so this runs again whenever the URL's query changes.
 */
function useDocumentTitle(name: string) {
  const searchParams = useSearchParams();
  useEffect(() => {
    document.title = `${name} · momoshtrem`;
  }, [name, searchParams]);
}

function MovieDetailView({ movie }: { movie: MovieTitle }) {
  const actions = useTitleActions(movie);
  const sheets = useSheet();
  const [search, setSearch] = useState<TorrentSearchContext | null>(null);
  const { library } = movie;
  const ready = library.availability_stage === 'ready';
  const subtitles = useItemSubtitles('movie', ready ? library.library_id : null, library.torrent?.info_hash ?? null);
  const copy = primaryActionFor(movie);
  useDocumentTitle(movie.title);

  const findTorrent = () => setSearch(legacySearchContext(movie));
  const onAction = (intent: StageIntent) => {
    if (intent === 'add') actions.add();
    else if (intent === 'find') findTorrent();
    else if (intent === 'how_to_play') sheets.open('infuse-setup', infuseSetupParams({ path: library.webdav_path, url: library.webdav_url }));
  };
  const busy = actions.adding ? 'add' : null;

  return (
    <>
      <TitleHero
        title={movie}
        panel={<PrimaryPanel copy={copy} onAction={onAction} busy={busy} titleName={movie.title} />}
        renderMore={(placement) => (
          <TitleMoreMenu title={movie} placement={placement} onRefresh={actions.refresh} onRemove={actions.remove} />
        )}
      />
      <div className="page detail-body">
        <div className="page-sections">
          {/* Once it's ready, what Infuse plays from comes first (HANDOFF §5.3) */}
          {ready && library.torrent && <CurrentTorrentPanel torrent={library.torrent} onReplace={findTorrent} />}
          {ready && <SubtitlesPanel {...subtitles} />}
          <CastRow cast={movie.cast} />
          <TrailerCard youtubeKey={movie.trailer_youtube_key} backdropPath={movie.backdrop_path} titleName={movie.title} />
          <MoreLikeThis items={movie.similar} />
        </div>
        <DetailsAside facts={titleFacts(movie)}>
          <OnTraktCard state={movie.watch_state} />
        </DetailsAside>
      </div>
      <ActionDock copy={copy} onAction={onAction} busy={busy} />
      {/* The legacy search until the torrent search sheet (#6) */}
      {search && (
        <TorrentSearchModal
          isOpen
          context={search}
          onClose={() => {
            setSearch(null);
            void actions.refetch();
          }}
          onAdded={() => void actions.torrentChanged()}
        />
      )}
    </>
  );
}

/**
 * A movie's page (PLAN F2): where it is (not in library → needs a torrent →
 * downloading → ready in Infuse) and the one next thing to do, then the
 * torrent and subtitles once it's ready, cast, trailer and more like it.
 */
export function MovieDetail({ tmdbId }: { tmdbId: number }) {
  const query = useTitle('movie', tmdbId);
  const movie = query.data?.media_type === 'movie' ? query.data : undefined;
  useOverArt(query.isPending || movie !== undefined);

  if (movie) return <MovieDetailView movie={movie} />;
  if (query.isPending) return <TitleDetailSkeleton />;
  if (query.data || isTitleNotFound(query.error)) return <NotFoundView />;
  return <TitleLoadError error={query.error} noun="movie" onRetry={() => void query.refetch()} />;
}
