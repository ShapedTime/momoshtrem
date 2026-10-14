import Link from 'next/link';
import type { ReactNode } from 'react';
import { CircleAlert, CircleCheck, Info, TriangleAlert, type LucideIcon } from 'lucide-react';
import { cx } from '@/lib/utils/cx';

/* ---- Panel -------------------------------------------------------------- */

interface PanelProps {
  /** 16 px padding (20 from 640). */
  padded?: boolean;
  as?: 'div' | 'section' | 'ul' | 'ol';
  className?: string;
  children: ReactNode;
  'aria-label'?: string;
  'aria-labelledby'?: string;
}

export function Panel({ padded, as: Tag = 'div', className, children, ...rest }: PanelProps) {
  return (
    <Tag className={cx('panel', padded && 'panel-pad', className)} {...rest}>
      {children}
    </Tag>
  );
}

/* ---- ListRow ------------------------------------------------------------ */

interface ListRowProps {
  title: ReactNode;
  description?: ReactNode;
  /** Leading icon or thumbnail. */
  leading?: ReactNode;
  /** Trailing control, pill or chevron. */
  trailing?: ReactNode;
  href?: string;
  as?: 'div' | 'li';
  className?: string;
}

/** A 56 px row; consecutive rows get a hairline divider. */
export function ListRow({ title, description, leading, trailing, href, as: Tag = 'div', className }: ListRowProps) {
  const body = (
    <>
      {leading}
      <div className="grow">
        <span className="font-medium text-fg-1">{title}</span>
        {description && <span className="t-small text-fg-3">{description}</span>}
      </div>
      {trailing}
    </>
  );
  if (href) {
    return (
      <Tag className="contents">
        <Link href={href} className={cx('list-row', className)}>
          {body}
        </Link>
      </Tag>
    );
  }
  return <Tag className={cx('list-row', className)}>{body}</Tag>;
}

/* ---- Banner ------------------------------------------------------------- */

export type BannerTone = 'neutral' | 'review' | 'warning' | 'error' | 'ready' | 'accent';

const BANNER_ICON: Record<BannerTone, LucideIcon> = {
  neutral: Info,
  review: CircleAlert,
  warning: TriangleAlert,
  error: CircleAlert,
  ready: CircleCheck,
  accent: Info,
};

interface BannerProps {
  tone?: BannerTone;
  title: ReactNode;
  children?: ReactNode;
  icon?: LucideIcon;
  /** At most one action. */
  action?: ReactNode;
  /** `alert` for errors the user must notice now; `status` for the rest. */
  role?: 'alert' | 'status';
  id?: string;
  className?: string;
}

export function Banner({ tone = 'neutral', title, children, icon, action, role, id, className }: BannerProps) {
  const Icon = icon ?? BANNER_ICON[tone];
  return (
    <div id={id} role={role} className={cx('banner', tone !== 'neutral' && `banner--${tone}`, className)}>
      <Icon size={20} aria-hidden="true" />
      <div className="grow">
        <strong>{title}</strong>
        {children && <span>{children}</span>}
      </div>
      {action && <div className="actions">{action}</div>}
    </div>
  );
}
