'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Modal, Input, Button, useToast } from '@/components/ui';
import { TorrentResults } from './TorrentResults';
import { QualityFilter } from './QualityFilter';
import { AssignmentResultsModal } from '@/components/library';
import { useTorrentSearch, sortTorrents, filterByQuality } from '@/hooks';
import { useAddTorrent, isShowResponse } from '@/hooks/useAddToLibrary';
import type {
  TorrentSearchContext,
  VideoQuality,
  TorrentSortField,
  SortDirection,
} from '@/types/jackett';
import type { AddTorrentResponse, AssignmentSummary, EpisodeMatch, UnmatchedFile } from '@/types/momoshtrem';

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

  // Add torrent state (using new momoshtrem flow)
  const { addTorrent, isAdding, isAdded, error: addError, reset: resetAddState } = useAddTorrent();

  // Toast notifications
  const { showToast } = useToast();

  // Local UI state
  const [searchQuery, setSearchQuery] = useState(context.query);
  const [qualityFilter, setQualityFilter] = useState<VideoQuality[]>([]);
  const [sortField, setSortField] = useState<TorrentSortField>('seeders');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');

  // Assignment results modal state (for TV shows)
  const [showAssignmentResults, setShowAssignmentResults] = useState(false);
  const [assignmentResults, setAssignmentResults] = useState<{
    showTitle: string;
    summary: AssignmentSummary;
    matched: EpisodeMatch[];
    unmatched: UnmatchedFile[];
  } | null>(null);

  // Reset state when modal opens with new context
  useEffect(() => {
    if (isOpen) {
      setSearchQuery(context.query);
      setQualityFilter([]);
      setSortField('seeders');
      setSortDirection('desc');
      resetAddState();
    }
  }, [isOpen, context.query, resetAddState]);

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

  // Handle adding torrent with new momoshtrem flow
  const handleAddTorrent = useCallback(
    async (magnetUri: string) => {
      // Determine media type for momoshtrem API
      // 'episode' searches are treated as 'tv' for assignment
      const mediaType = context.mediaType === 'movie' ? 'movie' : 'tv';

      if (!context.tmdbId) {
        showToast('error', 'Missing TMDB ID - cannot add to library');
        return;
      }

      const result = await addTorrent(magnetUri, mediaType, context.tmdbId);

      if (!result) {
        showToast('error', addError || 'Failed to add torrent');
        return;
      }

      // Handle response based on media type
      if (result.media_type === 'movie') {
        // Movie: simple success message
        const message = result.added_to_library
          ? 'Added to library and assigned torrent'
          : 'Torrent assigned to movie';
        showToast('success', message);
      } else {
        // TV Show: show assignment results modal
        if (isShowResponse(result) && result.summary) {
          setAssignmentResults({
            showTitle: context.title,
            summary: result.summary,
            matched: result.matched || [],
            unmatched: result.unmatched || [],
          });
          setShowAssignmentResults(true);

          // Also show a quick toast
          const matchText = `Matched ${result.summary.matched} of ${result.summary.total_files} files`;
          if (result.added_to_library) {
            showToast('success', `Added to library. ${matchText}`);
          } else {
            showToast('success', matchText);
          }
        } else {
          // Fallback for unexpected response
          showToast('success', 'Torrent assigned to show');
        }
      }
    },
    [context.mediaType, context.tmdbId, context.title, addTorrent, addError, showToast]
  );

  // Handle assignment results modal close
  const handleAssignmentResultsClose = useCallback(() => {
    setShowAssignmentResults(false);
    setAssignmentResults(null);
  }, []);

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
    <>
      <Modal isOpen={isOpen} onClose={handleClose} title={modalTitle} size="2xl">
        <div className="space-y-4">
          {/* Search Form */}
          <form onSubmit={handleSearch} className="flex flex-col sm:flex-row gap-2">
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

      {/* Assignment Results Modal (for TV shows) */}
      {assignmentResults && (
        <AssignmentResultsModal
          isOpen={showAssignmentResults}
          onClose={handleAssignmentResultsClose}
          showTitle={assignmentResults.showTitle}
          summary={assignmentResults.summary}
          matched={assignmentResults.matched}
          unmatched={assignmentResults.unmatched}
        />
      )}
    </>
  );
}
