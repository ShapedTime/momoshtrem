import Link from 'next/link';
import { ChevronRight, EllipsisVertical, Search } from 'lucide-react';
import { buttonClassName, IconButton, StatusPill } from '@/components/ui';
import type { EpisodeOrdering, EpisodeSummary } from '@/lib/api/contract';
import { episodeCode, episodeLabel } from '@/lib/title/episodes';
import { formatShortDate } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

export interface EpisodeRowProps {
  episode: EpisodeSummary;
  ordering: EpisodeOrdering;
  /** The first episode of the frontier: amber rule and "Up next". */
  isNext?: boolean;
  /** Mapping review for a needs-review episode (#5); without it the row opens the episode sheet. */
  reviewHref?: string | null;
  /** Missing: find a torrent for this episode. */
  onFind: (episode: EpisodeSummary) => void;
  /** Has a file: the episode sheet (how to play, subtitles, replace, remove). */
  onOpen: (episode: EpisodeSummary) => void;
}

/** DOM id of a row, for jump-to-episode (slice 6): "ep-29". */
export const episodeRowId = (episode: Pick<EpisodeSummary, 'episode_number'>) => `ep-${episode.episode_number}`;

function Pill({ episode }: { episode: EpisodeSummary }) {
  switch (episode.state) {
    case 'downloading':
      return <StatusPill status="downloading" label={`${Math.round(episode.pct ?? 0)}%`} pct={episode.pct ?? 0} />;
    case 'upcoming':
      return episode.air_date ? <StatusPill status="airs_on" label={`Airs ${formatShortDate(episode.air_date)}`} /> : <StatusPill status="upcoming" />;
    default:
      return <StatusPill status={episode.state} />;
  }
}

/**
 * One compact episode row (HANDOFF §5.4, 60 px): number, name, code and
 * runtime, a status pill, and the action that fits the state.
 */
export function EpisodeRow({ episode, ordering, isNext = false, reviewHref, onFind, onOpen }: EpisodeRowProps) {
  const code = episodeCode(episode.season_number, episode.episode_number);
  const sub = [
    episodeLabel(episode, ordering),
    episode.runtime ? `${episode.runtime} min` : null,
    episode.air_date && episode.state !== 'upcoming' ? formatShortDate(episode.air_date) : null,
  ].filter(Boolean);

  let action;
  if (episode.state === 'missing') {
    action = (
      <IconButton size="sm" aria-label={`Find a torrent for ${code}`} onClick={() => onFind(episode)}>
        <Search size={17} aria-hidden="true" />
      </IconButton>
    );
  } else if (episode.state === 'needs_review' && reviewHref) {
    action = (
      <Link href={reviewHref} className={buttonClassName({ variant: 'ghost', size: 'sm', iconOnly: true })} aria-label={`Review the mapping for ${code}`}>
        <ChevronRight size={17} aria-hidden="true" />
      </Link>
    );
  } else if (episode.state === 'upcoming') {
    // Nothing to do until it airs; keep the pills lined up
    action = <span className="w-9 shrink-0" aria-hidden="true" />;
  } else {
    action = (
      <IconButton size="sm" aria-label={`${code} options: how to play, subtitles, replace, remove`} onClick={() => onOpen(episode)}>
        <EllipsisVertical size={17} aria-hidden="true" />
      </IconButton>
    );
  }

  return (
    <li id={episodeRowId(episode)} className={cx('eprow', isNext && 'is-next', episode.state === 'watched' && 'is-watched')}>
      <span className="no" aria-hidden="true">
        {episode.episode_number}
      </span>
      <div className="info">
        {isNext && <span className="t-micro text-accent-text">Up next</span>}
        <span className="name" title={episode.name}>
          <span className="sr-only">{code}: </span>
          {episode.name}
        </span>
        <span className="sub">{sub.join(' · ')}</span>
      </div>
      <div className="end">
        <Pill episode={episode} />
        {action}
      </div>
    </li>
  );
}
