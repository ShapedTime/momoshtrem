'use client';

import Image from 'next/image';
import { useRouter } from 'next/navigation';
import { type CSSProperties, type ReactNode, useId, useState } from 'react';
import { ChevronLeft, Film, Star, Tv } from 'lucide-react';
import { IconButton, Media } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { Title } from '@/lib/api/contract';
import { creditLine } from '@/lib/title/facts';
import { formatRuntime, formatYears } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';
import { ClampedText } from './ClampedText';
import { WatchStateLine } from './WatchState';
import { WhyCard } from './WhyCard';

const ENDED = new Set(['Ended', 'Canceled']);

/** "Movie" or "Series · Nippon TV · Anime". */
function eyebrow(title: Title): string {
  if (title.media_type === 'movie') return 'Movie';
  return ['Series', title.networks[0], title.is_anime ? 'Anime' : null].filter(Boolean).join(' · ');
}

/** Rating · year(s) · runtime · genres (· status for shows); empty parts are left out. */
function metaParts(title: Title): Array<{ key: string; node: ReactNode }> {
  const isShow = title.media_type === 'tv';
  const years = isShow ? formatYears(title.year, title.end_year, !ENDED.has(title.status ?? '')) : title.year && String(title.year);
  const runtime = formatRuntime(isShow ? title.episode_run_time : title.runtime);
  const parts: Array<{ key: string; node: ReactNode }> = [];
  if (title.vote_average !== null) {
    parts.push({
      key: 'rating',
      node: (
        <span className="star">
          <Star size={13} fill="currentColor" aria-hidden="true" />
          <span className="sr-only">TMDB rating </span>
          {title.vote_average.toFixed(1)}
        </span>
      ),
    });
  }
  if (years) parts.push({ key: 'years', node: years });
  if (runtime) parts.push({ key: 'runtime', node: runtime });
  if (title.genres.length) parts.push({ key: 'genres', node: title.genres.join(', ') });
  if (isShow && title.status) parts.push({ key: 'status', node: title.status });
  return parts;
}

function BackButton() {
  const router = useRouter();
  return (
    <IconButton variant="glass" aria-label="Back" onClick={() => (window.history.length > 1 ? router.back() : router.push('/'))}>
      <ChevronLeft size={22} aria-hidden="true" />
    </IconButton>
  );
}

/** The backdrop crossfades in over the ambient glow once it loads (HANDOFF §4.3). */
function Backdrop({ src }: { src: string }) {
  const [loaded, setLoaded] = useState(false);
  return <Image src={src} alt="" fill priority sizes="100vw" onLoad={() => setLoaded(true)} className={cx(loaded ? 'opacity-100' : 'opacity-0')} />;
}

interface TitleHeroProps {
  title: Title;
  /** The primary action panel (right column on desktop, under the text on phone). */
  panel: ReactNode;
  /** The ⋯ menu: in the phone bar on phone, beside the eyebrow on desktop. */
  renderMore: (placement: 'phone' | 'desktop') => ReactNode;
}

/**
 * Detail hero for movies and shows: backdrop over an ambient glow in the
 * artwork's colour, the poster (shared element with the card you came
 * from), what it is, and the primary panel.
 */
export function TitleHero({ title, panel, renderMore }: TitleHeroProps) {
  const headingId = useId();
  const backdrop = buildImageUrl(title.backdrop_path, 'backdrop', 'large');
  const poster = buildImageUrl(title.poster_path, 'poster', 'large');
  const credit = creditLine(title);
  const cast = title.cast.slice(0, 3).map((member) => member.name);
  const ambient = title.backdrop_dominant_color ? ({ '--ambient': title.backdrop_dominant_color } as CSSProperties) : undefined;
  const EyebrowIcon = title.media_type === 'movie' ? Film : Tv;

  return (
    <section className="hero detail-hero" style={ambient} aria-labelledby={headingId}>
      <div className="phone-bar">
        <BackButton />
        {renderMore('phone')}
      </div>
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-bg" aria-hidden="true">
        {backdrop && <Backdrop src={backdrop} />}
      </div>
      <div className="hero-scrim" aria-hidden="true" />
      <div className="page detail-grid">
        <div className="poster-col">
          <Media
            shape="poster"
            src={poster}
            alt=""
            priority
            sizes="(min-width: 1440px) 260px, (min-width: 1024px) 240px, 180px"
            viewTransitionName="hero-poster"
          >
            {!poster && (
              <span className="poster-fallback" aria-hidden="true">
                {title.title}
              </span>
            )}
          </Media>
        </div>
        <div className="detail-title">
          <div className="flex items-center justify-between gap-3 lg:min-h-9">
            <p className="eyebrow">
              <EyebrowIcon size={15} aria-hidden="true" />
              {eyebrow(title)}
            </p>
            <div className="max-lg:hidden">{renderMore('desktop')}</div>
          </div>
          <h1 className="t-display" id={headingId}>
            {title.title}
          </h1>
          <p className="meta-line">
            {metaParts(title).map((part) => (
              <span key={part.key}>{part.node}</span>
            ))}
          </p>
          {title.overview && <ClampedText text={title.overview} className="overview" />}
          {(credit || cast.length > 0) && (
            <p className="meta-line credit">
              {credit && <span>{credit}</span>}
              {cast.length > 0 && <span>{cast.join(', ')}</span>}
            </p>
          )}
          <WatchStateLine state={title.watch_state} />
          <WhyCard reason={title.recommendation_reason} />
        </div>
        {panel}
      </div>
    </section>
  );
}
