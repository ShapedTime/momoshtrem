'use client';

import { useParams } from 'next/navigation';
import { useState, useCallback, useEffect } from 'react';
import { useMovie } from '@/hooks/useTMDB';
import { useLibraryStatus } from '@/hooks/useLibrary';
import { useTorrentStatus } from '@/hooks/useTorrents';
import { TorrentSearchModal, TorrentInfoSection } from '@/components/torrent';
import { SubtitleSearchModal, SubtitleList } from '@/components/subtitle';
import { useSubtitles } from '@/hooks';
import type { TorrentSearchContext } from '@/types/jackett';
import type { SubtitleSearchContext } from '@/types/subtitle';
import type { LibraryStatus, LibraryMovie } from '@/types/momoshtrem';
import { formatRuntime, extractYear, formatReleaseDate } from '@/lib/utils';
import {
  MediaDetails,
  MediaDetailsSkeleton,
  CastCarousel,
  CastCarouselSkeleton,
} from '@/components/media';
import { Button, AlertCircleIcon, Breadcrumb } from '@/components/ui';

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <AlertCircleIcon size={48} className="mx-auto mb-4 text-text-muted" />
        <h2 className="text-xl font-semibold text-white mb-2">
          Unable to load movie
        </h2>
        <p className="text-text-secondary mb-6">{message}</p>
        <Button variant="secondary" onClick={() => window.history.back()}>
          Go Back
        </Button>
      </div>
    </div>
  );
}

function LoadingState() {
  return (
    <>
      <MediaDetailsSkeleton />
      <CastCarouselSkeleton />
    </>
  );
}

export default function MoviePage() {
  const params = useParams();
  const movieId = params.id ? parseInt(params.id as string, 10) : null;

  const { data: movie, isLoading, error } = useMovie(movieId);

  // Library status
  const {
    status: libraryStatus,
    libraryId,
    hasAssignment,
    refresh: refreshLibraryStatus,
  } = useLibraryStatus('movie', movieId || 0);

  // Local state for optimistic updates
  const [localLibraryStatus, setLocalLibraryStatus] = useState<LibraryStatus>('not_in_library');

  // Library movie data (for assignment details)
  const [libraryMovie, setLibraryMovie] = useState<LibraryMovie | null>(null);
  const [isUnassigning, setIsUnassigning] = useState(false);

  // Fetch torrent status for the assignment
  const { status: torrentStatus, refresh: refreshTorrentStatus } = useTorrentStatus(
    libraryMovie?.assignment?.info_hash
  );

  // Sync library status from hook
  useEffect(() => {
    setLocalLibraryStatus(libraryStatus);
  }, [libraryStatus]);

  // Fetch library movie data when we have an assignment
  useEffect(() => {
    const fetchLibraryMovie = async () => {
      if (!libraryId || !hasAssignment) {
        setLibraryMovie(null);
        return;
      }

      try {
        const res = await fetch(`/api/library/movies/${libraryId}`);
        if (res.ok) {
          const data = await res.json();
          setLibraryMovie(data);
        }
      } catch (err) {
        console.error('Failed to fetch library movie:', err);
      }
    };

    fetchLibraryMovie();
  }, [libraryId, hasAssignment]);

  // Handler for library status changes
  const handleLibraryStatusChange = useCallback((newStatus: LibraryStatus) => {
    setLocalLibraryStatus(newStatus);
    refreshLibraryStatus();
  }, [refreshLibraryStatus]);

  // Handler for unassigning torrent
  const handleUnassign = useCallback(async () => {
    if (!libraryId) return;

    setIsUnassigning(true);
    try {
      const res = await fetch(`/api/library/movies/${libraryId}/assign`, {
        method: 'DELETE',
      });

      if (res.ok) {
        setLibraryMovie(null);
        setLocalLibraryStatus('in_library');
        refreshLibraryStatus();
      }
    } catch (err) {
      console.error('Failed to unassign torrent:', err);
    } finally {
      setIsUnassigning(false);
    }
  }, [libraryId, refreshLibraryStatus]);

  // Torrent search modal state
  const [isTorrentModalOpen, setIsTorrentModalOpen] = useState(false);
  const [torrentContext, setTorrentContext] = useState<TorrentSearchContext | null>(null);

  // Subtitle modal state
  const [isSubtitleModalOpen, setIsSubtitleModalOpen] = useState(false);
  const [subtitleContext, setSubtitleContext] = useState<SubtitleSearchContext | null>(null);

  // Subtitles hook
  const {
    subtitles,
    isLoading: isLoadingSubtitles,
    deleteSubtitle,
  } = useSubtitles('movie', libraryId || undefined);

  // Handler for "Search Torrents" button
  const handleSearchTorrents = useCallback(() => {
    if (!movie || !movieId) return;

    const releaseYear = movie.releaseDate
      ? parseInt(movie.releaseDate.substring(0, 4), 10)
      : undefined;
    const query = releaseYear ? `${movie.title} ${releaseYear}` : movie.title;

    setTorrentContext({
      mediaType: 'movie',
      query,
      title: movie.title,
      tmdbId: movieId,
      year: releaseYear,
    });
    setIsTorrentModalOpen(true);
  }, [movie, movieId]);

  // Handler for torrent modal close (may need to refresh status)
  const handleTorrentModalClose = useCallback(() => {
    setIsTorrentModalOpen(false);
    refreshLibraryStatus();
  }, [refreshLibraryStatus]);

  // Handler for "Find Subtitles" button
  const handleFindSubtitles = useCallback(() => {
    if (!movie || !movieId || !libraryId) return;

    setSubtitleContext({
      mediaType: 'movie',
      tmdbId: movieId,
      title: movie.title,
      itemId: libraryId,
    });
    setIsSubtitleModalOpen(true);
  }, [movie, movieId, libraryId]);

  // Handler for subtitle modal close
  const handleSubtitleModalClose = useCallback(() => {
    setIsSubtitleModalOpen(false);
  }, []);

  // Invalid ID state
  if (!movieId || isNaN(movieId)) {
    return <ErrorState message="Invalid movie ID" />;
  }

  // Loading state
  if (isLoading) {
    return <LoadingState />;
  }

  // Error state
  if (error) {
    return <ErrorState message={error} />;
  }

  // No data state
  if (!movie) {
    return <ErrorState message="Movie not found" />;
  }

  // Format metadata
  const runtime = formatRuntime(movie.runtime);
  const releaseYear = extractYear(movie.releaseDate);
  const releaseDate = formatReleaseDate(movie.releaseDate);

  return (
    <>
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        items={[
          { label: 'Home', href: '/' },
          { label: 'Movies', href: '/movies' },
          { label: movie.title },
        ]}
      />

      {/* Movie Details Hero */}
      <MediaDetails
        title={movie.title}
        tagline={movie.tagline}
        overview={movie.overview}
        backdropPath={movie.backdropPath}
        posterPath={movie.posterPath}
        rating={movie.voteAverage}
        releaseYear={releaseYear}
        runtime={runtime}
        releaseDate={releaseDate}
        genres={movie.genres}
        mediaType="movie"
        tmdbId={movieId}
        libraryStatus={localLibraryStatus}
        onLibraryStatusChange={handleLibraryStatusChange}
        onSearchTorrents={handleSearchTorrents}
      />

      {/* Cast Carousel */}
      {movie.credits.cast.length > 0 && (
        <CastCarousel cast={movie.credits.cast} maxItems={10} />
      )}

      {/* Torrent Info Section (when movie has torrent assigned) */}
      {libraryMovie?.assignment && (
        <section className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8">
          <TorrentInfoSection
            assignment={libraryMovie.assignment}
            torrentStatus={torrentStatus}
            onUnassign={handleUnassign}
            isUnassigning={isUnassigning}
          />

          {/* Subtitles Section */}
          <div className="mt-6 p-4 bg-bg-secondary rounded-lg border border-border">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-white">Subtitles</h3>
              <Button
                variant="secondary"
                onClick={handleFindSubtitles}
              >
                Find Subtitles
              </Button>
            </div>
            <SubtitleList
              subtitles={subtitles}
              isLoading={isLoadingSubtitles}
              onDelete={deleteSubtitle}
            />
          </div>
        </section>
      )}

      {/* Torrent Search Modal */}
      {torrentContext && (
        <TorrentSearchModal
          isOpen={isTorrentModalOpen}
          onClose={handleTorrentModalClose}
          context={torrentContext}
        />
      )}

      {/* Subtitle Search Modal */}
      {subtitleContext && (
        <SubtitleSearchModal
          isOpen={isSubtitleModalOpen}
          onClose={handleSubtitleModalClose}
          context={subtitleContext}
        />
      )}
    </>
  );
}
