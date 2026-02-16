/**
 * Torrent debug visualization types.
 * Maps to momoshtrem /api/torrents/:hash/debug API response.
 */

export interface TorrentDebugInfo {
  info_hash: string;
  name: string;
  piece_length: number;
  num_pieces: number;
  pieces: string; // base64 encoded
  files: DebugFileInfo[];
  readers: DebugReaderInfo[];
  stats: DebugStats;
}

export interface DebugFileInfo {
  path: string;
  length: number;
  bytes_completed: number;
  progress: number;
  begin_piece: number;
  end_piece: number;
}

export interface DebugReaderInfo {
  file_path: string;
  position: number;
  position_piece: number;
  file_length: number;
}

export interface DebugStats {
  pieces_complete: number;
  pieces_total: number;
  active_peers: number;
  connected_seeders: number;
  bytes_completed: number;
  total_size: number;
}

/** Priority levels matching anacrolix/torrent iota values. */
export enum PiecePriority {
  None = 0,
  Normal = 1,
  Readahead = 2,
  Next = 3,
  Now = 4,
}

export const PRIORITY_LABELS: Record<number, string> = {
  [PiecePriority.None]: 'None',
  [PiecePriority.Normal]: 'Normal',
  [PiecePriority.Readahead]: 'Readahead',
  [PiecePriority.Next]: 'Next',
  [PiecePriority.Now]: 'Now',
};

/** Decoded piece state. */
export interface PieceState {
  complete: boolean;
  partial: boolean;
  priority: number;
}

/** Decode base64-encoded piece bitmap into array of PieceState. */
export function decodePieces(b64: string): PieceState[] {
  const binary = atob(b64);
  const pieces: PieceState[] = new Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    const b = binary.charCodeAt(i);
    pieces[i] = {
      complete: (b & 0x80) !== 0,
      partial: (b & 0x40) !== 0,
      priority: b & 0x07,
    };
  }
  return pieces;
}
