'use client';

import { useState } from 'react';
import { TorrentProgress } from './TorrentProgress';
import { formatBytes, getTorrentDisplayStatus } from '@/types/torrent';
import type { TorrentStatus } from '@/types/torrent';
import type { TorrentAssignment } from '@/types/momoshtrem';

interface TorrentInfoSectionProps {
  /** The torrent assignment data */
  assignment: TorrentAssignment;
  /** Live torrent status (if torrent is active) */
  torrentStatus?: TorrentStatus | null;
  /** Called when user wants to unassign the torrent */
  onUnassign: () => Promise<void>;
  /** Whether unassign operation is in progress */
  isUnassigning?: boolean;
}

export function TorrentInfoSection({
  assignment,
  torrentStatus,
  onUnassign,
  isUnassigning = false,
}: TorrentInfoSectionProps) {
  const [showConfirm, setShowConfirm] = useState(false);

  const displayStatus = getTorrentDisplayStatus(torrentStatus);
  const hasActiveTorrent = displayStatus !== 'none';
  const isDownloading = displayStatus === 'downloading';
  const isComplete = displayStatus === 'seeding';
  const isPaused = displayStatus === 'paused';

  const handleRemoveClick = () => {
    setShowConfirm(true);
  };

  const handleConfirmRemove = async () => {
    await onUnassign();
    setShowConfirm(false);
  };

  const handleCancelRemove = () => {
    setShowConfirm(false);
  };

  // Extract filename from path
  const fileName = assignment.file_path?.split('/').pop() || 'Unknown file';

  return (
    <div className="bg-bg-elevated rounded-lg p-4 sm:p-6 space-y-4">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-2">
          <TorrentIcon />
          <h3 className="text-lg font-semibold text-white">Torrent</h3>
          <StatusBadge status={displayStatus} />
        </div>
      </div>

      {/* File info */}
      <div className="space-y-2">
        <p className="text-sm text-white font-medium truncate" title={fileName}>
          {fileName}
        </p>
        <div className="flex flex-wrap items-center gap-3 text-sm text-text-secondary">
          {assignment.file_size && (
            <span>{formatBytes(assignment.file_size)}</span>
          )}
          {assignment.resolution && (
            <span className="px-2 py-0.5 bg-bg-hover rounded text-xs font-medium">
              {assignment.resolution}
            </span>
          )}
          {assignment.source && (
            <span className="text-text-muted">{assignment.source}</span>
          )}
        </div>
      </div>

      {/* Progress (if active torrent) */}
      {hasActiveTorrent && torrentStatus && (
        <div className="pt-2">
          <TorrentProgress
            progress={torrentStatus.progress}
            downloadSpeed={torrentStatus.download_speed}
            uploadSpeed={torrentStatus.upload_speed}
            showSpeeds={isDownloading || isPaused}
            totalSize={torrentStatus.total_size}
            downloadedSize={torrentStatus.downloaded}
          />

          {/* Peer info */}
          <div className="flex items-center gap-4 mt-2 text-xs text-text-secondary">
            <span className="flex items-center gap-1">
              <SeedersIcon />
              {torrentStatus.seeders} seeders
            </span>
            <span className="flex items-center gap-1">
              <LeechersIcon />
              {torrentStatus.leechers} leechers
            </span>
          </div>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2">
        {!showConfirm ? (
          <button
            onClick={handleRemoveClick}
            disabled={isUnassigning}
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
            Remove Torrent
          </button>
        ) : (
          <div className="flex items-center gap-2">
            <span className="text-sm text-text-secondary">Remove torrent?</span>
            <button
              onClick={handleConfirmRemove}
              disabled={isUnassigning}
              className="
                px-3 py-1.5 text-sm font-medium
                bg-accent-red hover:bg-red-700
                text-white rounded
                disabled:opacity-50
                transition-colors
              "
            >
              {isUnassigning ? 'Removing...' : 'Yes'}
            </button>
            <button
              onClick={handleCancelRemove}
              disabled={isUnassigning}
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
  if (status === 'none') return null;

  const configs = {
    downloading: {
      label: 'Downloading',
      className: 'bg-accent-blue/20 text-accent-blue',
    },
    seeding: {
      label: 'Ready',
      className: 'bg-accent-green/20 text-accent-green',
    },
    paused: {
      label: 'Paused',
      className: 'bg-accent-yellow/20 text-accent-yellow',
    },
    none: {
      label: '',
      className: '',
    },
  };

  const config = configs[status];

  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${config.className}`}>
      {config.label}
    </span>
  );
}

function TorrentIcon() {
  return (
    <svg
      className="w-5 h-5 text-text-secondary"
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

function SeedersIcon() {
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

function LeechersIcon() {
  return (
    <svg
      className="w-3 h-3 text-accent-yellow"
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
