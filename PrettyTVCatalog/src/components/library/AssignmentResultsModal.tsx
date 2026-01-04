'use client';

import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import type { EpisodeMatch, UnmatchedFile, AssignmentSummary } from '@/types/momoshtrem';

interface AssignmentResultsModalProps {
  isOpen: boolean;
  onClose: () => void;
  showTitle: string;
  summary: AssignmentSummary;
  matched: EpisodeMatch[];
  unmatched: UnmatchedFile[];
}

function CheckIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="text-accent-green flex-shrink-0"
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function XIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="text-accent-red flex-shrink-0"
      aria-hidden="true"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function getConfidenceColor(confidence: string): string {
  switch (confidence) {
    case 'high':
      return 'text-accent-green';
    case 'medium':
      return 'text-accent-yellow';
    case 'low':
      return 'text-accent-red';
    default:
      return 'text-text-muted';
  }
}

export function AssignmentResultsModal({
  isOpen,
  onClose,
  showTitle,
  summary,
  matched,
  unmatched,
}: AssignmentResultsModalProps) {
  // Group matched episodes by season
  const episodesBySeason = matched.reduce((acc, ep) => {
    if (!acc[ep.season]) {
      acc[ep.season] = [];
    }
    acc[ep.season].push(ep);
    return acc;
  }, {} as Record<number, EpisodeMatch[]>);

  // Sort seasons and episodes
  const sortedSeasons = Object.keys(episodesBySeason)
    .map(Number)
    .sort((a, b) => a - b);

  for (const season of sortedSeasons) {
    episodesBySeason[season].sort((a, b) => a.episode - b.episode);
  }

  const hasMatches = summary.matched > 0;
  const hasUnmatched = summary.unmatched > 0;
  const matchPercentage = summary.total_files > 0
    ? Math.round((summary.matched / summary.total_files) * 100)
    : 0;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Assignment Results"
      size="xl"
    >
      <div className="space-y-6">
        {/* Summary Header */}
        <div className="text-center pb-4 border-b border-border">
          <h3 className="text-lg font-medium text-white mb-2">{showTitle}</h3>
          <div className="flex items-center justify-center gap-2">
            {hasMatches ? (
              <CheckIcon />
            ) : (
              <XIcon />
            )}
            <span className="text-text-secondary">
              Matched <span className="text-white font-semibold">{summary.matched}</span> of{' '}
              <span className="text-white font-semibold">{summary.total_files}</span> files
              <span className="text-text-muted ml-2">({matchPercentage}%)</span>
            </span>
          </div>
          {summary.skipped > 0 && (
            <p className="text-sm text-text-muted mt-1">
              {summary.skipped} files skipped (non-video)
            </p>
          )}
        </div>

        {/* Matched Episodes */}
        {hasMatches && (
          <div>
            <h4 className="text-sm font-medium text-text-secondary uppercase tracking-wide mb-3">
              Matched Episodes
            </h4>
            <div className="space-y-4 max-h-[40vh] overflow-y-auto pr-2">
              {sortedSeasons.map((season) => (
                <div key={season}>
                  <h5 className="text-sm font-medium text-white mb-2">
                    Season {season}
                  </h5>
                  <div className="space-y-1 ml-4">
                    {episodesBySeason[season].map((ep) => (
                      <div
                        key={ep.episode_id}
                        className="flex items-start gap-2 py-1"
                      >
                        <CheckIcon />
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-sm text-white">
                              E{String(ep.episode).padStart(2, '0')}
                            </span>
                            <span
                              className={`text-xs ${getConfidenceColor(ep.confidence)}`}
                            >
                              {ep.confidence}
                            </span>
                            {ep.resolution && (
                              <span className="text-xs text-text-muted">
                                {ep.resolution}
                              </span>
                            )}
                          </div>
                          <p
                            className="text-xs text-text-muted truncate"
                            title={ep.file_path}
                          >
                            {ep.file_path.split('/').pop()}
                          </p>
                          <p className="text-xs text-text-muted">
                            {formatFileSize(ep.file_size)}
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Unmatched Files */}
        {hasUnmatched && (
          <div>
            <h4 className="text-sm font-medium text-text-secondary uppercase tracking-wide mb-3">
              Unmatched Files
            </h4>
            <div className="space-y-1 max-h-[20vh] overflow-y-auto pr-2">
              {unmatched.map((file, index) => (
                <div
                  key={index}
                  className="flex items-start gap-2 py-1"
                >
                  <XIcon />
                  <div className="flex-1 min-w-0">
                    <p
                      className="text-sm text-text-secondary truncate"
                      title={file.file_path}
                    >
                      {file.file_path.split('/').pop()}
                    </p>
                    <p className="text-xs text-text-muted">
                      {file.reason === 'no_library_episode'
                        ? `S${String(file.season).padStart(2, '0')}E${String(file.episode).padStart(2, '0')} not in library`
                        : file.reason === 'no_match'
                        ? 'Could not identify episode'
                        : file.reason}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* No matches warning */}
        {!hasMatches && (
          <div className="text-center py-4">
            <p className="text-text-secondary">
              No episodes could be matched from this torrent.
            </p>
            <p className="text-sm text-text-muted mt-1">
              The filenames may not contain recognizable episode patterns.
            </p>
          </div>
        )}

        {/* Actions */}
        <div className="flex justify-end pt-4 border-t border-border">
          <Button onClick={onClose} variant="secondary">
            Done
          </Button>
        </div>
      </div>
    </Modal>
  );
}
