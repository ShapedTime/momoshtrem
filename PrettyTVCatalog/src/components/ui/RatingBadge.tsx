import { StarIcon } from './Icons';

type RatingBadgeVariant = 'card' | 'hero';

interface RatingBadgeProps {
  rating: number;
  variant?: RatingBadgeVariant;
}

/**
 * Displays a rating value with consistent styling.
 * - 'card' variant: Compact badge for overlay on cards (absolute positioned)
 * - 'hero' variant: Inline display with star icon for hero banners
 */
export function RatingBadge({ rating, variant = 'card' }: RatingBadgeProps) {
  const displayRating = rating.toFixed(1);

  if (variant === 'hero') {
    return (
      <div className="flex items-center gap-1 text-accent-yellow">
        <StarIcon size={18} />
        <span className="font-semibold">{displayRating}</span>
      </div>
    );
  }

  // Card variant - positioned absolutely on the card
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
