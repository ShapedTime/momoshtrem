import type { StorageSettings } from '@/lib/api/contract';
import { diskBar, floorNote } from '@/lib/settings/storage';
import { formatBytes } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';
import { SettingsCard } from '../SettingsCard';

/**
 * Where the disk goes (GET /settings/storage, as saved: edits don't move it):
 * the piece cache's kept, smart and streaming parts, other files and what's
 * free, with a marker at the limit, then the free-space floor.
 */
export function DiskCard({ settings }: { settings: StorageSettings }) {
  const bar = diskBar(settings);
  const floor = floorNote(settings);
  const limit = formatBytes(settings.limit_effective_bytes);
  return (
    <SettingsCard id="st-disk" title="Disk">
      <div className="set-disk">
        <div className="set-disk-top">
          <p>
            <span className="big tabular-nums">{formatBytes(bar.filled)}</span>{' '}
            <span className="tabular-nums text-fg-2">in the cache · limit {limit}</span>
          </p>
          <span className="t-small tabular-nums text-fg-3">
            {formatBytes(settings.disk.free_bytes)} free of {formatBytes(settings.disk.total_bytes)}
          </span>
        </div>
        <div className="set-disk-meter">
          <div className="set-disk-bar" role="img" aria-label={bar.label}>
            {bar.segments.map((segment) =>
              segment.part !== 'free' && segment.pct > 0 ? (
                <i key={segment.part} className={`set-disk-${segment.part}`} style={{ width: `${segment.pct}%`, minWidth: 2 }} />
              ) : null
            )}
          </div>
          <span className="set-disk-limit" style={{ left: `${bar.limitPct}%` }} aria-hidden="true" />
        </div>
        <ul className="set-disk-legend" aria-label="Disk use">
          {bar.segments.map((segment) => (
            <li key={segment.part}>
              <i className={`set-disk-${segment.part}`} aria-hidden="true" />
              {segment.label}
              <span className="tabular-nums">{formatBytes(segment.bytes)}</span>
            </li>
          ))}
          <li>
            <i className="set-disk-limit-key" aria-hidden="true" />
            Limit
            <span className="tabular-nums">{limit}</span>
          </li>
        </ul>
        <p className={cx('t-small', floor.warn ? 'text-warning' : 'text-fg-3')}>{floor.text}</p>
      </div>
    </SettingsCard>
  );
}
