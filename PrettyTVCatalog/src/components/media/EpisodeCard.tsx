import Image from 'next/image';
import { buildImageUrl } from '@/config/tmdb';
import { formatRuntime } from '@/lib/utils';
import { Button, FilmIcon, StarIcon } from '@/components/ui';
import type { Episode } from '@/types/tmdb';
import type { TorrentAssignment } from '@/types/momoshtrem';
import { formatBytes, type TorrentStatus } from '@/types/torrent';

export interface EpisodeAssignmentInfo {
  episodeId: number;
  assignment: TorrentAssignment;
  torrentStatus?: TorrentStatus | null;
}

interface EpisodeCardProps {
  episode: Episode;
  showName: string;
  seasonNumber: number;
  onSearchTorrents?: (query: string) => void;
  /** Assignment info for this episode (if has torrent assigned) */
  assignmentInfo?: EpisodeAssignmentInfo;
  /** Handler for unassigning torrent from episode */
  onUnassign?: (episodeId: number) => Promise<void>;
}

/**
 * Format season and episode numbers into standard code (S01E05).
 */
function formatEpisodeCode(season: number, episode: number): string {
  return `S${season.toString().padStart(2, '0')}E${episode.toString().padStart(2, '0')}`;
}

/**
 * Horizontal episode card with 16:9 still image.
 * Shows episode metadata and "Search Torrents" button.
 * Displays assignment status when episode has a torrent assigned.
 */
export function EpisodeCard({
  episode,
  showName,
  seasonNumber,
  onSearchTorrents,
  assignmentInfo,
  onUnassign,
}: EpisodeCardProps) {
  const stillUrl = buildImageUrl(episode.stillPath, 'still', 'large');
  const episodeCode = formatEpisodeCode(seasonNumber, episode.episodeNumber);
  const runtime = formatRuntime(episode.runtime);
  const searchQuery = `${showName} ${episodeCode}`;

  const handleSearchClick = () => {
    onSearchTorrents?.(searchQuery);
  };

  const handleUnassign = async () => {
    if (assignmentInfo && onUnassign) {
      await onUnassign(assignmentInfo.episodeId);
    }
  };

  // Determine assignment status for styling
  const hasAssignment = !!assignmentInfo;
  const torrentStatus = assignmentInfo?.torrentStatus;
  const isDownloading = torrentStatus && !torrentStatus.is_paused && torrentStatus.progress < 1;
  const isComplete = torrentStatus && torrentStatus.progress >= 1;
  const isPaused = torrentStatus?.is_paused;

  // Get border classes based on torrent status
  const getBorderClasses = () => {
    if (!hasAssignment) return '';
    if (!torrentStatus) return 'ring-1 ring-white/20';
    if (isPaused) return 'ring-2 ring-accent-yellow';
    if (isComplete) return 'ring-2 ring-accent-green';
    if (isDownloading) return 'ring-2 ring-accent-blue';
    return 'ring-1 ring-white/20';
  };

  return (
    <article
      className={`
        flex flex-col sm:flex-row gap-4
        p-4 rounded-lg
        bg-bg-elevated
        hover:bg-bg-hover
        transition-colors duration-200 motion-reduce:transition-none
        ${getBorderClasses()}
      `}
    >
      {/* Episode Still Image - 16:9 aspect ratio */}
      <div className="flex-shrink-0 w-full sm:w-[200px] lg:w-[260px]">
        <div className="relative aspect-video rounded-md overflow-hidden bg-bg-hover">
          {stillUrl ? (
            <Image
              src={stillUrl}
              alt={episode.name}
              fill
              sizes="(max-width: 640px) 100vw, 260px"
              className="object-cover"
            />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-text-muted">
              <FilmIcon size={32} />
            </div>
          )}
          {/* Episode number overlay */}
          <div className="absolute top-2 left-2 px-2 py-1 bg-black/70 rounded text-xs font-medium text-white">
            {episodeCode}
          </div>
          {/* Assignment status badge */}
          {hasAssignment && (
            <div className="absolute top-2 right-2">
              <EpisodeStatusBadge
                isComplete={!!isComplete}
                isDownloading={!!isDownloading}
                isPaused={!!isPaused}
                progress={torrentStatus?.progress}
              />
            </div>
          )}
        </div>
      </div>

      {/* Episode Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            {/* Episode Title */}
            <h3 className="text-lg font-semibold text-white line-clamp-1">
              {episode.name}
            </h3>

            {/* Meta info row */}
            <div className="flex flex-wrap items-center gap-3 mt-1 text-sm text-text-secondary">
              {episode.airDate && (
                <span>{new Date(episode.airDate).toLocaleDateString()}</span>
              )}
              {runtime && <span>{runtime}</span>}
              {episode.voteAverage > 0 && (
                <div className="flex items-center gap-1 text-accent-yellow">
                  <StarIcon size={14} />
                  <span className="font-medium">{episode.voteAverage.toFixed(1)}</span>
                </div>
              )}
            </div>
          </div>

          {/* Actions - Desktop */}
          <div className="hidden sm:flex items-center gap-2 flex-shrink-0">
            {hasAssignment ? (
              <>
                {/* Show assignment info and remove button */}
                <span className="text-xs text-text-secondary">
                  {formatBytes(assignmentInfo.assignment.file_size)}
                  {assignmentInfo.assignment.resolution && (
                    <> • {assignmentInfo.assignment.resolution}</>
                  )}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleUnassign}
                  aria-label={`Remove torrent for ${showName} ${episodeCode}`}
                  className="text-text-muted hover:text-accent-red"
                >
                  Remove
                </Button>
              </>
            ) : (
              <Button
                variant="secondary"
                size="sm"
                onClick={handleSearchClick}
                aria-label={`Search torrents for ${showName} ${episodeCode}`}
              >
                Search Torrents
              </Button>
            )}
          </div>
        </div>

        {/* Overview */}
        {episode.overview && (
          <p className="mt-2 text-sm text-text-secondary line-clamp-2 sm:line-clamp-3">
            {episode.overview}
          </p>
        )}

        {/* Assignment info row (when assigned) */}
        {hasAssignment && torrentStatus && (
          <div className="mt-2 flex items-center gap-4 text-xs text-text-secondary">
            {isDownloading && (
              <>
                <span className="text-accent-blue">
                  {Math.round(torrentStatus.progress * 100)}% downloaded
                </span>
                <span>↓ {formatBytes(torrentStatus.download_speed)}/s</span>
              </>
            )}
            {isComplete && (
              <span className="text-accent-green">Complete</span>
            )}
            {isPaused && (
              <span className="text-accent-yellow">Paused</span>
            )}
          </div>
        )}

        {/* Actions - Mobile */}
        <div className="sm:hidden mt-3">
          {hasAssignment ? (
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-text-secondary">
                {formatBytes(assignmentInfo.assignment.file_size)}
                {assignmentInfo.assignment.resolution && (
                  <> • {assignmentInfo.assignment.resolution}</>
                )}
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleUnassign}
                aria-label={`Remove torrent for ${showName} ${episodeCode}`}
                className="text-text-muted hover:text-accent-red"
              >
                Remove
              </Button>
            </div>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              onClick={handleSearchClick}
              className="w-full"
              aria-label={`Search torrents for ${showName} ${episodeCode}`}
            >
              Search Torrents
            </Button>
          )}
        </div>
      </div>
    </article>
  );
}

export function EpisodeCardSkeleton() {
  return (
    <div className="flex flex-col sm:flex-row gap-4 p-4 rounded-lg bg-bg-elevated">
      {/* Still skeleton */}
      <div className="flex-shrink-0 w-full sm:w-[200px] lg:w-[260px]">
        <div className="aspect-video rounded-md bg-bg-hover animate-pulse motion-reduce:animate-none" />
      </div>
      {/* Info skeleton */}
      <div className="flex-1 min-w-0">
        <div className="h-6 w-3/4 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
        <div className="h-4 w-1/3 bg-bg-hover rounded mt-2 animate-pulse motion-reduce:animate-none" />
        <div className="space-y-2 mt-3">
          <div className="h-4 w-full bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
          <div className="h-4 w-2/3 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
        </div>
      </div>
    </div>
  );
}

/**
 * Badge showing episode torrent status.
 */
function EpisodeStatusBadge({
  isComplete,
  isDownloading,
  isPaused,
  progress,
}: {
  isComplete: boolean;
  isDownloading: boolean;
  isPaused: boolean;
  progress?: number;
}) {
  if (isComplete) {
    return (
      <div
        className="flex items-center justify-center w-6 h-6 rounded-full bg-accent-green text-white"
        title="Download complete"
      >
        <CheckIcon />
      </div>
    );
  }

  if (isDownloading) {
    return (
      <div
        className="flex items-center justify-center px-2 py-1 rounded bg-accent-blue/90 text-white text-xs font-medium"
        title={`Downloading: ${Math.round((progress || 0) * 100)}%`}
      >
        <DownloadIcon className="w-3 h-3 mr-1" />
        {Math.round((progress || 0) * 100)}%
      </div>
    );
  }

  if (isPaused) {
    return (
      <div
        className="flex items-center justify-center w-6 h-6 rounded-full bg-accent-yellow text-white"
        title="Download paused"
      >
        <PauseIcon />
      </div>
    );
  }

  // Has assignment but no active torrent status
  return (
    <div
      className="flex items-center justify-center w-6 h-6 rounded-full bg-white/20 text-white"
      title="Torrent assigned"
    >
      <CheckIcon />
    </div>
  );
}

function CheckIcon() {
  return (
    <svg
      className="w-3.5 h-3.5"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={3}
        d="M5 13l4 4L19 7"
      />
    </svg>
  );
}

function DownloadIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      className={className}
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
      />
    </svg>
  );
}

function PauseIcon() {
  return (
    <svg
      className="w-3 h-3"
      fill="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
    </svg>
  );
}
