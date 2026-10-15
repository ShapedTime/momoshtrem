import { useId } from 'react';
import { Banner, Button, LoadingRegion, Skeleton, StatusPill, Tabs } from '@/components/ui';
import type { HDRezkaSeason, StreamEpisodeFailure } from '@/lib/api/contract';
import { allKeys, episodeKey, episodeRowState, episodeSourceLabel, libraryCode, missingKeys } from '@/lib/hdrezka/episodes';
import { hdrezkaProblem } from '@/lib/hdrezka/copy';
import { plural } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

const pad = (n: number) => String(n).padStart(2, '0');

interface EpisodesStepProps {
  status: 'pending' | 'error' | 'success';
  /** HDRezka's seasons for the chosen voice-over, with the library episode each one fills. */
  seasons: HDRezkaSeason[];
  error: unknown;
  onRetry: () => void;
  /** The open season tab. */
  seasonNumber: number | null;
  onSeasonChange: (season: number) => void;
  /** `episodeKey`s, across seasons. */
  selected: string[];
  onSelectedChange: (keys: string[]) => void;
}

/**
 * Step 4 for a show: which episodes get the stream. One season at a time;
 * each row reads "E05 · name → S01E05" with its state and current source.
 * Episodes that map to nothing in the library can't be picked.
 */
export function EpisodesStep({ status, seasons, error, onRetry, seasonNumber, onSeasonChange, selected, onSelectedChange }: EpisodesStepProps) {
  const headingId = useId();
  const listId = useId();
  const chosen = new Set(selected);
  const season = seasons.find((s) => s.number === seasonNumber) ?? seasons[0];

  if (status === 'pending') {
    return (
      <section className="flex flex-col gap-3" aria-labelledby={headingId}>
        <h3 className="t-title-3" id={headingId}>
          Episodes
        </h3>
        <LoadingRegion label="Loading episodes" className="flex flex-col gap-1">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} height={56} />
          ))}
        </LoadingRegion>
      </section>
    );
  }

  if (status === 'error' || !season) {
    const problem = status === 'error' ? hdrezkaProblem(error) : null;
    return (
      <section className="flex flex-col gap-3" aria-labelledby={headingId}>
        <h3 className="t-title-3" id={headingId}>
          Episodes
        </h3>
        {problem ? (
          <Banner
            tone="error"
            role="alert"
            title={problem.title}
            action={
              problem.retry ? (
                <Button size="sm" onClick={onRetry}>
                  Retry
                </Button>
              ) : undefined
            }
          >
            {problem.body}
          </Banner>
        ) : (
          <p className="t-small text-fg-2">HDRezka lists no episodes with this voice-over.</p>
        )}
      </section>
    );
  }

  const inSeason = (number: number) => selected.filter((key) => key.startsWith(`${number}:`)).length;
  /** Swap this season's selection for `keys`, keeping the other seasons'. */
  const selectInSeason = (keys: string[]) => onSelectedChange([...selected.filter((key) => !key.startsWith(`${season.number}:`)), ...keys]);
  const toggle = (key: string) => onSelectedChange(chosen.has(key) ? selected.filter((k) => k !== key) : [...selected, key]);
  const missing = missingKeys(season);
  const all = allKeys(season);

  return (
    <section className="flex flex-col gap-3" aria-labelledby={headingId}>
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="t-title-3" id={headingId}>
          Episodes
        </h3>
        <span className="t-small tabular-nums text-fg-2" aria-live="polite">
          {selected.length === 0 ? 'None selected' : `${plural(selected.length, 'episode')} selected`}
        </span>
      </div>
      {seasons.length > 1 && (
        <Tabs
          aria-label="Seasons on HDRezka"
          value={String(season.number)}
          onChange={(value) => onSeasonChange(Number(value))}
          tabs={seasons.map((s) => ({
            value: String(s.number),
            label: `Season ${s.number}`,
            count: inSeason(s.number) || undefined,
            controls: s.number === season.number ? listId : undefined,
          }))}
        />
      )}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={missing.length === 0} onClick={() => selectInSeason(missing)}>
          Select missing <span className="tabular-nums text-fg-3">{missing.length}</span>
        </Button>
        <Button size="sm" disabled={all.length === 0} onClick={() => selectInSeason(all)}>
          Select all <span className="tabular-nums text-fg-3">{all.length}</span>
        </Button>
        {inSeason(season.number) > 0 && (
          <Button size="sm" variant="ghost" onClick={() => selectInSeason([])}>
            Clear
          </Button>
        )}
      </div>
      <div id={listId} {...(seasons.length > 1 ? { role: 'tabpanel', 'aria-label': `Season ${season.number}` } : {})}>
      <ul className="hd-episodes" aria-label={`Season ${season.number} episodes on HDRezka`}>
        {season.episodes.map((episode) => {
          const key = episodeKey(season.number, episode.number);
          const { library } = episode;
          const code = `E${pad(episode.number)}`;
          const name = library ? `${code} · ${library.name} → ${libraryCode(library)}` : code;
          const source = library ? episodeSourceLabel(library) : null;
          return (
            <li key={key}>
              <label className={cx('hd-ep', !library && 'is-blocked')}>
                <input type="checkbox" aria-label={name} checked={chosen.has(key)} disabled={!library} onChange={() => toggle(key)} />
                <span className="flex min-w-0 grow flex-col">
                  <span className="hd-ep-name">
                    <b>{code}</b>
                    <span className={cx('truncate', !library && 'text-fg-3')}>· {library ? library.name : 'Not in your library'}</span>
                  </span>
                  {library && (
                    <span className="t-small truncate text-fg-2">
                      → {libraryCode(library)}
                      {source && (
                        <>
                          {' · '}
                          <span lang={library.source === 'hdrezka' ? 'ru' : undefined}>{source}</span>
                        </>
                      )}
                    </span>
                  )}
                </span>
                {library && <StatusPill status={episodeRowState(library)} />}
              </label>
            </li>
          );
        })}
      </ul>
      </div>
    </section>
  );
}

interface FailedEpisodesProps {
  failed: StreamEpisodeFailure[];
  /** Episode id → "S02E07", for the ones the seasons show. */
  codes: Map<number, string>;
  /** Some episodes of the run did get their stream. */
  partial: boolean;
  retrying: boolean;
  onRetry: () => void;
}

/** After a run: which episodes didn't get their stream and why, with Retry for only them. */
export function FailedEpisodes({ failed, codes, partial, retrying, onRetry }: FailedEpisodesProps) {
  const named = failed.map((f) => codes.get(f.episode_id)).filter((code): code is string => Boolean(code));
  const reasons = [...new Set(failed.map((f) => f.message))];
  return (
    <Banner
      tone="error"
      role="alert"
      title={`${plural(failed.length, 'episode')} didn’t stream`}
      action={
        <Button size="sm" loading={retrying} onClick={onRetry}>
          Retry {failed.length === 1 ? 'it' : 'them'}
        </Button>
      }
    >
      {named.length > 0 && `${named.join(', ')}. `}
      {reasons.join(' ')}
      {partial && ' The rest are ready in Infuse.'}
    </Banner>
  );
}
