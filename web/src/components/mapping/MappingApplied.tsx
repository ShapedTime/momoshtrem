'use client';

import { useEffect, useId, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Check, EyeOff, Film, Play, Tv } from 'lucide-react';
import { EmptyCat } from '@/components/brand';
import { Button, ButtonLink, StatusPill } from '@/components/ui';
import { useHowToPlay } from '@/hooks/useMappingJob';
import type { AssignmentJobDetail } from '@/lib/api/contract';
import { titleQuery } from '@/lib/api/endpoints';
import { appliedSummary, displayPath, maps, shortTitle, titleHref } from '@/lib/mapping/review';
import { plural } from '@/lib/title/format';

/**
 * "Ready in Infuse.": what Apply wrote, per season (Specials last), what was
 * left out, and the way to Infuse. Also what an applied job shows later.
 */
export function MappingApplied({ detail }: { detail: AssignmentJobDetail }) {
  const { job, proposals } = detail;
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const summary = appliedSummary(proposals);
  const howToPlay = useHowToPlay(job);
  const short = shortTitle(job.item.title);
  const movieFile = job.item.media_type === 'movie' ? proposals.find((p) => p.decision === 'accepted' && maps(p)) : undefined;
  // The title's WebDAV path, for "How to play"
  useQuery(titleQuery(job.item.media_type, job.item.tmdb_id));

  // Apply's button is gone: hand focus to the result instead of the page
  useEffect(() => {
    if (document.activeElement === document.body || document.activeElement === null) heading.current?.focus({ preventScroll: true });
  }, []);

  return (
    <section className="map-done page" aria-labelledby={headingId}>
      <div className="done-art" aria-hidden="true">
        <span className="done-glow" />
        <EmptyCat variant="sit" size={112} />
        <span className="done-check">
          <Check size={18} strokeWidth={2.6} />
        </span>
      </div>
      <p className="t-small font-medium text-fg-3">{job.item.title}</p>
      <h1 ref={heading} tabIndex={-1} className="t-display" id={headingId}>
        Ready in Infuse.
      </h1>
      <p className="t-body-l done-lede text-fg-2">
        {plural(summary.files, 'file')} mapped. They stream while the rest downloads, and show up after Infuse’s next library refresh.
      </p>
      <ul className="panel done-list" aria-label="Mapped to Infuse">
        {movieFile && (
          <li className="list-row">
            <span className="done-ic">
              <Film size={15} aria-hidden="true" />
            </span>
            <div className="grow">
              <strong>{job.item.title}</strong>
              <span className="t-small path text-fg-3">{displayPath(movieFile.file_path, job.torrent.name)}</span>
            </div>
            <StatusPill status="ready" />
          </li>
        )}
        {summary.seasons.map((season) => (
          <li key={season.season} className="list-row">
            <span className="done-ic">
              <Play size={15} fill="currentColor" aria-hidden="true" />
            </span>
            <div className="grow">
              <strong>{season.label}</strong>
              <span className="t-small t-num text-fg-3">{season.range}</span>
            </div>
            <StatusPill status="ready" label={`${season.count} ready`} />
          </li>
        ))}
        {summary.leftOut && (
          <li className="list-row">
            <span className="done-ic done-ic--muted">
              <EyeOff size={15} aria-hidden="true" />
            </span>
            <div className="grow">
              <strong className="text-fg-2">Left out</strong>
              <span className="t-small text-fg-3">{summary.leftOut}</span>
            </div>
          </li>
        )}
      </ul>
      <div className="done-actions">
        <Button variant="primary" size="lg" onClick={howToPlay}>
          <Tv size={20} aria-hidden="true" />
          How to play in Infuse
        </Button>
        <ButtonLink href={titleHref(job)} variant="secondary" size="lg">
          Back to {short}
        </ButtonLink>
      </div>
    </section>
  );
}
