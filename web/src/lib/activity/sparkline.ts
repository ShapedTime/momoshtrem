import { formatRate } from './format';

/**
 * The Download tile's sparkline (activity.js `spark`): the last minute of
 * total download speed on a fixed 0–40 MB/s axis, so the slope is honest and
 * two loads of the page compare.
 */

/** The axis top, bytes per second (decimal, as speeds read: "40 MB/s"). */
export const SPARK_SCALE_BPS = 40_000_000;
export const SPARK_WIDTH = 120;
export const SPARK_HEIGHT = 40;

/** A sample's height on the fixed scale, clamped to the box. */
export function sparkY(bps: number, height = SPARK_HEIGHT): number {
  const ratio = Math.min(1, Math.max(0, bps / SPARK_SCALE_BPS));
  return height - ratio * height;
}

/** The SVG paths: the line and the area under it. Fewer than two samples draw a flat line. */
export function sparkPaths(samples: readonly number[], width = SPARK_WIDTH, height = SPARK_HEIGHT): { line: string; area: string } {
  const values = samples.length >= 2 ? samples : [samples[0] ?? 0, samples[0] ?? 0];
  const points = values.map((value, i) => [(i / (values.length - 1)) * width, sparkY(value, height)]);
  const line = points.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)}`).join('');
  return { line, area: `${line}L${width} ${height}L0 ${height}Z` };
}

/** The sample under a pointer at `x` of `width`. */
export function sampleAt(x: number, width: number, count: number): number {
  if (count <= 1 || width <= 0) return 0;
  return Math.round((Math.min(width, Math.max(0, x)) / width) * (count - 1));
}

/** The tooltip: "Now · 29.8 MB/s", "12 s ago · 6.4 MB/s". */
export function sparkTip(index: number, samples: readonly number[], intervalSeconds: number): string {
  const ago = (samples.length - 1 - index) * intervalSeconds;
  return `${ago === 0 ? 'Now' : `${ago} s ago`} · ${formatRate(samples[index] ?? 0)}`;
}
