import { Fragment, type ReactNode } from 'react';
import { cx } from '@/lib/utils/cx';

/**
 * Split `text` after each separator so the browser can break there (a <wbr>
 * after it); `dim` separators get the quiet colour (HANDOFF §5.1).
 */
function breakAfter(text: string, separators: RegExp, dim: (separator: string) => boolean): ReactNode[] {
  return text.split(separators).map((part, i) => {
    if (i % 2 === 0) return part;
    return (
      <Fragment key={i}>
        {dim(part) ? <span className="sep">{part}</span> : part}
        <wbr />
      </Fragment>
    );
  });
}

interface ReleaseNameProps {
  name: string;
  as?: 'p' | 'span';
  className?: string;
}

/** A release name in Geist Mono, breaking only after `. - _ ] ) /` with dimmed dots. The full name is its title. */
export function ReleaseName({ name, as: Tag = 'p', className }: ReleaseNameProps) {
  return (
    <Tag className={cx('release-name', className)} title={name}>
      {breakAfter(name, /([._\-\])/])/, (separator) => separator === '.' || separator === '_')}
    </Tag>
  );
}

/** A file path: the folders dimmed, the file name in full, breaking after separators and spaces. */
export function FilePath({ path, className }: { path: string; className?: string }) {
  const slash = path.lastIndexOf('/');
  const folder = path.slice(0, slash + 1);
  const file = path.slice(slash + 1);
  const breaks = /([._\-\])/ ])/;
  return (
    <span className={cx('path', className)}>
      {folder && <span className="dir">{breakAfter(folder, breaks, () => false)}</span>}
      {breakAfter(file, breaks, () => false)}
    </span>
  );
}
