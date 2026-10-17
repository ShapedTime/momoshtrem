import { Progress, StatusPill } from '@/components/ui';
import type { Keep } from '@/lib/api/contract';
import { keepExpiryLabel, keepStatePill, keepWantsBytes } from '@/lib/title/keep';
import { cx } from '@/lib/utils/cx';

interface KeepStatusProps {
  keep: Keep;
  /** Names the progress bar: "Interstellar kept download". */
  name: string;
  /**
   * The bar of what's on disk. Off where the page already shows the download's progress (a
   * movie's downloading stage), which also drops the "Downloading N%" pill.
   */
  progress?: boolean;
  className?: string;
}

/**
 * What holds a title, season or episode on disk (#206): "Kept until watched ·
 * by Nov 16" (or "Smart pick · …") beside its state ("On disk", "Waits for
 * the stream to end", "Downloading 42%"), and what's on disk while it still
 * wants bytes.
 */
export function KeepStatus({ keep, name, progress = true, className }: KeepStatusProps) {
  const state = keepStatePill(keep.state, keep.pct, null);
  const showState = progress || keep.state !== 'downloading';
  return (
    <div className={cx('flex min-w-0 flex-col gap-2', className)}>
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill status={keep.reason} label={keepExpiryLabel(keep)} />
        {showState && <StatusPill status={state.status} label={state.label} />}
      </div>
      {progress && keepWantsBytes(keep.state) && <Progress value={keep.pct} tone={keep.state === 'downloading' ? 'downloading' : 'watched'} label={`${name} kept download`} />}
    </div>
  );
}
