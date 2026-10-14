import { CheckCheck, Star } from 'lucide-react';
import type { WatchState } from '@/lib/api/contract';
import { formatDate } from '@/lib/title/format';
import { watchSummary } from '@/lib/title/facts';

/** The hero's amber Trakt line ("Watched Mar 14, 2024 · You rated it 9/10"); nothing without watch state. */
export function WatchStateLine({ state }: { state: WatchState | null }) {
  const parts = state ? watchSummary(state) : [];
  if (!parts.length) return null;
  return (
    <p className="trakt-line">
      <CheckCheck size={16} aria-hidden="true" />
      <span>{parts.join(' · ')}</span>
    </p>
  );
}

/** "On Trakt" in the aside: what Trakt knows, read-only (Infuse scrobbles; momoshtrem only reads). */
export function OnTraktCard({ state }: { state: WatchState | null }) {
  if (!state) return null;
  const progress = state.progress && state.progress.aired > 0 ? state.progress : null;
  return (
    <section className="panel panel-pad flex flex-col gap-3" aria-labelledby="on-trakt-h">
      <div className="flex items-center justify-between gap-3">
        <h2 className="t-title-3" id="on-trakt-h">
          On Trakt
        </h2>
        <span className="t-small text-fg-3">read-only</span>
      </div>
      {progress ? (
        <p className="flex items-center gap-2">
          <CheckCheck size={18} className="text-accent-text" aria-hidden="true" />
          <span>
            {progress.completed} of {progress.aired} episodes watched
          </span>
        </p>
      ) : state.watched_at ? (
        <p className="flex items-center gap-2">
          <CheckCheck size={18} className="text-accent-text" aria-hidden="true" />
          <span>
            Watched {formatDate(state.watched_at)}
            {state.plays > 1 && <span className="text-fg-3"> · {state.plays} plays</span>}
          </span>
        </p>
      ) : null}
      {state.rating !== null && (
        <p className="flex items-center gap-2">
          <Star size={18} className="text-accent-text" aria-hidden="true" />
          <span>
            Your rating: <strong>{state.rating}</strong>/10
          </span>
        </p>
      )}
      <p className="t-small text-fg-3">Infuse scrobbles plays to Trakt; momoshtrem only reads.</p>
    </section>
  );
}
