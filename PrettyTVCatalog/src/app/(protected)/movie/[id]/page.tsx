'use client';

import { useParams } from 'next/navigation';
import { useState, useCallback } from 'react';
import { useMovie } from '@/hooks/useTMDB';
import { TorrentSearchModal } from '@/components/torrent';
import type { TorrentSearchContext } from '@/types/jackett';
import { formatRuntime, extractYear, formatReleaseDate } from '@/lib/utils';
import {
  MediaDetails,
  MediaDetailsSkeleton,
  CastCarousel,
  CastCarouselSkeleton,
} from '@/components/media';
import { Button, AlertCircleIcon } from '@/components/ui';

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

  // Torrent search modal state
  const [isTorrentModalOpen, setIsTorrentModalOpen] = useState(false);
  const [torrentContext, setTorrentContext] = useState<TorrentSearchContext | null>(null);

  // Handler for "Search Torrents" button
  const handleSearchTorrents = useCallback(() => {
    if (!movie) return;

    const releaseYear = movie.releaseDate
      ? parseInt(movie.releaseDate.substring(0, 4), 10)
      : undefined;
    const query = releaseYear ? `${movie.title} ${releaseYear}` : movie.title;

    setTorrentContext({
      mediaType: 'movie',
      query,
      title: movie.title,
      year: releaseYear,
    });
    setIsTorrentModalOpen(true);
  }, [movie]);

  // Handler for adding torrent (placeholder for Task 10)
  const handleAddTorrent = useCallback((magnetUri: string) => {
    console.log('Add torrent:', magnetUri);
    // Task 10 will implement distribyted integration
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
        onSearchTorrents={handleSearchTorrents}
      />

      {/* Cast Carousel */}
      {movie.credits.cast.length > 0 && (
        <CastCarousel cast={movie.credits.cast} maxItems={10} />
      )}

      {/* Torrent Search Modal */}
      {torrentContext && (
        <TorrentSearchModal
          isOpen={isTorrentModalOpen}
          onClose={() => setIsTorrentModalOpen(false)}
          context={torrentContext}
          onAddTorrent={handleAddTorrent}
        />
      )}
    </>
  );
}
