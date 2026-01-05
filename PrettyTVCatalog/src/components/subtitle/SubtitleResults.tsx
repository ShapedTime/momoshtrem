'use client';

import { SpinnerIcon, AlertCircleIcon } from '@/components/ui';
import type { SubtitleSearchResult, SubtitleLanguageCode } from '@/types/subtitle';
import { SUBTITLE_LANGUAGES } from '@/types/subtitle';

// Validate if a string is a valid SubtitleLanguageCode
function isValidLanguageCode(code: string): code is SubtitleLanguageCode {
  return SUBTITLE_LANGUAGES.some((lang) => lang.code === code);
}

interface SubtitleResultsProps {
  results: SubtitleSearchResult[];
  isLoading: boolean;
  error: string | null;
  downloadedLanguages: SubtitleLanguageCode[];
  onDownload: (result: SubtitleSearchResult) => void;
  isDownloading: boolean;
}

function formatDownloadCount(count: number): string {
  if (count >= 1000000) {
    return `${(count / 1000000).toFixed(1)}M`;
  }
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}K`;
  }
  return count.toString();
}

export function SubtitleResults({
  results,
  isLoading,
  error,
  downloadedLanguages,
  onDownload,
  isDownloading,
}: SubtitleResultsProps) {
  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-text-secondary">
        <SpinnerIcon className="w-8 h-8 animate-spin mb-3" />
        <p>Searching OpenSubtitles...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-red-400">
        <AlertCircleIcon className="w-8 h-8 mb-3" />
        <p>{error}</p>
      </div>
    );
  }

  if (results.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-text-secondary">
        <p>No subtitles found. Try different languages.</p>
      </div>
    );
  }

  return (
    <div className="space-y-2 max-h-96 overflow-y-auto">
      {results.map((result) => {
        // Safely check if language is already downloaded
        const isDownloaded =
          isValidLanguageCode(result.language_code) &&
          downloadedLanguages.includes(result.language_code);

        return (
          <div
            key={result.file_id}
            className={`
              p-3 rounded-lg border transition-colors
              ${
                isDownloaded
                  ? 'bg-green-900/20 border-green-700/50'
                  : 'bg-bg-secondary border-border hover:border-border-hover'
              }
            `}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="px-2 py-0.5 bg-accent-blue/20 text-accent-blue text-xs font-medium rounded">
                    {result.language_name}
                  </span>
                  {result.ratings != null && result.ratings > 0 && (
                    <span className="text-xs text-yellow-400">
                      {result.ratings.toFixed(1)} rating
                    </span>
                  )}
                </div>
                <p className="text-sm text-white truncate" title={result.release_name}>
                  {result.release_name}
                </p>
                <div className="flex items-center gap-3 mt-1 text-xs text-text-muted">
                  <span>{formatDownloadCount(result.download_count)} downloads</span>
                  <span>{result.file_name}</span>
                </div>
              </div>

              <div className="flex-shrink-0">
                {isDownloaded ? (
                  <span className="px-3 py-1.5 text-sm text-green-400 bg-green-900/30 rounded-md">
                    Downloaded
                  </span>
                ) : (
                  <button
                    onClick={() => onDownload(result)}
                    disabled={isDownloading}
                    className="
                      px-3 py-1.5 text-sm font-medium rounded-md transition-colors
                      bg-accent-blue text-white
                      hover:bg-accent-blue-hover
                      disabled:opacity-50 disabled:cursor-not-allowed
                    "
                  >
                    {isDownloading ? 'Downloading...' : 'Download'}
                  </button>
                )}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
