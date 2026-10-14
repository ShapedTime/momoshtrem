'use client';

import { useCallback, useState } from 'react';
import { SpinnerIcon } from '@/components/ui';
import type { Subtitle } from '@/types/subtitle';

interface SubtitleListProps {
  subtitles: Subtitle[];
  isLoading: boolean;
  onDelete: (subtitleId: number) => Promise<boolean>;
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
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
    >
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
      <line x1="10" y1="11" x2="10" y2="17" />
      <line x1="14" y1="11" x2="14" y2="17" />
    </svg>
  );
}

export function SubtitleList({ subtitles, isLoading, onDelete }: SubtitleListProps) {
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const handleDelete = useCallback(
    async (subtitleId: number) => {
      setDeletingId(subtitleId);
      try {
        await onDelete(subtitleId);
      } finally {
        setDeletingId(null);
      }
    },
    [onDelete]
  );

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-text-secondary text-sm">
        <SpinnerIcon className="w-4 h-4 animate-spin" />
        <span>Loading subtitles...</span>
      </div>
    );
  }

  if (subtitles.length === 0) {
    return (
      <p className="text-sm text-text-muted">
        No subtitles downloaded yet.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      <h4 className="text-sm font-medium text-text-secondary">Downloaded Subtitles</h4>
      <div className="flex flex-wrap gap-2">
        {subtitles.map((subtitle) => (
          <div
            key={subtitle.id}
            className="
              flex items-center gap-2 px-3 py-1.5
              bg-bg-secondary rounded-lg border border-border
              text-sm
            "
          >
            <span className="font-medium text-white">
              {subtitle.language_name}
            </span>
            <span className="text-text-muted">
              ({subtitle.format.toUpperCase()})
            </span>
            <span className="text-text-muted text-xs">
              {formatFileSize(subtitle.file_size)}
            </span>
            <button
              onClick={() => handleDelete(subtitle.id)}
              disabled={deletingId === subtitle.id}
              className="
                p-1 -mr-1 text-text-muted hover:text-red-400
                transition-colors disabled:opacity-50
              "
              title="Delete subtitle"
            >
              {deletingId === subtitle.id ? (
                <SpinnerIcon className="w-4 h-4 animate-spin" />
              ) : (
                <TrashIcon />
              )}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
