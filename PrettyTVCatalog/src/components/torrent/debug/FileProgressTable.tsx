'use client';

import type { DebugFileInfo } from '@/types/torrent-debug';
import { formatBytes } from '@/types/torrent';

interface FileProgressTableProps {
  files: DebugFileInfo[];
}

export function FileProgressTable({ files }: FileProgressTableProps) {
  if (files.length === 0) return null;

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-text-muted border-b border-bg-hover">
            <th className="pb-2 pr-4">File</th>
            <th className="pb-2 pr-4 text-right">Size</th>
            <th className="pb-2 pr-4 text-right">Downloaded</th>
            <th className="pb-2 pr-4">Progress</th>
            <th className="pb-2 text-right">Pieces</th>
          </tr>
        </thead>
        <tbody>
          {files.map((file) => {
            const pct = (file.progress * 100).toFixed(1);
            const fileName = file.path.split('/').pop() || file.path;
            return (
              <tr
                key={file.path}
                className="border-b border-bg-hover/50 text-text-secondary"
              >
                <td className="py-2 pr-4 max-w-xs truncate" title={file.path}>
                  {fileName}
                </td>
                <td className="py-2 pr-4 text-right whitespace-nowrap">
                  {formatBytes(file.length)}
                </td>
                <td className="py-2 pr-4 text-right whitespace-nowrap">
                  {formatBytes(file.bytes_completed)}
                </td>
                <td className="py-2 pr-4 w-48">
                  <div className="flex items-center gap-2">
                    <div className="flex-1 h-2 bg-bg-hover rounded-full overflow-hidden">
                      <div
                        className="h-full rounded-full transition-all duration-300"
                        style={{
                          width: `${pct}%`,
                          backgroundColor:
                            file.progress >= 1
                              ? '#22c55e'
                              : file.progress > 0
                                ? '#2563eb'
                                : '#1c1c1c',
                        }}
                      />
                    </div>
                    <span className="text-xs w-12 text-right">{pct}%</span>
                  </div>
                </td>
                <td className="py-2 text-right whitespace-nowrap">
                  {file.begin_piece}–{file.end_piece}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
