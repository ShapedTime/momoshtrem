'use client';

import Image from 'next/image';
import Link from 'next/link';
import { useState, useCallback, useMemo } from 'react';
import { buildImageUrl } from '@/config/tmdb';
import { LibraryStatusBadge } from './LibraryStatusBadge';
import type { LibraryMovie, LibraryShow, LibraryStatus } from '@/types/momoshtrem';
import type { TorrentStatus } from '@/types/torrent';

interface LibraryCardProps {
  item: LibraryMovie | LibraryShow;
  mediaType: 'movie' | 'tv';
  posterPath?: string | null;
  onRemove?: (id: number, mediaType: 'movie' | 'tv') => void;
  /** Optional torrent status for visual border/glow indicator */
  torrentStatus?: TorrentStatus | null;
}

function FilmIcon({ size = 48 }: { size?: number }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
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
  );
}

function TrashIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </svg>
  );
}

function getLibraryStatus(item: LibraryMovie | LibraryShow, mediaType: 'movie' | 'tv'): LibraryStatus {
  if (mediaType === 'movie') {
    const movie = item as LibraryMovie;
    return movie.has_assignment ? 'has_assignment' : 'in_library';
  } else {
    const show = item as LibraryShow;
    const hasAnyAssignment = show.seasons?.some((season) =>
      season.episodes?.some((episode) => episode.has_assignment)
    );
    return hasAnyAssignment ? 'has_assignment' : 'in_library';
  }
}

/**
 * Get border/glow classes based on torrent status.
 * - No assignment: No border
 * - Has assignment, downloading: Blue ring + blue glow
 * - Has assignment, complete/seeding: Green ring + green glow
 * - Has assignment, paused: Yellow ring
 */
function getTorrentBorderClasses(
  hasAssignment: boolean,
  torrentStatus: TorrentStatus | null | undefined
): string {
  if (!hasAssignment) return '';

  if (!torrentStatus) {
    // Has assignment but torrent not active (no status available)
    // Show a subtle white ring to indicate assigned but not downloading
    return 'ring-1 ring-white/20';
  }

  if (torrentStatus.is_paused) {
    return 'ring-2 ring-accent-yellow';
  }

  if (torrentStatus.progress >= 1) {
    // Complete/seeding
    return 'ring-2 ring-accent-green shadow-lg shadow-accent-green/20';
  }

  // Downloading
  return 'ring-2 ring-accent-blue shadow-lg shadow-accent-blue/20';
}

export function LibraryCard({ item, mediaType, posterPath, onRemove, torrentStatus }: LibraryCardProps) {
  const [showConfirm, setShowConfirm] = useState(false);
  const [isRemoving, setIsRemoving] = useState(false);

  const href = mediaType === 'movie' ? `/movie/${item.tmdb_id}` : `/tv/${item.tmdb_id}`;
  const posterUrl = posterPath ? buildImageUrl(posterPath, 'poster', 'medium') : null;
  const status = getLibraryStatus(item, mediaType);
  const hasAssignment = status === 'has_assignment';

  // Compute border/glow classes based on torrent status
  const borderClasses = useMemo(
    () => getTorrentBorderClasses(hasAssignment, torrentStatus),
    [hasAssignment, torrentStatus]
  );

  const handleRemoveClick = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setShowConfirm(true);
  }, []);

  const handleConfirmRemove = useCallback(async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsRemoving(true);
    onRemove?.(item.id, mediaType);
  }, [item.id, mediaType, onRemove]);

  const handleCancelRemove = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setShowConfirm(false);
  }, []);

  return (
    <div className="group relative">
      <Link
        href={href}
        className="
          block
          w-[140px] sm:w-[160px] lg:w-[180px] xl:w-[200px]
          flex-shrink-0
          focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
          focus-visible:ring-offset-2 focus-visible:ring-offset-bg-primary
          rounded-md
        "
      >
        {/* Poster container */}
        <div
          className={`
            relative aspect-[2/3] rounded-md overflow-hidden
            bg-bg-elevated
            transition-all duration-200 motion-reduce:transition-none
            group-hover:scale-105 group-hover:z-10
            ${borderClasses}
          `}
        >
          {posterUrl ? (
            <Image
              src={posterUrl}
              alt={item.title}
              fill
              sizes="(max-width: 640px) 140px, (max-width: 1024px) 160px, (max-width: 1280px) 180px, 200px"
              className="object-cover"
            />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-text-muted">
              <FilmIcon size={48} />
            </div>
          )}

          {/* Status badge */}
          <LibraryStatusBadge status={status} variant="card" />

          {/* Remove button overlay */}
          {onRemove && !showConfirm && (
            <button
              onClick={handleRemoveClick}
              className="
                absolute top-2 right-2
                p-1.5 rounded-md
                bg-black/60 hover:bg-accent-red
                text-white
                opacity-0 group-hover:opacity-100
                transition-opacity duration-200 motion-reduce:transition-none
                focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
              "
              aria-label={`Remove ${item.title} from library`}
            >
              <TrashIcon />
            </button>
          )}

          {/* Confirm remove overlay */}
          {showConfirm && (
            <div
              className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/80 p-4"
              onClick={(e) => e.preventDefault()}
            >
              <p className="text-sm text-white text-center">Remove?</p>
              <div className="flex gap-2">
                <button
                  onClick={handleConfirmRemove}
                  disabled={isRemoving}
                  className="
                    px-3 py-1.5 text-xs font-medium
                    bg-accent-red hover:bg-red-700
                    text-white rounded
                    disabled:opacity-50
                    transition-colors
                  "
                >
                  {isRemoving ? 'Removing...' : 'Yes'}
                </button>
                <button
                  onClick={handleCancelRemove}
                  disabled={isRemoving}
                  className="
                    px-3 py-1.5 text-xs font-medium
                    bg-white/20 hover:bg-white/30
                    text-white rounded
                    disabled:opacity-50
                    transition-colors
                  "
                >
                  No
                </button>
              </div>
            </div>
          )}

          {/* Title overlay on hover */}
          <div
            className="
              absolute inset-x-0 bottom-0 p-3
              bg-gradient-to-t from-black/90 via-black/60 to-transparent
              opacity-0 group-hover:opacity-100
              transition-opacity duration-200 motion-reduce:transition-none
            "
          >
            <p className="text-sm font-medium text-white line-clamp-2">{item.title}</p>
            {item.year && (
              <p className="text-xs text-text-secondary mt-1">{item.year}</p>
            )}
          </div>
        </div>
      </Link>
    </div>
  );
}

export function LibraryCardSkeleton() {
  return (
    <div className="w-[140px] sm:w-[160px] lg:w-[180px] xl:w-[200px] flex-shrink-0">
      <div className="aspect-[2/3] rounded-md bg-bg-hover animate-pulse motion-reduce:animate-none" />
    </div>
  );
}
