'use client';

import { useRouter } from 'next/navigation';
import { useRecentlyAired } from '@/hooks/useRecentlyAired';
import { CalendarIcon, RefreshIcon } from '@/components/ui/Icons';
import type { RecentlyAiredEpisode } from '@/types/momoshtrem';

// ============================================================================
// Types
// ============================================================================

interface RecentlyAiredSectionProps {
  className?: string;
}

// ============================================================================
// Helper Functions
// ============================================================================

function formatAirDate(dateString: string): string {
  const date = new Date(dateString);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  // Check if same day
  if (date.toDateString() === today.toDateString()) {
    return 'Today';
  }
  if (date.toDateString() === yesterday.toDateString()) {
    return 'Yesterday';
  }

  // Format as "Mon, Jan 6"
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
}

function formatRelativeTime(dateString: string | null): string {
  if (!dateString) return 'never';

  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

function groupEpisodesByDate(episodes: RecentlyAiredEpisode[]): Map<string, RecentlyAiredEpisode[]> {
  const grouped = new Map<string, RecentlyAiredEpisode[]>();

  for (const ep of episodes) {
    const existing = grouped.get(ep.air_date) || [];
    existing.push(ep);
    grouped.set(ep.air_date, existing);
  }

  return grouped;
}

// ============================================================================
// Sub-Components
// ============================================================================

function SyncButton({
  onSync,
  isSyncing,
  lastSyncTime,
}: {
  onSync: () => void;
  isSyncing: boolean;
  lastSyncTime: string | null;
}) {
  const lastSyncFormatted = formatRelativeTime(lastSyncTime);

  return (
    <button
      onClick={onSync}
      disabled={isSyncing}
      className="flex items-center gap-2 text-sm text-white/60 hover:text-white disabled:opacity-50 transition-colors"
      title={`Last synced: ${lastSyncFormatted}`}
    >
      <RefreshIcon
        size={16}
        className={isSyncing ? 'animate-spin' : ''}
      />
      <span className="hidden sm:inline">
        {isSyncing ? 'Syncing...' : `Synced ${lastSyncFormatted}`}
      </span>
    </button>
  );
}

function DateGroup({
  date,
  episodes,
  onEpisodeClick,
}: {
  date: string;
  episodes: RecentlyAiredEpisode[];
  onEpisodeClick: (ep: RecentlyAiredEpisode) => void;
}) {
  const formattedDate = formatAirDate(date);

  return (
    <div className="mb-4">
      <h3 className="text-sm font-medium text-white/50 mb-2">{formattedDate}</h3>
      <div className="space-y-2">
        {episodes.map((ep) => (
          <EpisodeCard
            key={`${ep.show_id}-${ep.episode_id}`}
            episode={ep}
            onClick={() => onEpisodeClick(ep)}
          />
        ))}
      </div>
    </div>
  );
}

function EpisodeCard({
  episode,
  onClick,
}: {
  episode: RecentlyAiredEpisode;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`
        w-full text-left p-3 rounded-lg bg-white/5 hover:bg-white/10
        transition-colors cursor-pointer
        ${episode.has_assignment ? 'border-l-4 border-green-500' : ''}
      `}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <h4 className="font-medium text-white truncate">{episode.show_title}</h4>
          <p className="text-sm text-white/60 truncate">
            S{String(episode.season_number).padStart(2, '0')}E
            {String(episode.episode_number).padStart(2, '0')}
            {episode.episode_name && ` - ${episode.episode_name}`}
          </p>
        </div>
        {episode.has_assignment && (
          <span className="flex-shrink-0 text-xs bg-green-500/20 text-green-400 px-2 py-1 rounded">
            Ready
          </span>
        )}
      </div>
    </button>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-4">
      <div className="h-4 w-20 bg-white/10 rounded animate-pulse" />
      {[1, 2, 3].map((i) => (
        <div key={i} className="h-16 bg-white/5 rounded-lg animate-pulse" />
      ))}
    </div>
  );
}

// ============================================================================
// Main Component
// ============================================================================

export function RecentlyAiredSection({ className = '' }: RecentlyAiredSectionProps) {
  const router = useRouter();
  const {
    episodes,
    lastSyncTime,
    syncStatus,
    isLoading,
    error,
    refresh,
    triggerSync,
    isSyncing,
  } = useRecentlyAired(30);

  const handleEpisodeClick = (ep: RecentlyAiredEpisode) => {
    router.push(`/tv/${ep.show_tmdb_id}`);
  };

  // Don't render if sync is disabled
  if (syncStatus === 'disabled') {
    return null;
  }

  // Error state
  if (error) {
    return (
      <section className={`mb-8 ${className}`}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold text-white flex items-center gap-2">
            <CalendarIcon size={20} />
            Recently Aired
          </h2>
        </div>
        <div className="text-center py-6 bg-white/5 rounded-lg">
          <p className="text-red-400 mb-2">{error}</p>
          <button
            onClick={refresh}
            className="text-sm text-white/60 hover:text-white transition-colors"
          >
            Try Again
          </button>
        </div>
      </section>
    );
  }

  // Loading state
  if (isLoading) {
    return (
      <section className={`mb-8 ${className}`}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold text-white flex items-center gap-2">
            <CalendarIcon size={20} />
            Recently Aired
          </h2>
        </div>
        <LoadingSkeleton />
      </section>
    );
  }

  // Empty state - don't render section if no episodes
  if (episodes.length === 0) {
    return null;
  }

  // Group episodes by date
  const groupedEpisodes = groupEpisodesByDate(episodes);
  const sortedDates = Array.from(groupedEpisodes.keys()).sort((a, b) => b.localeCompare(a));

  return (
    <section className={`mb-8 ${className}`}>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold text-white flex items-center gap-2">
          <CalendarIcon size={20} />
          Recently Aired
        </h2>
        <SyncButton
          onSync={triggerSync}
          isSyncing={isSyncing}
          lastSyncTime={lastSyncTime}
        />
      </div>

      <div className="max-h-[400px] overflow-y-auto pr-2 scrollbar-thin scrollbar-thumb-white/10 scrollbar-track-transparent">
        {sortedDates.map((date) => (
          <DateGroup
            key={date}
            date={date}
            episodes={groupedEpisodes.get(date)!}
            onEpisodeClick={handleEpisodeClick}
          />
        ))}
      </div>
    </section>
  );
}
