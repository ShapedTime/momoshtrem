'use client';

import { useParams } from 'next/navigation';
import { useState, useEffect, useCallback } from 'react';
import { useTVShow, useSeason } from '@/hooks/useTMDB';
import { TorrentSearchModal } from '@/components/torrent';
import type { TorrentSearchContext } from '@/types/jackett';
import { extractYear, formatReleaseDate } from '@/lib/utils';
import {
  MediaDetails,
  MediaDetailsSkeleton,
  CastCarousel,
  CastCarouselSkeleton,
  SeasonPicker,
  SeasonPickerSkeleton,
  EpisodeList,
  EpisodeListSkeleton,
} from '@/components/media';
import { Button, AlertCircleIcon } from '@/components/ui';

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <AlertCircleIcon size={48} className="mx-auto mb-4 text-text-muted" />
        <h2 className="text-xl font-semibold text-white mb-2">
          Unable to load TV show
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
      <div className="px-4 sm:px-6 lg:px-12 py-6 max-w-screen-2xl mx-auto">
        <SeasonPickerSkeleton />
        <EpisodeListSkeleton />
      </div>
      <CastCarouselSkeleton />
    </>
  );
}

export default function TVShowPage() {
  const params = useParams();
  const showId = params.id ? parseInt(params.id as string, 10) : null;

  // Fetch show details
  const { data: show, isLoading: showLoading, error: showError } = useTVShow(showId);

  // Selected season state (null until show loads)
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null);

  // Torrent search modal state
  const [isTorrentModalOpen, setIsTorrentModalOpen] = useState(false);
  const [torrentContext, setTorrentContext] = useState<TorrentSearchContext | null>(null);

  // Fetch season details when selectedSeason is set
  const {
    data: seasonData,
    isLoading: seasonLoading,
    error: seasonError,
  } = useSeason(showId, selectedSeason);

  // Initialize selected season when show loads
  useEffect(() => {
    if (show && show.seasons?.length > 0 && selectedSeason === null) {
      // Find first non-specials season (season 0 is often "Specials")
      const firstSeason = show.seasons.find((s) => s.seasonNumber > 0) || show.seasons[0];
      setSelectedSeason(firstSeason.seasonNumber);
    }
  }, [show, selectedSeason]);

  // Handler for "Search Torrents" button on show
  const handleSearchTorrents = useCallback(() => {
    if (!show || !showId) return;

    const year = show.firstAirDate
      ? parseInt(show.firstAirDate.substring(0, 4), 10)
      : undefined;

    setTorrentContext({
      mediaType: 'tv',
      query: show.name,
      title: show.name,
      tmdbId: showId,
      year,
    });
    setIsTorrentModalOpen(true);
  }, [show, showId]);

  // Handler for episode torrent search
  const handleEpisodeTorrentSearch = useCallback(
    (query: string) => {
      if (!show || !showId) return;

      // Parse season and episode from query format "ShowName S01E05"
      const match = query.match(/S(\d+)E(\d+)/i);

      const year = show.firstAirDate
        ? parseInt(show.firstAirDate.substring(0, 4), 10)
        : undefined;

      setTorrentContext({
        mediaType: 'episode',
        query,
        title: show.name,
        tmdbId: showId,
        year,
        season: match ? parseInt(match[1], 10) : undefined,
        episode: match ? parseInt(match[2], 10) : undefined,
      });
      setIsTorrentModalOpen(true);
    },
    [show, showId]
  );

  // Invalid ID state
  if (!showId || isNaN(showId)) {
    return <ErrorState message="Invalid TV show ID" />;
  }

  // Loading state
  if (showLoading) {
    return <LoadingState />;
  }

  // Error state
  if (showError) {
    return <ErrorState message={showError} />;
  }

  // No data state
  if (!show) {
    return <ErrorState message="TV show not found" />;
  }

  // Format metadata
  const releaseYear = extractYear(show.firstAirDate);
  const releaseDate = formatReleaseDate(show.firstAirDate);

  // Get seasons excluding "Specials" (season 0) for picker, but include all if only specials exist
  const regularSeasons = show.seasons?.filter((s) => s.seasonNumber > 0) || [];
  const displaySeasons = regularSeasons.length > 0 ? regularSeasons : show.seasons || [];

  return (
    <>
      {/* TV Show Details Hero */}
      <MediaDetails
        title={show.name}
        tagline={show.tagline}
        overview={show.overview}
        backdropPath={show.backdropPath}
        posterPath={show.posterPath}
        rating={show.voteAverage}
        releaseYear={releaseYear}
        runtime={null}
        releaseDate={releaseDate}
        genres={show.genres}
        mediaType="tv"
        onSearchTorrents={handleSearchTorrents}
      />

      {/* Season Picker & Episodes Section */}
      <section className="px-4 sm:px-6 lg:px-12 py-6 max-w-screen-2xl mx-auto">
        {displaySeasons.length > 0 && selectedSeason !== null && (
          <SeasonPicker
            seasons={displaySeasons}
            selectedSeason={selectedSeason}
            onSeasonChange={setSelectedSeason}
            disabled={seasonLoading}
          />
        )}

        {/* Episode List */}
        {seasonLoading ? (
          <EpisodeListSkeleton />
        ) : seasonData?.episodes ? (
          <EpisodeList
            episodes={seasonData.episodes}
            showName={show.name}
            seasonNumber={selectedSeason || 1}
            onSearchTorrents={handleEpisodeTorrentSearch}
          />
        ) : seasonError ? (
          <p className="text-text-secondary py-4">Failed to load episodes: {seasonError}</p>
        ) : null}
      </section>

      {/* Cast Carousel */}
      {show.credits.cast.length > 0 && (
        <CastCarousel cast={show.credits.cast} maxItems={10} />
      )}

      {/* Torrent Search Modal */}
      {torrentContext && (
        <TorrentSearchModal
          isOpen={isTorrentModalOpen}
          onClose={() => setIsTorrentModalOpen(false)}
          context={torrentContext}
        />
      )}
    </>
  );
}
