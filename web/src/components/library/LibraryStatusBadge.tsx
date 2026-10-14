import { StatusPill } from '@/components/ui';
import type { LibraryStatus } from '@/types/momoshtrem';

interface LibraryStatusBadgeProps {
  status: LibraryStatus;
  /** Variant for different display contexts */
  variant?: 'card' | 'inline';
  className?: string;
}

/**
 * Library state as a status pill, using the shared vocabulary (lib/status):
 * an assigned title is "Ready"; one without a torrent "Needs a torrent".
 */
export function LibraryStatusBadge({ status, variant = 'card', className = '' }: LibraryStatusBadgeProps) {
  if (status === 'not_in_library') {
    return null;
  }

  const isCard = variant === 'card';
  // Cards are narrow (104 px on phone), so they use the short label, as in the design
  const pill =
    status === 'has_assignment' ? (
      <StatusPill status="ready" onArt={isCard} />
    ) : (
      <StatusPill status="needs_torrent" label={isCard ? 'No torrent' : undefined} onArt={isCard} />
    );

  if (isCard) {
    return <span className={`absolute top-2 left-2 z-[2] ${className}`}>{pill}</span>;
  }
  return className ? <span className={className}>{pill}</span> : pill;
}
