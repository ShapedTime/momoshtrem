'use client';

import { NotFoundView } from '@/components/system/NotFoundView';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { useOverArt } from '@/components/shell/ShellContext';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { useHDRezkaEnabled } from '@/hooks/useHDRezka';
import { useItemSubtitles } from '@/hooks/useItemSubtitles';
import { openLegacySearch } from '@/hooks/useLegacySearch';
import { useTitle, useTitleActions } from '@/hooks/useTitle';
import type { MovieTitle } from '@/lib/api/contract';
import { useSheet } from '@/lib/sheets/useSheet';
import { titleFacts } from '@/lib/title/facts';
import { legacySearchContext } from '@/lib/title/legacy-search';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import { ActionDock } from './ActionDock';
import { CastRow } from './CastRow';
import { CurrentStreamPanel } from './CurrentStreamPanel';
import { CurrentTorrentPanel } from './CurrentTorrentPanel';
import { DetailsAside } from './DetailsAside';
import { LegacySearchHost } from './LegacySearchHost';
import { MoreLikeThis } from './MoreLikeThis';
import { PrimaryPanel } from './PrimaryPanel';
import { NeedsReviewBanner } from './show';
import { SubtitlesPanel } from './SubtitlesPanel';
import { TitleHero } from './TitleHero';
import { TitleMoreMenu } from './TitleMoreMenu';
import { isTitleNotFound, TitleDetailSkeleton, TitleLoadError } from './TitleStates';
import { TrailerCard } from './TrailerCard';
import { OnTraktCard } from './WatchState';

function MovieDetailView({ movie }: { movie: MovieTitle }) {
  const actions = useTitleActions(movie);
  const sheets = useSheet();
  const hdrezka = useHDRezkaEnabled();
  const { library } = movie;
  const ready = library.availability_stage === 'ready';
  // A new file (another torrent, or a stream) re-grades subtitle matches
  const fileKey = library.torrent?.info_hash ?? library.stream?.added_at ?? null;
  const subtitles = useItemSubtitles('movie', ready ? library.library_id : null, fileKey);
  const copy = primaryActionFor(movie, { hdrezka });
  useDocumentTitle(movie.title);

  const findTorrent = () => openLegacySearch(legacySearchContext(movie));
  const streamFromHDRezka = () => sheets.open('hdrezka', { movie: String(movie.tmdb_id) });
  const onAction = (intent: StageIntent) => {
    if (intent === 'add') actions.add();
    else if (intent === 'find') findTorrent();
    else if (intent === 'hdrezka') streamFromHDRezka();
    else if (intent === 'how_to_play') sheets.open('infuse-setup', infuseSetupParams({ path: library.webdav_path, url: library.webdav_url }));
  };
  const busy = actions.adding ? 'add' : null;

  return (
    <>
      <TitleHero
        title={movie}
        panel={<PrimaryPanel copy={copy} onAction={onAction} busy={busy} titleName={movie.title} />}
        renderMore={(placement) => (
          <TitleMoreMenu
            title={movie}
            placement={placement}
            onRefresh={actions.refresh}
            onRemove={actions.remove}
            onStreamFromHDRezka={hdrezka ? streamFromHDRezka : undefined}
          />
        )}
      />
      <div className="page detail-body">
        <div className="page-sections">
          {/* A torrent with several plausible files waits for a pick (#5) */}
          {library.in_library && <NeedsReviewBanner movie count={library.needs_review} jobId={library.review_job_id} />}
          {/* Once it's ready, what Infuse plays from comes first (HANDOFF §5.3) */}
          {ready && library.torrent && (
            <CurrentTorrentPanel torrent={library.torrent} onReplace={findTorrent} onStreamFromHDRezka={hdrezka ? streamFromHDRezka : undefined} />
          )}
          {ready && library.stream && (
            <CurrentStreamPanel stream={library.stream} onChange={hdrezka ? streamFromHDRezka : undefined} onReplaceWithTorrent={findTorrent} />
          )}
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
      <LegacySearchHost onAdded={() => void actions.torrentChanged()} onClose={() => void actions.refetch()} />
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
