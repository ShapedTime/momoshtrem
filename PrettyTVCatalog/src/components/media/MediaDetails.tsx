import Image from 'next/image';
import { buildImageUrl } from '@/config/tmdb';
import { Button, RatingBadge, FilmIcon } from '@/components/ui';
import { AddToLibraryButton, LibraryStatusBadge } from '@/components/library';
import type { Genre } from '@/types/tmdb';
import type { LibraryStatus } from '@/types/momoshtrem';

interface MediaDetailsProps {
  title: string;
  tagline?: string | null;
  overview: string;
  backdropPath: string | null;
  posterPath: string | null;
  rating: number;
  releaseYear: number | null;
  runtime?: string | null;
  releaseDate?: string | null;
  genres: Genre[];
  mediaType: 'movie' | 'tv';
  tmdbId: number;
  libraryStatus: LibraryStatus;
  onLibraryStatusChange?: (status: LibraryStatus, libraryId?: number) => void;
  onSearchTorrents: () => void;
}

export function MediaDetails({
  title,
  tagline,
  overview,
  backdropPath,
  posterPath,
  rating,
  releaseYear,
  runtime,
  releaseDate,
  genres,
  mediaType,
  tmdbId,
  libraryStatus,
  onLibraryStatusChange,
  onSearchTorrents,
}: MediaDetailsProps) {
  const backdropUrl = buildImageUrl(backdropPath, 'backdrop', 'large');
  const posterUrl = buildImageUrl(posterPath, 'poster', 'large');
  const mediaTypeLabel = mediaType === 'movie' ? 'Movie' : 'TV Show';

  return (
    <div>
      {/* Hero Section with Backdrop */}
      <section className="relative h-[50vh] sm:h-[60vh] lg:h-[70vh] lg:max-h-[800px] w-full">
        {/* Backdrop Image */}
        {backdropUrl ? (
          <Image
            src={backdropUrl}
            alt={title}
            fill
            sizes="100vw"
            className="object-cover object-top"
            priority
          />
        ) : (
          <div className="absolute inset-0 bg-bg-elevated" />
        )}

        {/* Gradient Overlays */}
        <div className="absolute inset-0 bg-gradient-to-t from-bg-primary via-bg-primary/60 to-transparent" />
        <div className="absolute inset-0 bg-gradient-to-r from-bg-primary/90 via-bg-primary/40 to-transparent" />

        {/* Content Container */}
        <div
          className="
            absolute inset-0
            flex flex-col lg:flex-row items-end lg:items-end
            gap-6 lg:gap-8
            px-4 sm:px-6 lg:px-12 pb-8 sm:pb-12 lg:pb-16
            max-w-screen-2xl mx-auto
          "
        >
          {/* Poster - Hidden on mobile, visible on lg+ */}
          <div className="hidden lg:block flex-shrink-0">
            <div
              className="
                relative w-[200px] xl:w-[250px]
                aspect-[2/3] rounded-lg overflow-hidden
                shadow-lg bg-bg-elevated
              "
            >
              {posterUrl ? (
                <Image
                  src={posterUrl}
                  alt={title}
                  fill
                  sizes="250px"
                  className="object-cover"
                  priority
                />
              ) : (
                <div className="absolute inset-0 flex items-center justify-center text-text-muted">
                  <FilmIcon size={64} />
                </div>
              )}
            </div>
          </div>

          {/* Info Section */}
          <div className="flex-1 min-w-0">
            {/* Media Type Badge */}
            <span className="text-xs uppercase tracking-wider text-text-secondary mb-2 block">
              {mediaTypeLabel}
            </span>

            {/* Title */}
            <h1 className="text-3xl sm:text-4xl lg:text-5xl font-bold text-white mb-2 sm:mb-3">
              {title}
            </h1>

            {/* Tagline */}
            {tagline && (
              <p className="text-lg text-text-secondary italic mb-4">{tagline}</p>
            )}

            {/* Meta Info Row */}
            <div className="flex flex-wrap items-center gap-3 sm:gap-4 mb-4">
              {rating > 0 && <RatingBadge rating={rating} variant="hero" />}
              {releaseYear && (
                <span className="text-text-secondary">{releaseYear}</span>
              )}
              {runtime && <span className="text-text-secondary">{runtime}</span>}
            </div>

            {/* Genres */}
            {genres.length > 0 && (
              <div className="flex flex-wrap gap-2 mb-4">
                {genres.map((genre) => (
                  <span
                    key={genre.id}
                    className="px-3 py-1 rounded-full bg-white/10 text-text-secondary text-sm"
                  >
                    {genre.name}
                  </span>
                ))}
              </div>
            )}

            {/* Overview */}
            {overview && (
              <p
                className="
                  text-sm sm:text-base text-text-secondary
                  line-clamp-3 sm:line-clamp-4 lg:line-clamp-none
                  max-w-2xl mb-6
                "
              >
                {overview}
              </p>
            )}

            {/* Action Buttons */}
            <div className="flex flex-wrap items-center gap-3">
              <Button
                variant="primary"
                size="lg"
                onClick={onSearchTorrents}
                aria-label={`Search torrents for ${title}`}
              >
                Search Torrents
              </Button>
              <AddToLibraryButton
                mediaType={mediaType}
                tmdbId={tmdbId}
                title={title}
                status={libraryStatus}
                onStatusChange={onLibraryStatusChange}
                size="lg"
              />
              {libraryStatus !== 'not_in_library' && (
                <LibraryStatusBadge status={libraryStatus} variant="inline" />
              )}
            </div>
          </div>
        </div>
      </section>

      {/* Additional Info Section - Below hero */}
      {releaseDate && (
        <div className="px-4 sm:px-6 lg:px-12 py-6 max-w-screen-2xl mx-auto">
          <div className="text-sm text-text-secondary">
            <span className="text-text-muted">Release Date: </span>
            {releaseDate}
          </div>
        </div>
      )}
    </div>
  );
}

export function MediaDetailsSkeleton() {
  return (
    <div>
      {/* Hero Skeleton */}
      <section className="relative h-[50vh] sm:h-[60vh] lg:h-[70vh] lg:max-h-[800px] w-full">
        <div className="absolute inset-0 bg-bg-elevated animate-pulse motion-reduce:animate-none" />
        <div className="absolute inset-0 bg-gradient-to-t from-bg-primary via-bg-primary/60 to-transparent" />
        <div className="absolute inset-0 bg-gradient-to-r from-bg-primary/90 via-bg-primary/40 to-transparent" />

        <div
          className="
            absolute inset-0
            flex flex-col lg:flex-row items-end lg:items-end
            gap-6 lg:gap-8
            px-4 sm:px-6 lg:px-12 pb-8 sm:pb-12 lg:pb-16
          "
        >
          {/* Poster Skeleton - lg+ only */}
          <div className="hidden lg:block flex-shrink-0">
            <div className="w-[200px] xl:w-[250px] aspect-[2/3] rounded-lg bg-bg-hover animate-pulse motion-reduce:animate-none" />
          </div>

          {/* Content Skeleton */}
          <div className="flex-1 min-w-0 w-full">
            <div className="h-4 w-16 bg-bg-hover rounded mb-3 animate-pulse motion-reduce:animate-none" />
            <div className="h-10 sm:h-12 lg:h-14 w-64 sm:w-96 bg-bg-hover rounded mb-3 animate-pulse motion-reduce:animate-none" />
            <div className="h-5 w-48 bg-bg-hover rounded mb-4 animate-pulse motion-reduce:animate-none" />
            <div className="flex gap-3 mb-4">
              <div className="h-6 w-16 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
              <div className="h-6 w-12 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
              <div className="h-6 w-16 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
            </div>
            <div className="flex gap-2 mb-4">
              <div className="h-7 w-20 bg-bg-hover rounded-full animate-pulse motion-reduce:animate-none" />
              <div className="h-7 w-24 bg-bg-hover rounded-full animate-pulse motion-reduce:animate-none" />
              <div className="h-7 w-16 bg-bg-hover rounded-full animate-pulse motion-reduce:animate-none" />
            </div>
            <div className="space-y-2 mb-6 max-w-2xl">
              <div className="h-4 w-full bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
              <div className="h-4 w-full bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
              <div className="h-4 w-3/4 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
            </div>
            <div className="h-12 w-40 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
          </div>
        </div>
      </section>
    </div>
  );
}
