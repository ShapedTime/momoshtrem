'use client';

import Image from 'next/image';
import { type CSSProperties, type ReactNode, useEffect, useId, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Bookmark, Play, Star, Zap } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { PhoneBar } from '@/components/shell/PhoneBar';
import { useOverArt } from '@/components/shell/ShellContext';
import { StageButton } from '@/components/title/StageButton';
import { Button, ButtonLink, Skeleton } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { useTitleActions } from '@/hooks/useTitle';
import type { Title, UpNextItem } from '@/lib/api/contract';
import { titleQuery } from '@/lib/api/endpoints';
import type { ForYouPick, HeroPickSource } from '@/lib/for-you/hero';
import { useSheet } from '@/lib/sheets/useSheet';
import { formatRuntime, formatYears } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import { cx } from '@/lib/utils/cx';

const ENDED = new Set(['Ended', 'Canceled']);

/** Rating · year(s) · runtime · two genres; empty parts are left out. */
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
  if (runtime) parts.push({ key: 'runtime', node: isShow ? `~${runtime}` : runtime });
  if (title.genres.length) parts.push({ key: 'genres', node: title.genres.slice(0, 2).join(', ') });
  return parts;
}

function KickerIcon({ source }: { source: HeroPickSource }) {
  switch (source) {
    case 'trending':
      return <Zap size={16} aria-hidden="true" />;
    case 'up_next':
      return <Play size={16} aria-hidden="true" />;
    case 'watchlist':
      return <Bookmark size={16} aria-hidden="true" />;
    case 'recommendation':
      return <CatAvatar size={22} />;
  }
}

/** The backdrop fades in over the ambient glow once it loads; a new pick's backdrop fades in over the last one's glow. */
function Backdrop({ src }: { src: string }) {
  const [loaded, setLoaded] = useState(false);
  return <Image src={src} alt="" fill priority sizes="100vw" onLoad={() => setLoaded(true)} className={cx(loaded ? 'opacity-100' : 'opacity-0')} />;
}

/** A gentle parallax on the backdrop as the page scrolls; none under reduced motion. */
function useParallax() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const layer = ref.current;
    if (!layer || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    let frame = 0;
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        layer.style.transform = `translate3d(0, ${Math.min(window.scrollY, 600) * 0.18}px, 0)`;
      });
    };
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      window.removeEventListener('scroll', onScroll);
      cancelAnimationFrame(frame);
      layer.style.transform = '';
    };
  }, []);
  return ref;
}

/**
 * The pick's main button. An Up next pick plays its episode (the
 * infuse-setup sheet with that file); any other follows the title's stage,
 * as on its page: Add to library, Find a torrent, View download, How to play.
 */
function HeroPrimary({ title, episode }: { title: Title; episode: UpNextItem | undefined }) {
  const sheets = useSheet();
  const actions = useTitleActions(title);

  if (episode?.availability === 'ready') {
    return (
      <Button variant="primary" size="lg" onClick={() => sheets.open('infuse-setup', infuseSetupParams({ path: episode.webdav_path, url: episode.webdav_url }))}>
        <Play size={18} fill="currentColor" aria-hidden="true" /> Play in Infuse
      </Button>
    );
  }

  const { library } = title;
  const source: Record<string, string> = { [title.media_type === 'movie' ? 'movie' : 'show']: String(title.tmdb_id) };
  const onAction = (intent: StageIntent) => {
    if (intent === 'add') actions.add();
    else if (intent === 'find') sheets.open('torrent', source);
    else if (intent === 'hdrezka') sheets.open('hdrezka', source);
    else if (intent === 'how_to_play') sheets.open('infuse-setup', infuseSetupParams({ path: library.webdav_path, url: library.webdav_url }));
  };
  return <StageButton spec={primaryActionFor(title).primary} onAction={onAction} busy={actions.adding ? 'add' : null} />;
}

interface HomeHeroProps {
  /** For the phone bar's account menu (the hero draws its own bar over the art). */
  username: string;
  /** One to three, stable for the day (lib/for-you/hero.ts). */
  picks: ForYouPick[];
  /** Up next, so an Up next pick plays its episode. */
  upNext?: UpNextItem[];
}

/**
 * For You's hero (HANDOFF §4.3): the pick's backdrop over a glow in its
 * colour, what it is and why, its next step and Details, and a pager. The
 * top bar is transparent over it; the backdrop drifts as you scroll.
 */
export function HomeHero({ username, picks, upNext = [] }: HomeHeroProps) {
  const headingId = useId();
  const [index, setIndex] = useState(0);
  const pick = picks[Math.min(index, picks.length - 1)];
  const query = useQuery(titleQuery(pick.media_type, pick.tmdb_id));
  const parallax = useParallax();
  useOverArt(!query.isError);

  // A title that won't load leaves no hero rather than a broken one
  if (query.isError) return null;

  const title = query.data;
  const backdrop = title ? buildImageUrl(title.backdrop_path, 'backdrop', 'large') : null;
  const ambient = title?.backdrop_dominant_color ? ({ '--ambient': title.backdrop_dominant_color } as CSSProperties) : undefined;
  const episode = pick.source === 'up_next' ? upNext.find((item) => item.show.tmdb_id === pick.tmdb_id) : undefined;
  const text = pick.reason || title?.overview || '';

  return (
    <section className="hero home-hero" style={ambient} aria-labelledby={headingId}>
      <PhoneBar username={username} />
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-bg" ref={parallax} aria-hidden="true">
        {backdrop && <Backdrop key={backdrop} src={backdrop} />}
      </div>
      <div className="hero-scrim" aria-hidden="true" />
      <div className="page">
        <div className="hero-content">
          <p className="eyebrow">
            <KickerIcon source={pick.source} />
            <span>{pick.kicker}</span>
          </p>
          {title ? (
            <>
              <h2 className="t-display-xl" id={headingId}>
                {title.title}
              </h2>
              <p className="meta-line">
                {metaParts(title).map((part) => (
                  <span key={part.key}>{part.node}</span>
                ))}
              </p>
              {text && <p className={cx('overview', pick.reason ? 'why-line t-body-l' : 'clamp-3')}>{text}</p>}
              <div className="actions">
                <HeroPrimary title={title} episode={episode} />
                <ButtonLink href={titleHref(title.media_type, title.tmdb_id)} variant="glass" size="lg">
                  Details
                </ButtonLink>
              </div>
            </>
          ) : (
            <HeroContentSkeleton headingId={headingId} />
          )}
          {picks.length > 1 && (
            <div className="hero-pager" role="group" aria-label="Picks">
              {picks.map((p, i) => (
                <button
                  key={`${p.media_type}:${p.tmdb_id}`}
                  type="button"
                  aria-pressed={i === index}
                  aria-label={`Pick ${i + 1} of ${picks.length}`}
                  onClick={() => setIndex(i)}
                >
                  <i aria-hidden="true" />
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

/** The title, facts, overview and buttons while the pick's title loads, at their final sizes. */
function HeroContentSkeleton({ headingId }: { headingId: string }) {
  return (
    <div className="skel-delay flex flex-col gap-4" role="status" aria-live="polite">
      <h2 className="sr-only" id={headingId}>
        Loading the pick…
      </h2>
      <Skeleton height={56} width="min(420px, 80%)" />
      <Skeleton shape="line" width={220} />
      <Skeleton shape="line" width="min(520px, 100%)" />
      <div className="actions">
        <Skeleton height={52} width={180} className="rounded-md" />
        <Skeleton height={52} width={112} className="rounded-md" />
      </div>
    </div>
  );
}

/** The hero's box while For You works out its picks: the same size, so nothing jumps when they arrive. */
export function HomeHeroSkeleton({ username }: { username: string }) {
  const headingId = useId();
  useOverArt();
  return (
    <section className="hero home-hero" aria-labelledby={headingId}>
      <PhoneBar username={username} />
      <div className="hero-glow" aria-hidden="true" />
      <div className="hero-scrim" aria-hidden="true" />
      <div className="page">
        <div className="hero-content">
          <Skeleton shape="line" width={140} className="skel-delay" />
          <HeroContentSkeleton headingId={headingId} />
        </div>
      </div>
    </section>
  );
}
