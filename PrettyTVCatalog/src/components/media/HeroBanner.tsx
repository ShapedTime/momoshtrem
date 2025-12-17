import Image from 'next/image';
import Link from 'next/link';
import type { SearchResult } from '@/types/tmdb';
import { isMovie, getMediaTitle, getMediaReleaseYear } from '@/types/tmdb';
import { buildImageUrl } from '@/config/tmdb';
import { Button, RatingBadge } from '@/components/ui';

interface HeroBannerProps {
  media: SearchResult;
}

export function HeroBanner({ media }: HeroBannerProps) {
  const title = getMediaTitle(media);
  const year = getMediaReleaseYear(media);
  const backdropUrl = buildImageUrl(media.backdropPath, 'backdrop', 'large');
  const href = isMovie(media) ? `/movie/${media.id}` : `/tv/${media.id}`;
  const mediaTypeLabel = isMovie(media) ? 'Movie' : 'TV Show';

  return (
    <section className="relative h-[40vh] sm:h-[50vh] lg:h-[60vh] lg:max-h-[700px] w-full mb-8 sm:mb-10 lg:mb-12">
      {/* Backdrop image */}
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

      {/* Gradient overlays */}
      <div className="absolute inset-0 bg-gradient-to-t from-bg-primary via-bg-primary/50 to-transparent" />
      <div className="absolute inset-0 bg-gradient-to-r from-bg-primary/80 via-transparent to-transparent" />

      {/* Content */}
      <div
        className="
          absolute inset-0 flex flex-col justify-end
          px-4 sm:px-6 lg:px-12 pb-8 sm:pb-12 lg:pb-16
          max-w-screen-2xl mx-auto
        "
      >
        {/* Media type badge */}
        <span className="text-xs uppercase tracking-wider text-text-secondary mb-2">
          {mediaTypeLabel}
        </span>

        {/* Title */}
        <h1
          className="
            text-3xl sm:text-4xl lg:text-5xl font-bold text-white
            mb-3 sm:mb-4
            max-w-2xl
          "
        >
          {title}
        </h1>

        {/* Meta info */}
        <div className="flex items-center gap-4 mb-4">
          {media.voteAverage > 0 && <RatingBadge rating={media.voteAverage} variant="hero" />}
          {year && <span className="text-text-secondary">{year}</span>}
        </div>

        {/* Overview */}
        {media.overview && (
          <p
            className="
              text-sm sm:text-base text-text-secondary
              line-clamp-2 sm:line-clamp-3
              max-w-xl mb-6
            "
          >
            {media.overview}
          </p>
        )}

        {/* CTA Button */}
        <div className="flex gap-3">
          <Link href={href}>
            <Button variant="primary" size="lg">
              View Details
            </Button>
          </Link>
        </div>
      </div>
    </section>
  );
}

export function HeroBannerSkeleton() {
  return (
    <section className="relative h-[40vh] sm:h-[50vh] lg:h-[60vh] lg:max-h-[700px] w-full mb-8 sm:mb-10 lg:mb-12">
      <div className="absolute inset-0 bg-bg-elevated animate-pulse motion-reduce:animate-none" />
      <div className="absolute inset-0 bg-gradient-to-t from-bg-primary via-bg-primary/50 to-transparent" />

      <div
        className="
          absolute inset-0 flex flex-col justify-end
          px-4 sm:px-6 lg:px-12 pb-8 sm:pb-12 lg:pb-16
        "
      >
        <div className="h-4 w-16 bg-bg-hover rounded mb-3 animate-pulse motion-reduce:animate-none" />
        <div className="h-10 sm:h-12 lg:h-14 w-64 sm:w-80 bg-bg-hover rounded mb-4 animate-pulse motion-reduce:animate-none" />
        <div className="h-5 w-32 bg-bg-hover rounded mb-4 animate-pulse motion-reduce:animate-none" />
        <div className="h-4 w-full max-w-xl bg-bg-hover rounded mb-2 animate-pulse motion-reduce:animate-none" />
        <div className="h-4 w-3/4 max-w-xl bg-bg-hover rounded mb-6 animate-pulse motion-reduce:animate-none" />
        <div className="h-12 w-36 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
      </div>
    </section>
  );
}
