'use client';

import { formatBytes, formatSpeed, formatProgress } from '@/types/torrent';

interface TorrentProgressProps {
  /** Progress value between 0 and 1 */
  progress: number;
  /** Download speed in bytes/sec */
  downloadSpeed?: number;
  /** Upload speed in bytes/sec */
  uploadSpeed?: number;
  /** Show download/upload speeds */
  showSpeeds?: boolean;
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
  /** Total size in bytes */
  totalSize?: number;
  /** Downloaded size in bytes */
  downloadedSize?: number;
}

const sizeClasses = {
  sm: 'h-1',
  md: 'h-2',
  lg: 'h-3',
};

export function TorrentProgress({
  progress,
  downloadSpeed = 0,
  uploadSpeed = 0,
  showSpeeds = false,
  size = 'md',
  totalSize,
  downloadedSize,
}: TorrentProgressProps) {
  const percentage = Math.min(Math.max(progress * 100, 0), 100);
  const isComplete = progress >= 1;

  return (
    <div className="space-y-1.5">
      {/* Progress bar */}
      <div className={`w-full bg-bg-elevated rounded-full overflow-hidden ${sizeClasses[size]}`}>
        <div
          className={`h-full rounded-full transition-all duration-300 ${
            isComplete ? 'bg-accent-green' : 'bg-accent-blue'
          }`}
          style={{ width: `${percentage}%` }}
          role="progressbar"
          aria-valuenow={percentage}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={`Download progress: ${formatProgress(progress)}`}
        />
      </div>

      {/* Stats row */}
      <div className="flex items-center justify-between text-xs text-text-secondary">
        {/* Size info */}
        <div className="flex items-center gap-2">
          {downloadedSize !== undefined && totalSize !== undefined ? (
            <span>
              {formatBytes(downloadedSize)} / {formatBytes(totalSize)}
            </span>
          ) : (
            <span>{formatProgress(progress)}</span>
          )}
        </div>

        {/* Speed info */}
        {showSpeeds && (downloadSpeed > 0 || uploadSpeed > 0) && (
          <div className="flex items-center gap-3">
            {downloadSpeed > 0 && (
              <span className="flex items-center gap-1">
                <DownArrowIcon />
                {formatSpeed(downloadSpeed)}
              </span>
            )}
            {uploadSpeed > 0 && (
              <span className="flex items-center gap-1">
                <UpArrowIcon />
                {formatSpeed(uploadSpeed)}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function DownArrowIcon() {
  return (
    <svg
      className="w-3 h-3 text-accent-blue"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M19 14l-7 7m0 0l-7-7m7 7V3"
      />
    </svg>
  );
}

function UpArrowIcon() {
  return (
    <svg
      className="w-3 h-3 text-accent-green"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M5 10l7-7m0 0l7 7m-7-7v18"
      />
    </svg>
  );
}
