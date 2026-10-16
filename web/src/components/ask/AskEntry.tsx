import Link from 'next/link';
import type { ReactNode } from 'react';
import { ArrowRight } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { cx } from '@/lib/utils/cx';

interface AskEntryProps {
  /** Where it goes: `askHref(q)`. */
  href: string;
  /** The bold first line: "Not quite it?", "Ask momoshtrem…". */
  title: ReactNode;
  /** The line under it: "Ask momoshtrem about “q”". */
  sub: ReactNode;
  /** Noir's avatar: 36 on Search, 40 in For You's Ask band. */
  avatarSize?: number;
  className?: string;
}

/**
 * A wide link into Ask momoshtrem (the prototype's `.ask-entry`): Noir, two
 * lines and an arrow. Search's "Not quite it?" card and For You's Ask band.
 * Render it only while `useAskAvailable()` is true.
 */
export function AskEntry({ href, title, sub, avatarSize = 36, className }: AskEntryProps) {
  return (
    <Link href={href} className={cx('ask-entry', className)}>
      <CatAvatar size={avatarSize} />
      <span className="grow">
        <strong>{title}</strong>
        <span className="t-small">{sub}</span>
      </span>
      <span className="btn btn-secondary btn-icon btn-sm" aria-hidden="true">
        <ArrowRight size={18} />
      </span>
    </Link>
  );
}
