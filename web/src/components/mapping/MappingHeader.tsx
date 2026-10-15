'use client';

import { useState, type ReactNode } from 'react';
import { ChevronLeft, Ellipsis, ListChecks, Magnet, Trash2 } from 'lucide-react';
import { Button, ButtonLink, IconButton, Media, Menu, MenuContent, MenuItem, MenuTrigger, ReleaseName, Sheet } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import type { AssignmentJob } from '@/lib/api/contract';
import { type ReviewCounts, shortTitle, summaryParts, titleHref, torrentFacts } from '@/lib/mapping/review';
import { formatBytes } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

/** "● 9 matched ● 2 need review ● 1 special ● 3 ignored ● 1 unmatched", live as rows change. */
export function MappingSummary({ counts }: { counts: ReviewCounts }) {
  return (
    <div className="map-summary" aria-live="polite">
      {summaryParts(counts).map((part) => (
        <span key={part.key} className={cx('ms', `ms--${part.tone}`, part.count === 0 && 'is-zero')}>
          <i aria-hidden="true" />
          <b className="t-num">{part.count}</b> {part.label}
        </span>
      ))}
    </div>
  );
}

/** The quiet ⋯ with "Discard this mapping": a popover on desktop, a sheet on phone. */
export function MappingMoreMenu({ onDiscard }: { onDiscard: () => void }) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const [sheetOpen, setSheetOpen] = useState(false);
  const icon = <Ellipsis size={20} aria-hidden="true" />;
  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>
          <IconButton size="sm" aria-label="More mapping actions">
            {icon}
          </IconButton>
        </MenuTrigger>
        <MenuContent aria-label="More mapping actions">
          {/* After the menu has closed, so the confirm dialog takes focus */}
          <MenuItem danger icon={<Trash2 size={18} aria-hidden="true" />} onSelect={() => window.setTimeout(onDiscard, 0)}>
            Discard this mapping…
          </MenuItem>
        </MenuContent>
      </Menu>
    );
  }
  return (
    <>
      <IconButton aria-label="More mapping actions" aria-haspopup="dialog" onClick={() => setSheetOpen(true)}>
        {icon}
      </IconButton>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="More" size="narrow">
        <Button
          variant="danger-quiet"
          block
          onClick={() => {
            setSheetOpen(false);
            window.setTimeout(onDiscard, 0);
          }}
        >
          <Trash2 size={18} aria-hidden="true" /> Discard this mapping…
        </Button>
      </Sheet>
    </>
  );
}

interface MappingHeaderProps {
  job: AssignmentJob;
  /** The live summary (review only). */
  summary?: ReactNode;
  /** The ⋯ menu, while the job is open. */
  more?: ReactNode;
}

/**
 * Back to the title, poster, "Check the episodes", the release name (wrapping
 * only after separators), what's known about the torrent and the summary.
 */
export function MappingHeader({ job, summary, more }: MappingHeaderProps) {
  const { item } = job;
  const poster = buildImageUrl(item.poster_path, 'poster', 'small');
  const facts = torrentFacts(job, formatBytes);
  const movie = item.media_type === 'movie';
  return (
    <header className="map-head page">
      <div className="map-head-top">
        <ButtonLink href={titleHref(job)} variant="ghost" size="sm" className="map-back">
          <ChevronLeft size={18} aria-hidden="true" />
          <span>
            <span className="sr-only">Back to </span>
            {shortTitle(item.title)}
          </span>
        </ButtonLink>
        {more}
      </div>
      <div className="map-head-main">
        <div className="map-poster">
          <Media shape="poster" src={poster} alt="" sizes="96px">
            {!poster && (
              <span className="poster-fallback" aria-hidden="true">
                {shortTitle(item.title)}
              </span>
            )}
          </Media>
        </div>
        <div className="map-titles">
          <p className="map-eyebrow">
            <ListChecks size={15} aria-hidden="true" />
            <span>
              {movie ? 'File mapping' : 'Episode mapping'} · {item.title}
            </span>
          </p>
          <h1 className="map-h1">{movie ? 'Check the files' : 'Check the episodes'}</h1>
          <div className="map-torrent">
            <Magnet size={15} aria-hidden="true" />
            <ReleaseName name={job.torrent.name} />
          </div>
          {facts.length > 0 && (
            <p className="map-facts dot-sep">
              {facts.map((fact, i) => (
                <span key={`${i}-${fact}`}>{fact}</span>
              ))}
            </p>
          )}
          {summary}
        </div>
      </div>
    </header>
  );
}
