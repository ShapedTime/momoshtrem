import Link from 'next/link';
import type { ReactNode } from 'react';
import { cx } from '@/lib/utils/cx';
import { Media } from './Media';
import { Progress } from './Status';

interface EpisodeCardProps {
  /** The show's page. */
  href: string;
  show: string;
  /** The episode's name; one line. */
  name: string;
  /** "S01E29" or "S22E39 · #1124", over the still. */
  code: string;
  stillSrc?: string | null;
  /** On glass at the still's corner: "Today", "Fri" (new episodes). */
  when?: string | null;
  /** 0–100: a bar along the still's bottom edge while it downloads. */
  downloadPct?: number | null;
  /** The status pill (`<StatusPill>`). */
  status: ReactNode;
  /** The one action that fits the availability (Play in Infuse, Details, Find, Add); none until it airs. */
  action?: ReactNode;
  className?: string;
}

/**
 * Landscape episode card (HANDOFF §3 EpisodeCard, `.lcard`): the still with
 * its code, the show, the episode, then status and action. Everything is
 * visible without hover.
 */
export function EpisodeCard({ href, show, name, code, stillSrc, when, downloadPct, status, action, className }: EpisodeCardProps) {
  return (
    <article className={cx('lcard', className)}>
      <Link href={href} className="rounded-md" aria-label={`${show} ${code}: ${name}`}>
        <Media shape="wide" src={stillSrc} alt="" sizes="(min-width: 1440px) 320px, (min-width: 1024px) 30vw, (min-width: 640px) 45vw, 85vw">
          <div className="over">
            <span className="ep">{code}</span>
            {when && <span className="pill pill--glass">{when}</span>}
          </div>
          {downloadPct !== null && downloadPct !== undefined && (
            <Progress value={downloadPct} label={`${show} ${code} download`} className="absolute inset-x-0 bottom-0 z-3 rounded-none" />
          )}
        </Media>
      </Link>
      <div className="body">
        <span className="show">{show}</span>
        <Link href={href} className="name" tabIndex={-1}>
          {name}
        </Link>
        <div className="foot">
          {status}
          {action}
        </div>
      </div>
    </article>
  );
}
