'use client';

import { useParams } from 'next/navigation';
import { useState, useEffect, useCallback, useMemo } from 'react';
import { useTVShow, useSeason } from '@/hooks/useTMDB';
import { useLibraryStatus } from '@/hooks/useLibrary';
import { useTorrents } from '@/hooks/useTorrents';
import { TorrentSearchModal } from '@/components/torrent';
import { SubtitleSearchModal } from '@/components/subtitle';
import type { TorrentSearchContext } from '@/types/jackett';
import type { SubtitleSearchContext } from '@/types/subtitle';
import type { LibraryStatus, LibraryShow } from '@/types/momoshtrem';
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
  type EpisodeAssignmentInfo,
} from '@/components/media';
import { Button, AlertCircleIcon, Breadcrumb } from '@/components/ui';

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

  // Library status
  const {
    status: libraryStatus,
    libraryId,
    hasAssignment,
    refresh: refreshLibraryStatus,
  } = useLibraryStatus('tv', showId || 0);

  // Local state for optimistic updates
  const [localLibraryStatus, setLocalLibraryStatus] = useState<LibraryStatus>('not_in_library');

  // Library show data (for episode assignments)
  const [libraryShow, setLibraryShow] = useState<LibraryShow | null>(null);

  // Fetch torrent statuses
  const { torrentMap, refresh: refreshTorrents } = useTorrents({
    autoRefresh: hasAssignment,
    refreshInterval: 5000,
  });

  // Sync library status from hook
  useEffect(() => {
    setLocalLibraryStatus(libraryStatus);
  }, [libraryStatus]);

  // Fetch library show data when in library
  useEffect(() => {
    const fetchLibraryShow = async () => {
      if (!libraryId) {
        setLibraryShow(null);
        return;
      }

      try {
        const res = await fetch(`/api/library/shows/${libraryId}`);
        if (res.ok) {
          const data = await res.json();
          setLibraryShow(data);
        }
      } catch (err) {
        console.error('Failed to fetch library show:', err);
      }
    };

    fetchLibraryShow();
  }, [libraryId]);

  // Handler for library status changes
  const handleLibraryStatusChange = useCallback((newStatus: LibraryStatus) => {
    setLocalLibraryStatus(newStatus);
    refreshLibraryStatus();
  }, [refreshLibraryStatus]);

  // Selected season state (null until show loads)
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null);

  // Torrent search modal state
  const [isTorrentModalOpen, setIsTorrentModalOpen] = useState(false);
  const [torrentContext, setTorrentContext] = useState<TorrentSearchContext | null>(null);

  // Subtitle search modal state
  const [isSubtitleModalOpen, setIsSubtitleModalOpen] = useState(false);
  const [subtitleContext, setSubtitleContext] = useState<SubtitleSearchContext | null>(null);

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

  // Handler for torrent modal close (may need to refresh status)
  const handleTorrentModalClose = useCallback(() => {
    setIsTorrentModalOpen(false);
    refreshLibraryStatus();
  }, [refreshLibraryStatus]);

  // Handler for finding subtitles for an episode
  const handleFindSubtitles = useCallback(
    (episodeId: number, tmdbId: number, seasonNumber: number, episodeNumber: number, title: string) => {
      setSubtitleContext({
        mediaType: 'episode',
        tmdbId,
        title,
        itemId: episodeId,
        season: seasonNumber,
        episode: episodeNumber,
      });
      setIsSubtitleModalOpen(true);
    },
    []
  );

  // Handler for subtitle modal close
  const handleSubtitleModalClose = useCallback(() => {
    setIsSubtitleModalOpen(false);
  }, []);

  // Build episode assignments map for the current season
  const episodeAssignments = useMemo(() => {
    if (!libraryShow || selectedSeason === null) {
      return new Map<number, EpisodeAssignmentInfo>();
    }

    const librarySeason = libraryShow.seasons?.find(
      (s) => s.season_number === selectedSeason
    );

    if (!librarySeason) {
      return new Map<number, EpisodeAssignmentInfo>();
    }

    const map = new Map<number, EpisodeAssignmentInfo>();
    for (const ep of librarySeason.episodes) {
      if (ep.has_assignment && ep.assignment) {
        map.set(ep.episode_number, {
          episodeId: ep.id,
          assignment: ep.assignment,
          torrentStatus: torrentMap.get(ep.assignment.info_hash) || null,
        });
      }
    }

    return map;
  }, [libraryShow, selectedSeason, torrentMap]);

  // Handler for unassigning episode torrent
  const handleUnassignEpisode = useCallback(async (episodeId: number) => {
    try {
      const res = await fetch(`/api/episodes/${episodeId}/assign`, {
        method: 'DELETE',
      });

      if (res.ok) {
        // Refresh library show data to update assignments
        if (libraryId) {
          const showRes = await fetch(`/api/library/shows/${libraryId}`);
          if (showRes.ok) {
            const data = await showRes.json();
            setLibraryShow(data);
          }
        }
        refreshTorrents();
      }
    } catch (err) {
      console.error('Failed to unassign episode:', err);
    }
  }, [libraryId, refreshTorrents]);

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
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        items={[
          { label: 'Home', href: '/' },
          { label: 'TV Shows', href: '/tv-shows' },
          { label: show.name },
        ]}
      />

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
        tmdbId={showId}
        libraryStatus={localLibraryStatus}
        onLibraryStatusChange={handleLibraryStatusChange}
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
            tmdbId={showId}
            onSearchTorrents={handleEpisodeTorrentSearch}
            episodeAssignments={episodeAssignments}
            onUnassignEpisode={handleUnassignEpisode}
            onFindSubtitles={handleFindSubtitles}
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
