import { Play } from 'lucide-react';
import { Media } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';

interface TrailerCardProps {
  youtubeKey: string | null;
  /** TMDB backdrop for the thumbnail. */
  backdropPath: string | null;
  titleName: string;
}

/**
 * The trailer as a backdrop thumbnail that opens YouTube in a new tab, so
 * the page never embeds a third-party player (and the CSP stays as is).
 */
export function TrailerCard({ youtubeKey, backdropPath, titleName }: TrailerCardProps) {
  if (!youtubeKey) return null;
  return (
    <section className="section" aria-labelledby="trailer-h">
      <div className="section-head">
        <h2 className="t-title-3" id="trailer-h">
          Trailer
        </h2>
      </div>
      <a
        className="trailer"
        href={`https://www.youtube.com/watch?v=${encodeURIComponent(youtubeKey)}`}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={`Watch the ${titleName} trailer on YouTube (opens a new tab)`}
      >
        <Media shape="wide" src={buildImageUrl(backdropPath, 'backdrop', 'medium')} alt="" sizes="(min-width: 1024px) 720px, 100vw">
          <span className="play" aria-hidden="true">
            <Play size={26} fill="currentColor" />
          </span>
          <span className="tl" aria-hidden="true">
            Official trailer · opens YouTube
          </span>
        </Media>
      </a>
    </section>
  );
}
