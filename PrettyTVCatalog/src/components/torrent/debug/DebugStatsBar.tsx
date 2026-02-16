'use client';

import type { DebugStats } from '@/types/torrent-debug';
import { formatBytes } from '@/types/torrent';

interface DebugStatsBarProps {
  stats: DebugStats;
}

export function DebugStatsBar({ stats }: DebugStatsBarProps) {
  const progress = stats.total_size > 0
    ? ((stats.bytes_completed / stats.total_size) * 100).toFixed(1)
    : '0.0';

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
      <StatCard label="Progress" value={`${progress}%`} />
      <StatCard
        label="Pieces"
        value={`${stats.pieces_complete} / ${stats.pieces_total}`}
      />
      <StatCard
        label="Downloaded"
        value={formatBytes(stats.bytes_completed)}
      />
      <StatCard
        label="Total Size"
        value={formatBytes(stats.total_size)}
      />
      <StatCard label="Active Peers" value={String(stats.active_peers)} />
      <StatCard label="Seeders" value={String(stats.connected_seeders)} />
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-bg-elevated rounded-lg p-3">
      <p className="text-xs text-text-muted uppercase tracking-wider">{label}</p>
      <p className="text-lg font-semibold text-white mt-1">{value}</p>
    </div>
  );
}
