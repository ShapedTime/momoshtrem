'use client';

import type { DebugReaderInfo } from '@/types/torrent-debug';
import { formatBytes } from '@/types/torrent';

interface ActiveReadersProps {
  readers: DebugReaderInfo[];
}

export function ActiveReaders({ readers }: ActiveReadersProps) {
  if (readers.length === 0) {
    return (
      <div className="text-sm text-text-muted">
        No active readers. Start streaming a file to see reader positions.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {readers.map((reader, idx) => {
        const progress =
          reader.file_length > 0
            ? ((reader.position / reader.file_length) * 100).toFixed(1)
            : '0.0';
        const fileName = reader.file_path.split('/').pop() || reader.file_path;

        return (
          <div key={idx} className="bg-bg-elevated rounded-lg p-3 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm text-white truncate" title={reader.file_path}>
                {fileName}
              </span>
              <span className="text-xs text-text-muted ml-2 whitespace-nowrap">
                Piece {reader.position_piece}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <div className="flex-1 h-1.5 bg-bg-hover rounded-full overflow-hidden">
                <div
                  className="h-full rounded-full bg-cyan-500 transition-all duration-300"
                  style={{ width: `${progress}%` }}
                />
              </div>
              <span className="text-xs text-text-secondary w-20 text-right">
                {formatBytes(reader.position)} / {formatBytes(reader.file_length)}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
