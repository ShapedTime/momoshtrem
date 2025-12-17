/**
 * Format runtime in minutes to "Xh Ym" format.
 * @param minutes - Runtime in minutes (e.g., 142)
 * @returns Formatted string (e.g., "2h 22m") or null if invalid
 */
export function formatRuntime(minutes: number | null): string | null {
  if (!minutes || minutes <= 0) return null;
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  if (hours === 0) return `${mins}m`;
  if (mins === 0) return `${hours}h`;
  return `${hours}h ${mins}m`;
}

/**
 * Extract year from date string.
 * @param dateStr - ISO date string (e.g., "2024-03-15")
 * @returns Year as number or null
 */
export function extractYear(dateStr: string | null | undefined): number | null {
  if (!dateStr) return null;
  const year = parseInt(dateStr.substring(0, 4), 10);
  return isNaN(year) ? null : year;
}

/**
 * Format date string to readable format.
 * @param dateStr - ISO date string
 * @returns Formatted date (e.g., "March 15, 2024") or null
 */
export function formatReleaseDate(dateStr: string | null | undefined): string | null {
  if (!dateStr) return null;
  try {
    return new Date(dateStr).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  } catch {
    return null;
  }
}
