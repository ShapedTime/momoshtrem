import type { MediaType } from '@/lib/api/contract';

/**
 * The Filters sheet's size range (#93): two thumbs on a fixed log scale,
 * per episode for TV and the whole release for movies, as `min_size_gb` and
 * `max_size_gb` (PLAN §6 "Search"). A thumb at its end of the track sets no
 * bound, so the ends read "No minimum" and "No maximum".
 */

export interface SizeScale {
  /** GB at each end of the track. */
  min: number;
  max: number;
}

/** 100 MB to 20 GB per episode; 500 MB to 100 GB for a movie. */
export const SIZE_SCALE: Record<MediaType, SizeScale> = {
  tv: { min: 0.1, max: 20 },
  movie: { min: 0.5, max: 100 },
};

/** Positions on the track: 0 is the low end, `SIZE_STEPS` the high end. */
export const SIZE_STEPS = 100;

/** Two significant figures: 0.105 → 0.11, 4.13 → 4.1, 12.6 → 13. */
const roundSize = (gb: number) => Number(gb.toPrecision(2));

/** A position on the track as GB. */
export function sizeAt(position: number, scale: SizeScale): number {
  const clamped = Math.min(SIZE_STEPS, Math.max(0, position));
  return roundSize(scale.min * (scale.max / scale.min) ** (clamped / SIZE_STEPS));
}

/** GB as the nearest position on the track, clamped to its ends. */
export function positionOf(gb: number, scale: SizeScale): number {
  if (gb <= scale.min) return 0;
  if (gb >= scale.max) return SIZE_STEPS;
  return Math.round((Math.log(gb / scale.min) / Math.log(scale.max / scale.min)) * SIZE_STEPS);
}

/** "200 MB", "4 GB", "4.1 GB", "20 GB" (decimal, like the cards). */
export function sizeLabel(gb: number): string {
  if (gb < 1) return `${Math.round(gb * 1000)} MB`;
  return `${Number(gb.toFixed(gb < 10 ? 1 : 0))} GB`;
}

interface SizeBounds {
  min_size_gb?: number;
  max_size_gb?: number;
}

/** The bounds as track positions: no bound sits at its end. */
export function sizeRange(bounds: SizeBounds, scale: SizeScale): [number, number] {
  return [
    bounds.min_size_gb === undefined ? 0 : positionOf(bounds.min_size_gb, scale),
    bounds.max_size_gb === undefined ? SIZE_STEPS : positionOf(bounds.max_size_gb, scale),
  ];
}

/** Track positions as bounds: a thumb at its end drops its bound. */
export function withSizeRange<T extends SizeBounds>(filters: T, [low, high]: [number, number], scale: SizeScale): T {
  const next = { ...filters };
  delete next.min_size_gb;
  delete next.max_size_gb;
  if (low > 0) next.min_size_gb = sizeAt(low, scale);
  if (high < SIZE_STEPS) next.max_size_gb = sizeAt(high, scale);
  return next;
}

/** A thumb's words: its size, or "No minimum" / "No maximum" at its end. */
export function rangeText(position: number, thumb: 'min' | 'max', scale: SizeScale): string {
  if (thumb === 'min' && position <= 0) return 'No minimum';
  if (thumb === 'max' && position >= SIZE_STEPS) return 'No maximum';
  return sizeLabel(sizeAt(position, scale));
}
