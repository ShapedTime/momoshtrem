'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Modal, Button, useToast } from '@/components/ui';
import { LanguageSelector } from './LanguageSelector';
import { SubtitleResults } from './SubtitleResults';
import { SubtitleList } from './SubtitleList';
import { useSubtitles, useSubtitleSearch } from '@/hooks';
import type {
  SubtitleSearchContext,
  SubtitleSearchResult,
  SubtitleLanguageCode,
} from '@/types/subtitle';

interface SubtitleSearchModalProps {
  isOpen: boolean;
  onClose: () => void;
  context: SubtitleSearchContext;
}

export function SubtitleSearchModal({
  isOpen,
  onClose,
  context,
}: SubtitleSearchModalProps) {
  // Subtitle management hook
  const {
    subtitles,
    isLoading: isLoadingSubtitles,
    downloadSubtitle,
    deleteSubtitle,
    refresh: refreshSubtitles,
  } = useSubtitles(context.mediaType, context.itemId);

  // Search hook
  const { results, isSearching, error, search, clearResults } =
    useSubtitleSearch(context);

  // Toast notifications
  const { showToast } = useToast();

  // Local state
  const [selectedLanguages, setSelectedLanguages] = useState<SubtitleLanguageCode[]>(['en']);
  const [isDownloading, setIsDownloading] = useState(false);

  // Downloaded language codes for highlighting
  const downloadedLanguages = useMemo(
    () => subtitles.map((s) => s.language_code as SubtitleLanguageCode),
    [subtitles]
  );

  // Reset state when modal opens
  useEffect(() => {
    if (isOpen) {
      setSelectedLanguages(['en']);
    }
  }, [isOpen]);

  // Auto-search on modal open (with default language only)
  useEffect(() => {
    if (isOpen) {
      // Search with default 'en' language on open, not on every language change
      search(['en']);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  // Handle search button click
  const handleSearch = useCallback(() => {
    if (selectedLanguages.length > 0) {
      search(selectedLanguages);
    }
  }, [selectedLanguages, search]);

  // Handle language change
  const handleLanguageChange = useCallback(
    (languages: SubtitleLanguageCode[]) => {
      setSelectedLanguages(languages);
    },
    []
  );

  // Handle download
  const handleDownload = useCallback(
    async (result: SubtitleSearchResult) => {
      setIsDownloading(true);

      try {
        const subtitle = await downloadSubtitle({
          item_type: context.mediaType,
          item_id: context.itemId,
          file_id: result.file_id,
          language_code: result.language_code,
          language_name: result.language_name,
        });

        if (subtitle) {
          showToast('success', `Downloaded ${result.language_name} subtitles`);
          refreshSubtitles();
        } else {
          showToast('error', 'Failed to download subtitle');
        }
      } catch {
        showToast('error', 'Failed to download subtitle');
      } finally {
        setIsDownloading(false);
      }
    },
    [context.mediaType, context.itemId, downloadSubtitle, showToast, refreshSubtitles]
  );

  // Handle delete
  const handleDelete = useCallback(
    async (subtitleId: number): Promise<boolean> => {
      const success = await deleteSubtitle(subtitleId);
      if (success) {
        showToast('success', 'Subtitle deleted');
      } else {
        showToast('error', 'Failed to delete subtitle');
      }
      return success;
    },
    [deleteSubtitle, showToast]
  );

  // Handle close
  const handleClose = useCallback(() => {
    clearResults();
    onClose();
  }, [clearResults, onClose]);

  // Build modal title
  const modalTitle = useMemo(() => {
    if (context.mediaType === 'episode') {
      return `Subtitles: ${context.title} S${String(context.season).padStart(2, '0')}E${String(context.episode).padStart(2, '0')}`;
    }
    return `Subtitles: ${context.title}`;
  }, [context]);

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title={modalTitle} size="2xl">
      <div className="space-y-6">
        {/* Downloaded Subtitles */}
        <SubtitleList
          subtitles={subtitles}
          isLoading={isLoadingSubtitles}
          onDelete={handleDelete}
        />

        {/* Divider */}
        <div className="border-t border-border" />

        {/* Language Selector */}
        <LanguageSelector
          selected={selectedLanguages}
          onChange={handleLanguageChange}
        />

        {/* Search Button */}
        <Button
          onClick={handleSearch}
          disabled={isSearching || selectedLanguages.length === 0}
          variant="primary"
          className="w-full"
        >
          {isSearching ? 'Searching...' : 'Search OpenSubtitles'}
        </Button>

        {/* Results */}
        {(results.length > 0 || isSearching || error) && (
          <SubtitleResults
            results={results}
            isLoading={isSearching}
            error={error}
            downloadedLanguages={downloadedLanguages}
            onDownload={handleDownload}
            isDownloading={isDownloading}
          />
        )}
      </div>
    </Modal>
  );
}
