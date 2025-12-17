import Image from 'next/image';
import Link from 'next/link';
import type { SearchResult } from '@/types/tmdb';
import { isMovie, getMediaTitle, getMediaReleaseYear } from '@/types/tmdb';
import { buildImageUrl } from '@/config/tmdb';

interface MediaCardProps {
  media: SearchResult;
  priority?: boolean;
}

function RatingBadge({ rating }: { rating: number }) {
  const displayRating = rating.toFixed(1);
  return (
    <div
      className="
        absolute top-2 right-2 z-10
        bg-black/70 backdrop-blur-sm
        px-2 py-1 rounded
        text-xs font-semibold text-accent-yellow
      "
    >
      {displayRating}
    </div>
  );
}

export function MediaCard({ media, priority = false }: MediaCardProps) {
  const title = getMediaTitle(media);
  const year = getMediaReleaseYear(media);
  const posterUrl = buildImageUrl(media.posterPath, 'poster', 'medium');
  const href = isMovie(media) ? `/movie/${media.id}` : `/tv/${media.id}`;

  return (
    <Link
      href={href}
      className="
        group relative block
        w-[140px] sm:w-[160px] lg:w-[180px] xl:w-[200px]
        flex-shrink-0
        focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
        focus-visible:ring-offset-2 focus-visible:ring-offset-bg-primary
        rounded-md
      "
    >
      {/* Poster container */}
      <div
        className="
          relative aspect-[2/3] rounded-md overflow-hidden
          bg-bg-elevated
          transition-transform duration-200
          group-hover:scale-105 group-hover:z-10
        "
      >
        {posterUrl ? (
          <Image
            src={posterUrl}
            alt={title}
            fill
            sizes="(max-width: 640px) 140px, (max-width: 1024px) 160px, (max-width: 1280px) 180px, 200px"
            className="object-cover"
            priority={priority}
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-text-muted">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="48"
              height="48"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18" />
              <line x1="7" y1="2" x2="7" y2="22" />
              <line x1="17" y1="2" x2="17" y2="22" />
              <line x1="2" y1="12" x2="22" y2="12" />
              <line x1="2" y1="7" x2="7" y2="7" />
              <line x1="2" y1="17" x2="7" y2="17" />
              <line x1="17" y1="17" x2="22" y2="17" />
              <line x1="17" y1="7" x2="22" y2="7" />
            </svg>
          </div>
        )}

        {/* Rating badge */}
        {media.voteAverage > 0 && <RatingBadge rating={media.voteAverage} />}

        {/* Title overlay on hover */}
        <div
          className="
            absolute inset-x-0 bottom-0 p-3
            bg-gradient-to-t from-black/90 via-black/60 to-transparent
            opacity-0 group-hover:opacity-100
            transition-opacity duration-200
          "
        >
          <p className="text-sm font-medium text-white line-clamp-2">{title}</p>
          {year && (
            <p className="text-xs text-text-secondary mt-1">{year}</p>
          )}
        </div>
      </div>
    </Link>
  );
}

export function MediaCardSkeleton() {
  return (
    <div className="w-[140px] sm:w-[160px] lg:w-[180px] xl:w-[200px] flex-shrink-0">
      <div className="aspect-[2/3] rounded-md bg-bg-hover animate-pulse" />
    </div>
  );
}
