'use client';

import { useState, useCallback } from 'react';
import { useToast } from '@/components/ui';
import { STATUS } from '@/lib/status';
import type { LibraryStatus } from '@/types/momoshtrem';
import { apiFetch } from '@/lib/api/client';

interface AddToLibraryButtonProps {
  mediaType: 'movie' | 'tv';
  tmdbId: number;
  title: string;
  /** Current library status */
  status: LibraryStatus;
  /** Called when status changes */
  onStatusChange?: (newStatus: LibraryStatus, libraryId?: number) => void;
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
  /** Additional class names */
  className?: string;
}

const sizeStyles = {
  sm: 'h-9 px-4 text-sm',
  md: 'h-11 px-6 text-base',
  lg: 'h-12 px-8 text-lg',
};

function PlusIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  );
}

function Spinner() {
  return (
    <svg
      className="animate-spin motion-reduce:animate-none h-5 w-5"
      xmlns="http://www.w3.org/2000/svg"
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

export function AddToLibraryButton({
  mediaType,
  tmdbId,
  title,
  status,
  onStatusChange,
  size = 'md',
  className = '',
}: AddToLibraryButtonProps) {
  const [isAdding, setIsAdding] = useState(false);
  const { toast } = useToast();

  const handleClick = useCallback(async () => {
    if (status !== 'not_in_library' || isAdding) return;

    setIsAdding(true);

    try {
      const response = await apiFetch('/api/library/add', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ media_type: mediaType, tmdb_id: tmdbId }),
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to add to library');
      }

      const data = await response.json();
      toast({ tone: 'ready', title: `Added "${title}" to library` });
      onStatusChange?.('in_library', data.library_id);
    } catch (error) {
      toast({ tone: 'error', title: error instanceof Error ? error.message : 'Failed to add to library' });
    } finally {
      setIsAdding(false);
    }
  }, [mediaType, tmdbId, title, status, isAdding, toast, onStatusChange]);

  // Determine button appearance based on status
  const isDisabled = status !== 'not_in_library' || isAdding;

  const ReadyIcon = STATUS.ready_infuse.icon;
  const NeedsTorrentIcon = STATUS.needs_torrent.icon;
  let buttonContent: React.ReactNode;
  let buttonStyle: string;

  if (isAdding) {
    buttonContent = (
      <>
        <Spinner />
        <span>Adding...</span>
      </>
    );
    buttonStyle = 'bg-white/10 text-white cursor-wait';
  } else if (status === 'has_assignment') {
    buttonContent = (
      <>
        <ReadyIcon size={14} aria-hidden="true" />
        <span>{STATUS.ready_infuse.label}</span>
      </>
    );
    buttonStyle = 'bg-accent-green/20 text-accent-green border border-accent-green/30';
  } else if (status === 'in_library') {
    buttonContent = (
      <>
        <NeedsTorrentIcon size={14} aria-hidden="true" />
        <span>{STATUS.needs_torrent.label}</span>
      </>
    );
    buttonStyle = 'bg-white/10 text-text-secondary border border-white/10';
  } else {
    buttonContent = (
      <>
        <PlusIcon />
        <span>Add to Library</span>
      </>
    );
    buttonStyle = 'bg-white/10 hover:bg-white/20 text-white border border-white/20';
  }

  return (
    <button
      onClick={handleClick}
      disabled={isDisabled}
      className={`
        inline-flex items-center justify-center gap-2
        rounded-md
        transition-colors duration-200 motion-reduce:transition-none
        focus:outline-none focus-visible:ring-2 focus-visible:ring-accent
        focus-visible:ring-offset-2 focus-visible:ring-offset-bg-primary
        disabled:cursor-default
        w-full sm:w-auto
        ${sizeStyles[size]}
        ${buttonStyle}
        ${className}
      `}
      aria-label={
        status === 'not_in_library'
          ? `Add ${title} to library`
          : status === 'in_library'
          ? `${title} is in your library`
          : `${title} is ready to stream`
      }
    >
      {buttonContent}
    </button>
  );
}
