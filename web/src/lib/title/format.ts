import type { Timestamp } from '@/lib/api/contract';

/** Display formats for title pages: runtimes, sizes, dates, years and languages. */

/** 167 → "2h 47m"; 45 → "45m". Null for unknown or zero. */
export function formatRuntime(minutes: number | null): string | null {
  if (!minutes || minutes <= 0) return null;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours === 0) return `${rest}m`;
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`;
}

/** Decimal units, as Infuse and the indexers show them: 18_400_000_000 → "18.4 GB". */
export function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = Math.max(0, bytes);
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

/** 2_300_000 → "2.3 MB/s". */
export function formatSpeed(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`;
}

/** 1204 → "1,204". */
export function formatNumber(value: number): string {
  return value.toLocaleString('en-US');
}

/** Vote counts: 6800 → "6.8k", 37_000 → "37k", 512 → "512". */
export function formatCount(value: number): string {
  if (value < 1000) return String(value);
  const thousands = value / 1000;
  return `${thousands < 10 ? Number(thousands.toFixed(1)) : Math.round(thousands)}k`;
}

const dateFormat = (options: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat('en-US', options);
const FULL = dateFormat({ month: 'short', day: 'numeric', year: 'numeric' });
const DAY_MONTH = dateFormat({ month: 'short', day: 'numeric' });
const MONTH_YEAR = dateFormat({ month: 'short', year: 'numeric' });

/** "Mar 14, 2024". */
export function formatDate(timestamp: Timestamp): string {
  return FULL.format(new Date(timestamp));
}

/** "Oct 2" this year, "Oct 2, 2025" otherwise. */
export function formatShortDate(timestamp: Timestamp, now: Date = new Date()): string {
  const date = new Date(timestamp);
  return date.getFullYear() === now.getFullYear() ? DAY_MONTH.format(date) : FULL.format(date);
}

/** "Jan 2023". */
export function formatMonthYear(timestamp: Timestamp): string {
  return MONTH_YEAR.format(new Date(timestamp));
}

/** Shows: "2016–2020", "2023–" while it runs, or just the year. */
export function formatYears(year: number | null, endYear: number | null, running: boolean): string | null {
  if (year === null) return null;
  if (endYear !== null && endYear !== year) return `${year}–${endYear}`;
  return running ? `${year}–` : String(year);
}

/** ISO 639-1 → English name ("ja" → "Japanese"); the code itself when unknown. */
export function languageName(code: string): string {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code.toUpperCase();
  } catch {
    return code.toUpperCase();
  }
}

/** 1 → "1 episode", 2 → "2 episodes". */
export function plural(count: number, one: string, many = `${one}s`): string {
  return `${formatNumber(count)} ${count === 1 ? one : many}`;
}
