'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Modal, Input, Button, useToast } from '@/components/ui';
import { TorrentResults } from './TorrentResults';
import { QualityFilter } from './QualityFilter';
import { useTorrentSearch, sortTorrents, filterByQuality, useAddTorrent } from '@/hooks';
import type {
  TorrentSearchContext,
  VideoQuality,
  TorrentSortField,
  SortDirection,
} from '@/types/jackett';
import type { TMDBMetadata } from '@/types/distribyted';

interface TorrentSearchModalProps {
  isOpen: boolean;
  onClose: () => void;
  context: TorrentSearchContext;
}

export function TorrentSearchModal({
  isOpen,
  onClose,
  context,
}: TorrentSearchModalProps) {
  // Search state
  const { results, isLoading, error, search, clearResults } = useTorrentSearch();

  // Add torrent state
  const { addTorrent, isAdding, isAdded, error: addError } = useAddTorrent();

  // Toast notifications
  const { showToast } = useToast();

  // Local UI state
  const [searchQuery, setSearchQuery] = useState(context.query);
  const [qualityFilter, setQualityFilter] = useState<VideoQuality[]>([]);
  const [sortField, setSortField] = useState<TorrentSortField>('seeders');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');

  // Reset state when modal opens with new context
  useEffect(() => {
    if (isOpen) {
      setSearchQuery(context.query);
      setQualityFilter([]);
      setSortField('seeders');
      setSortDirection('desc');
    }
  }, [isOpen, context.query]);

  // Auto-search on open
  useEffect(() => {
    if (isOpen && context.query) {
      search(context.query);
    }
  }, [isOpen, context.query, search]);

  // Handle search submit
  const handleSearch = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (searchQuery.trim()) {
        search(searchQuery.trim());
      }
    },
    [searchQuery, search]
  );

  // Handle close - cleanup
  const handleClose = useCallback(() => {
    clearResults();
    onClose();
  }, [clearResults, onClose]);

  // Apply filtering and sorting
  const processedResults = useMemo(() => {
    const filtered = filterByQuality(results, qualityFilter);
    return sortTorrents(filtered, sortField, sortDirection);
  }, [results, qualityFilter, sortField, sortDirection]);

  // Toggle quality filter
  const toggleQuality = useCallback((quality: VideoQuality) => {
    setQualityFilter((prev) =>
      prev.includes(quality)
        ? prev.filter((q) => q !== quality)
        : [...prev, quality]
    );
  }, []);

  // Handle sort change
  const handleSortChange = useCallback(
    (field: TorrentSortField, direction: SortDirection) => {
      setSortField(field);
      setSortDirection(direction);
    },
    []
  );

  // Build TMDB metadata from context
  const buildMetadata = useCallback((): TMDBMetadata | undefined => {
    // Need tmdbId and year to build valid metadata
    if (!context.tmdbId || !context.year) {
      return undefined;
    }

    // Map mediaType: 'episode' -> 'tv' for the API
    const type = context.mediaType === 'movie' ? 'movie' : 'tv';

    return {
      type,
      tmdb_id: context.tmdbId,
      title: context.title,
      year: context.year,
      season: context.season,
      episode: context.episode,
    };
  }, [context]);

  // Handle adding torrent
  const handleAddTorrent = useCallback(
    async (magnetUri: string) => {
      const metadata = buildMetadata();
      const success = await addTorrent(magnetUri, metadata);

      if (success) {
        showToast('success', 'Torrent added to streaming queue');
      } else {
        showToast('error', addError || 'Failed to add torrent');
      }
    },
    [addTorrent, addError, showToast, buildMetadata]
  );

  // Build modal title based on context
  const modalTitle = useMemo(() => {
    switch (context.mediaType) {
      case 'movie':
        return `Search Torrents: ${context.title}${context.year ? ` (${context.year})` : ''}`;
      case 'episode':
        return `Search: ${context.title} S${String(context.season).padStart(2, '0')}E${String(context.episode).padStart(2, '0')}`;
      case 'tv':
      default:
        return `Search Torrents: ${context.title}`;
    }
  }, [context]);

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title={modalTitle} size="2xl">
      <div className="space-y-4">
        {/* Search Form */}
        <form onSubmit={handleSearch} className="flex gap-2">
          <div className="flex-1">
            <Input
              type="search"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search for torrents..."
              aria-label="Search torrents"
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={isLoading || !searchQuery.trim()}
          >
            Search
          </Button>
        </form>

        {/* Quality Filter */}
        <QualityFilter selected={qualityFilter} onToggle={toggleQuality} />

        {/* Results */}
        <TorrentResults
          results={processedResults}
          isLoading={isLoading}
          error={error}
          sortField={sortField}
          sortDirection={sortDirection}
          onSortChange={handleSortChange}
          onAddTorrent={handleAddTorrent}
          isAdding={isAdding}
          isAdded={isAdded}
        />
      </div>
    </Modal>
  );
}
