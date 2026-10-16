import type { ReactNode } from 'react';
import { CircleAlert } from 'lucide-react';
import { Panel } from '@/components/ui';
import { cx } from '@/lib/utils/cx';

interface SettingsCardProps {
  id: string;
  title: string;
  description?: ReactNode;
  /** A quiet action beside the title, e.g. AI & picks' "Run now". */
  aside?: ReactNode;
  children: ReactNode;
}

/** A titled settings card (screens/settings): rows or free content below the title. */
export function SettingsCard({ id, title, description, aside, children }: SettingsCardProps) {
  return (
    <Panel as="section" aria-labelledby={`${id}-h`} className="set-card">
      <header className="set-card-head">
        <h3 id={`${id}-h`} className="t-title-3">
          {title}
        </h3>
        {aside}
      </header>
      {description && <p className="t-small set-card-sub text-fg-3">{description}</p>}
      {children}
    </Panel>
  );
}

/** The ids a row's control points at with `aria-describedby`: its error first, then its help. */
export function rowDescribedBy(id: string, error?: string | null): string {
  return error ? `${id}-error ${id}-help` : `${id}-help`;
}

interface SettingRowProps {
  /** The row's id; its help and error are `${id}-help` and `${id}-error`. */
  id: string;
  label: string;
  /** The control's id, when it can take a <label>. Otherwise the label is `${id}-label`. */
  labelFor?: string;
  help: ReactNode;
  error?: string | null;
  /** Label above the control: segmented controls (side by side from 1024 px) and ranges. */
  stack?: boolean;
  children: ReactNode;
}

/** One setting: its label and help, then the control; a server error shows under the help. */
export function SettingRow({ id, label, labelFor, help, error, stack, children }: SettingRowProps) {
  return (
    <div id={id} className={cx('set-row', stack && 'set-row--stack')}>
      <div className="set-row-text">
        {labelFor ? (
          <label htmlFor={labelFor} className="set-row-label">
            {label}
          </label>
        ) : (
          <span id={`${id}-label`} className="set-row-label">
            {label}
          </span>
        )}
        <span id={`${id}-help`} className="help">
          {help}
        </span>
        {error && (
          <span id={`${id}-error`} className="error-text mt-1">
            <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0" />
            <span>{error}</span>
          </span>
        )}
      </div>
      <div className="set-row-ctl">{children}</div>
    </div>
  );
}
