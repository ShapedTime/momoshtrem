'use client';

import { useCallback, useMemo, useState } from 'react';
import { useTorrents } from '@/hooks/useTorrents';
import {
  TorrentManagementCard,
  TorrentManagementCardSkeleton,
} from '@/components/torrent';

function RefreshIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      className={className}
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
      />
    </svg>
  );
}

function EmptyState() {
  return (
    <div className="text-center py-16">
      <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-bg-elevated mb-4">
        <svg
          className="w-8 h-8 text-text-muted"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1.5}
            d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
          />
        </svg>
      </div>
      <h3 className="text-lg font-medium text-white mb-2">No Active Torrents</h3>
      <p className="text-text-secondary max-w-sm mx-auto">
        Add torrents to movies or TV shows from their detail pages and they will appear here.
      </p>
    </div>
  );
}

function LoadingState() {
  return (
    <div className="space-y-4">
      {Array.from({ length: 3 }).map((_, i) => (
        <TorrentManagementCardSkeleton key={i} />
      ))}
    </div>
  );
}

export default function TorrentsPage() {
  const {
    torrents,
    isLoading,
    error,
    refresh,
    pauseTorrent,
    resumeTorrent,
    removeTorrent,
  } = useTorrents({ autoRefresh: true, refreshInterval: 3000 });

  const [filter, setFilter] = useState<'active' | 'seeding' | 'paused' | null>(null);

  // Filter and sort torrents
  const displayedTorrents = useMemo(() => {
    let filtered = torrents;

    // Apply filter
    if (filter === 'active') {
      filtered = torrents.filter((t) => !t.is_paused && t.progress < 1);
    } else if (filter === 'seeding') {
      filtered = torrents.filter((t) => !t.is_paused && t.progress >= 1);
    } else if (filter === 'paused') {
      filtered = torrents.filter((t) => t.is_paused);
    }

    // Sort: active (not paused) first, then alphabetical
    return [...filtered].sort((a, b) => {
      if (a.is_paused !== b.is_paused) {
        return a.is_paused ? 1 : -1;
      }
      return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
    });
  }, [torrents, filter]);

  const toggleFilter = useCallback((newFilter: 'active' | 'seeding' | 'paused') => {
    setFilter((current) => (current === newFilter ? null : newFilter));
  }, []);

  const handleRefresh = useCallback(async () => {
    await refresh();
  }, [refresh]);

  return (
    <main className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8 lg:py-12">
      {/* Page header */}
      <header className="mb-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl sm:text-3xl font-bold text-white">
              Torrents
            </h1>
            <p className="mt-2 text-text-secondary">
              Manage active downloads and uploads
            </p>
          </div>
          <button
            onClick={handleRefresh}
            disabled={isLoading}
            className="
              flex items-center gap-2
              px-4 py-2 text-sm font-medium
              bg-bg-elevated hover:bg-bg-hover
              text-white rounded-md
              transition-colors
              disabled:opacity-50 disabled:cursor-not-allowed
              focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
            "
          >
            <RefreshIcon
              className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`}
            />
            Refresh
          </button>
        </div>
      </header>

      {/* Stats summary */}
      {torrents.length > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
          <StatCard
            label="Active"
            value={torrents.filter((t) => !t.is_paused && t.progress < 1).length}
            color="blue"
            isSelected={filter === 'active'}
            onClick={() => toggleFilter('active')}
          />
          <StatCard
            label="Seeding"
            value={torrents.filter((t) => !t.is_paused && t.progress >= 1).length}
            color="green"
            isSelected={filter === 'seeding'}
            onClick={() => toggleFilter('seeding')}
          />
          <StatCard
            label="Paused"
            value={torrents.filter((t) => t.is_paused).length}
            color="yellow"
            isSelected={filter === 'paused'}
            onClick={() => toggleFilter('paused')}
          />
          <StatCard label="Total" value={torrents.length} color="white" />
        </div>
      )}

      {/* Error state */}
      {error && (
        <div className="text-center py-8">
          <p className="text-accent-red mb-4">{error}</p>
          <button
            onClick={handleRefresh}
            className="px-4 py-2 bg-white/10 hover:bg-white/20 text-white rounded-md transition-colors"
          >
            Try Again
          </button>
        </div>
      )}

      {/* Loading state */}
      {isLoading && torrents.length === 0 && !error && <LoadingState />}

      {/* Empty state */}
      {!isLoading && !error && torrents.length === 0 && <EmptyState />}

      {/* Torrents list */}
      {torrents.length > 0 && (
        <div className="space-y-4">
          {displayedTorrents.map((torrent) => (
            <TorrentManagementCard
              key={torrent.info_hash}
              torrent={torrent}
              onPause={() => pauseTorrent(torrent.info_hash)}
              onResume={() => resumeTorrent(torrent.info_hash)}
              onRemove={() => removeTorrent(torrent.info_hash)}
            />
          ))}
        </div>
      )}
    </main>
  );
}

function StatCard({
  label,
  value,
  color,
  isSelected,
  onClick,
}: {
  label: string;
  value: number;
  color: 'blue' | 'green' | 'yellow' | 'white';
  isSelected?: boolean;
  onClick?: () => void;
}) {
  const colorClasses = {
    blue: 'text-accent-blue',
    green: 'text-accent-green',
    yellow: 'text-accent-yellow',
    white: 'text-white',
  };

  const isClickable = !!onClick;

  return (
    <div
      role={isClickable ? 'button' : undefined}
      tabIndex={isClickable ? 0 : undefined}
      onClick={onClick}
      onKeyDown={isClickable ? (e) => e.key === 'Enter' && onClick() : undefined}
      className={`
        bg-bg-elevated rounded-lg p-4 transition-all
        ${isClickable ? 'cursor-pointer hover:bg-bg-hover' : ''}
        ${isSelected ? 'ring-2 ring-accent-blue ring-offset-2 ring-offset-bg-primary' : ''}
      `}
    >
      <p className="text-sm text-text-secondary">{label}</p>
      <p className={`text-2xl font-bold ${colorClasses[color]}`}>{value}</p>
    </div>
  );
}
