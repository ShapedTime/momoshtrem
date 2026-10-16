import type { PieceRun, TorrentDetailFile } from '@/lib/api/contract';

/**
 * The piece map (activity.js `drawPieces`): the backend sends run-length
 * piece states in piece order; the canvas draws one square per piece.
 */

/** Cell values, in drawing order of the legend. */
export const PIECE = { missing: 0, have: 1, partial: 2, priority: 3 } as const;
export type PieceCell = (typeof PIECE)[keyof typeof PIECE];

export const PIECE_LABEL: Record<PieceCell, string> = {
  [PIECE.missing]: 'missing',
  [PIECE.have]: 'have',
  [PIECE.partial]: 'downloading',
  [PIECE.priority]: 'prioritised for streaming',
};

/**
 * One cell per piece. `total` (pieces_total) pads a short map with missing
 * pieces and cuts a long one, so the map always has the torrent's size.
 */
export function expandRuns(runs: readonly PieceRun[], total?: number | null): Uint8Array {
  const length = total ?? runs.reduce((sum, run) => sum + Math.max(0, run.count), 0);
  const cells = new Uint8Array(length); // zero-filled: missing
  let at = 0;
  for (const run of runs) {
    const end = Math.min(length, at + Math.max(0, run.count));
    cells.fill(PIECE[run.state], at, end);
    at = end;
    if (at >= length) break;
  }
  return cells;
}

/** How many pieces are on disk. */
export function piecesHave(runs: readonly PieceRun[]): number {
  return runs.reduce((sum, run) => (run.state === 'have' ? sum + run.count : sum), 0);
}

/** The grid for a width: 5 px squares under 520 px, 6 px above, 1 px apart (activity.js). */
export function pieceGrid(width: number, total: number): { cell: number; step: number; cols: number; rows: number } {
  const cell = width < 520 ? 5 : 6;
  const step = cell + 1;
  const cols = Math.max(1, Math.floor((width + 1) / step));
  return { cell, step, cols, rows: Math.ceil(total / cols) };
}

/** The file a piece starts in (files are laid out back to back in torrent order), for the hover tip. */
export function fileAtPiece<F extends Pick<TorrentDetailFile, 'path' | 'size_bytes'>>(files: readonly F[], pieceLength: number, piece: number): F | null {
  const offset = piece * pieceLength;
  let end = 0;
  for (const file of files) {
    end += file.size_bytes;
    if (offset < end) return file;
  }
  return files.at(-1) ?? null;
}
