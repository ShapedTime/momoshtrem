'use client';

import { useState } from 'react';
import { TorrentProgress } from './TorrentProgress';
import {
  formatBytes,
  getTorrentDisplayStatus,
  type TorrentStatus,
} from '@/types/torrent';

interface TorrentManagementCardProps {
  torrent: TorrentStatus;
  onPause: () => Promise<boolean>;
  onResume: () => Promise<boolean>;
  onRemove: () => Promise<boolean>;
  isActioning?: boolean;
}

export function TorrentManagementCard({
  torrent,
  onPause,
  onResume,
  onRemove,
  isActioning = false,
}: TorrentManagementCardProps) {
  const [showRemoveConfirm, setShowRemoveConfirm] = useState(false);
  const [isRemoving, setIsRemoving] = useState(false);
  const [isPausing, setIsPausing] = useState(false);

  const displayStatus = getTorrentDisplayStatus(torrent);
  const isDownloading = displayStatus === 'downloading';
  const isSeeding = displayStatus === 'seeding';
  const isPaused = displayStatus === 'paused';

  const handlePauseResume = async () => {
    setIsPausing(true);
    try {
      if (isPaused) {
        await onResume();
      } else {
        await onPause();
      }
    } finally {
      setIsPausing(false);
    }
  };

  const handleRemoveClick = () => {
    setShowRemoveConfirm(true);
  };

  const handleConfirmRemove = async () => {
    setIsRemoving(true);
    try {
      await onRemove();
    } finally {
      setIsRemoving(false);
      setShowRemoveConfirm(false);
    }
  };

  const handleCancelRemove = () => {
    setShowRemoveConfirm(false);
  };

  // Truncate torrent name for display
  const displayName = torrent.name.length > 60
    ? torrent.name.substring(0, 60) + '...'
    : torrent.name;

  return (
    <div className="bg-bg-elevated rounded-lg p-4 sm:p-6 space-y-4">
      {/* Header row */}
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <h3
            className="text-base font-medium text-white truncate"
            title={torrent.name}
          >
            {displayName}
          </h3>
          <div className="flex items-center gap-2 mt-1">
            <StatusBadge status={displayStatus} />
            <span className="text-xs text-text-muted font-mono">
              {torrent.info_hash.substring(0, 12)}...
            </span>
          </div>
        </div>
      </div>

      {/* Progress section */}
      <div className="space-y-3">
        <TorrentProgress
          progress={torrent.progress}
          downloadSpeed={torrent.download_speed}
          uploadSpeed={torrent.upload_speed}
          showSpeeds={isDownloading || isPaused}
          totalSize={torrent.total_size}
          downloadedSize={torrent.downloaded}
          size="md"
        />

        {/* Stats row */}
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-text-secondary">
          <div className="flex items-center gap-1.5">
            <span className="text-accent-green">↑</span>
            <span>{torrent.seeders} seeders</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-accent-yellow">↓</span>
            <span>{torrent.leechers} leechers</span>
          </div>
          <div>
            <span className="text-text-muted">Size:</span>{' '}
            <span>{formatBytes(torrent.total_size)}</span>
          </div>
        </div>
      </div>

      {/* Actions row */}
      <div className="flex items-center gap-3 pt-2 border-t border-bg-hover">
        {!showRemoveConfirm ? (
          <>
            {/* Pause/Resume button */}
            <button
              onClick={handlePauseResume}
              disabled={isActioning || isPausing}
              className={`
                flex items-center gap-2
                px-3 py-2 text-sm font-medium
                rounded-md transition-colors
                disabled:opacity-50 disabled:cursor-not-allowed
                ${isPaused
                  ? 'bg-accent-green/20 text-accent-green hover:bg-accent-green/30'
                  : 'bg-accent-yellow/20 text-accent-yellow hover:bg-accent-yellow/30'
                }
              `}
            >
              {isPausing ? (
                <SpinnerIcon />
              ) : isPaused ? (
                <PlayIcon />
              ) : (
                <PauseIcon />
              )}
              {isPaused ? 'Resume' : 'Pause'}
            </button>

            {/* Remove button */}
            <button
              onClick={handleRemoveClick}
              disabled={isActioning || isRemoving}
              className="
                flex items-center gap-2
                px-3 py-2 text-sm font-medium
                bg-bg-hover hover:bg-accent-red/20 hover:text-accent-red
                text-text-secondary
                rounded-md transition-colors
                disabled:opacity-50 disabled:cursor-not-allowed
              "
            >
              <TrashIcon />
              Remove
            </button>
          </>
        ) : (
          <div className="flex items-center gap-3">
            <span className="text-sm text-text-secondary">Remove this torrent?</span>
            <button
              onClick={handleConfirmRemove}
              disabled={isRemoving}
              className="
                px-3 py-1.5 text-sm font-medium
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
                px-3 py-1.5 text-sm font-medium
                bg-white/10 hover:bg-white/20
                text-white rounded
                disabled:opacity-50
                transition-colors
              "
            >
              No
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: ReturnType<typeof getTorrentDisplayStatus> }) {
  const configs = {
    downloading: {
      label: 'Downloading',
      className: 'bg-accent-blue/20 text-accent-blue',
    },
    seeding: {
      label: 'Seeding',
      className: 'bg-accent-green/20 text-accent-green',
    },
    paused: {
      label: 'Paused',
      className: 'bg-accent-yellow/20 text-accent-yellow',
    },
    none: {
      label: 'Unknown',
      className: 'bg-bg-hover text-text-muted',
    },
  };

  const config = configs[status];

  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${config.className}`}>
      {config.label}
    </span>
  );
}

function PlayIcon() {
  return (
    <svg
      className="w-4 h-4"
      fill="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path d="M8 5v14l11-7z" />
    </svg>
  );
}

function PauseIcon() {
  return (
    <svg
      className="w-4 h-4"
      fill="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg
      className="w-4 h-4"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
      />
    </svg>
  );
}

function SpinnerIcon() {
  return (
    <svg
      className="w-4 h-4 animate-spin"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}

export function TorrentManagementCardSkeleton() {
  return (
    <div className="bg-bg-elevated rounded-lg p-4 sm:p-6 space-y-4 animate-pulse">
      <div className="h-5 bg-bg-hover rounded w-3/4" />
      <div className="h-2 bg-bg-hover rounded w-full" />
      <div className="flex gap-4">
        <div className="h-4 bg-bg-hover rounded w-20" />
        <div className="h-4 bg-bg-hover rounded w-20" />
      </div>
      <div className="flex gap-3 pt-2 border-t border-bg-hover">
        <div className="h-9 bg-bg-hover rounded w-24" />
        <div className="h-9 bg-bg-hover rounded w-24" />
      </div>
    </div>
  );
}
