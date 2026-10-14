import type { LibraryStatus } from '@/types/momoshtrem';

interface LibraryStatusBadgeProps {
  status: LibraryStatus;
  /** Variant for different display contexts */
  variant?: 'card' | 'inline';
  className?: string;
}

function CheckIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="3"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function PlayIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
      aria-hidden="true"
    >
      <polygon points="5 3 19 12 5 21 5 3" />
    </svg>
  );
}

export function LibraryStatusBadge({
  status,
  variant = 'card',
  className = '',
}: LibraryStatusBadgeProps) {
  // Don't show badge if not in library
  if (status === 'not_in_library') {
    return null;
  }

  const isCard = variant === 'card';

  if (status === 'has_assignment') {
    return (
      <span
        className={`
          inline-flex items-center gap-1
          ${isCard
            ? 'absolute top-2 left-2 px-2 py-1 text-xs font-medium rounded bg-accent-green/90 text-white'
            : 'px-2 py-0.5 text-xs font-medium rounded bg-accent-green/20 text-accent-green'
          }
          ${className}
        `}
      >
        <PlayIcon />
        <span>Ready</span>
      </span>
    );
  }

  // in_library status
  return (
    <span
      className={`
        inline-flex items-center gap-1
        ${isCard
          ? 'absolute top-2 left-2 px-2 py-1 text-xs font-medium rounded bg-white/80 text-black'
          : 'px-2 py-0.5 text-xs font-medium rounded bg-white/10 text-text-secondary'
        }
        ${className}
      `}
    >
      <CheckIcon />
      <span>In Library</span>
    </span>
  );
}
