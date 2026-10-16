import Link from 'next/link';
import type { CSSProperties, Ref } from 'react';
import { ChevronRight, EllipsisVertical, Search } from 'lucide-react';
import { buttonClassName, IconButton, Media, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { EpisodeOrdering, EpisodeSummary } from '@/lib/api/contract';
import { episodeCode, episodeLabel } from '@/lib/title/episodes';
import { formatShortDate } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

export interface EpisodeRowProps {
  episode: EpisodeSummary;
  ordering: EpisodeOrdering;
  /** Up next (Trakt's next episode, else the frontier's first): amber rule and "Up next". */
  isNext?: boolean;
  /** Mapping review for a needs-review episode (#5); without it the row opens the episode sheet. */
  reviewHref?: string | null;
  /** Missing: find a torrent for this episode. */
  onFind: (episode: EpisodeSummary) => void;
  /** The episode sheet: with a file, how to play, subtitles, replace and remove; watched without one, find a torrent. */
  onOpen: (episode: EpisodeSummary) => void;
  /** "With stills" density: a 16:9 still instead of the number (92 px rows). */
  rich?: boolean;
  /** Just jumped to: a short amber flash. */
  flash?: boolean;
  /** Virtualised lists: position, size and measuring. */
  style?: CSSProperties;
  position?: { index: number; of: number };
  ref?: Ref<HTMLLIElement>;
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
export function EpisodeRow({ episode, ordering, isNext = false, reviewHref, onFind, onOpen, rich = false, flash = false, style, position, ref }: EpisodeRowProps) {
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
    // Watched wins the state (B5), so a watched row may or may not have a file
    const options = episode.webdav_path ? 'how to play, subtitles, replace, remove' : 'find a torrent';
    action = (
      <IconButton size="sm" aria-label={`${code} options: ${options}`} onClick={() => onOpen(episode)}>
        <EllipsisVertical size={17} aria-hidden="true" />
      </IconButton>
    );
  }

  return (
    <li
      ref={ref}
      id={episodeRowId(episode)}
      // Focusable from script only: a jump or an expanded run moves focus here, so Tab goes on from this row
      tabIndex={-1}
      className={cx('eprow', rich && 'eprow--rich', isNext && 'is-next', episode.state === 'watched' && 'is-watched', flash && 'flash')}
      style={style}
      data-index={position?.index}
      aria-setsize={position?.of}
      aria-posinset={position ? position.index + 1 : undefined}
    >
      {rich ? (
        <Media shape="wide" src={buildImageUrl(episode.still_path, 'still', 'large')} alt="" sizes="136px" />
      ) : (
        <span className="no" aria-hidden="true">
          {episode.episode_number}
        </span>
      )}
      <div className="info">
        {isNext && <span className="t-micro text-accent-text">Up next</span>}
        <span className="name" title={episode.name}>
          <span className="sr-only">{code}: </span>
          {rich && <span className="t-num text-fg-3">{episode.episode_number}. </span>}
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
