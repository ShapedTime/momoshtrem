import { type Ref, useId } from 'react';
import { Captions, RefreshCw } from 'lucide-react';
import { Banner, Button, Chip, LoadingRegion, Skeleton } from '@/components/ui';
import type { HDRezkaQuality, HDRezkaStreams, HDRezkaTitle, HDRezkaTranslator } from '@/lib/api/contract';
import { hdrezkaProblem } from '@/lib/hdrezka/copy';
import { categoryLabel, qualityNote, qualityUsable, subtitlesLine, translatorBlocked, translatorChips, translatorKey } from '@/lib/hdrezka/pick';
import type { HDRezkaPick } from '@/lib/hdrezka/session';
import { cx } from '@/lib/utils/cx';
import { OpenOnHDRezka } from './MatchStep';

/** A failed HDRezka call inside a step: what went wrong, and Retry when it can help. */
function StepError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const problem = hdrezkaProblem(error);
  return (
    <Banner
      tone="error"
      role="alert"
      title={problem.title}
      action={
        problem.retry ? (
          <Button size="sm" onClick={onRetry}>
            <RefreshCw size={15} aria-hidden="true" /> Retry
          </Button>
        ) : undefined
      }
    >
      {problem.body}
    </Banner>
  );
}

interface PickedMatchProps {
  pick: HDRezkaPick;
  /** The title page, once it's loaded: its names win over the hit's. */
  title: HDRezkaTitle | undefined;
  onChange: () => void;
}

/** The match in use, collapsed: its names, year and kind, with Change to pick another. */
export function PickedMatch({ pick, title, onChange }: PickedMatchProps) {
  const name = title?.title ?? pick.title ?? null;
  const original = title?.original_title ?? pick.original_title ?? null;
  const year = title?.year ?? pick.year ?? null;
  const facts = [original !== name ? original : null, year !== null ? String(year) : null, pick.category ? categoryLabel(pick.category) : null].filter(Boolean);
  return (
    <section className="panel hd-picked" aria-label="Match">
      <div className="flex min-w-0 grow flex-col gap-0.5">
        <span className="t-micro text-fg-3">Match</span>
        {name ? (
          <p className="hd-name" lang="ru">
            {name}
          </p>
        ) : (
          <Skeleton shape="line" width="60%" />
        )}
        {facts.length > 0 && <p className="t-small text-fg-2">{facts.join(' · ')}</p>}
      </div>
      <div className="flex shrink-0 items-center">
        <OpenOnHDRezka url={pick.url} name={name ?? 'this title'} />
        <Button variant="ghost" size="sm" onClick={onChange} aria-label="Change the match">
          Change
        </Button>
      </div>
    </section>
  );
}

interface VoiceOverStepProps {
  status: 'pending' | 'error' | 'success';
  title: HDRezkaTitle | undefined;
  error: unknown;
  onRetry: () => void;
  selected: HDRezkaTranslator | null;
  onSelect: (translator: HDRezkaTranslator) => void;
  onTryTorrents: () => void;
  /** Focused when a match is picked, so keyboards land on the next step. */
  headingRef?: Ref<HTMLHeadingElement>;
}

/**
 * Step 2: the voice-over, as radio rows. Premium ones stay visible but
 * can't be picked; Ads, CAMRip and Director's cut are chips.
 */
export function VoiceOverStep({ status, title, error, onRetry, selected, onSelect, onTryTorrents, headingRef }: VoiceOverStepProps) {
  const headingId = useId();
  const name = useId();
  const allPremium = title !== undefined && title.translators.length > 0 && title.translators.every((t) => t.premium);
  return (
    <section className="flex flex-col gap-3" aria-labelledby={headingId}>
      <h3 className="t-title-3 outline-none" id={headingId} tabIndex={-1} ref={headingRef}>
        Voice-over
      </h3>
      {status === 'pending' ? (
        <LoadingRegion label="Loading voice-overs" className="hd-options">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} height={50} />
          ))}
        </LoadingRegion>
      ) : status === 'error' || !title ? (
        <StepError error={error} onRetry={onRetry} />
      ) : (
        <>
          {allPremium && (
            <Banner
              tone="warning"
              title="Every voice-over here needs HDRezka Premium"
              action={
                <Button size="sm" onClick={onTryTorrents}>
                  Try torrents
                </Button>
              }
            >
              momoshtrem streams without an account. Pick another match, or find a torrent.
            </Banner>
          )}
          <div role="radiogroup" aria-labelledby={headingId} className="hd-options">
            {title.translators.map((t) => {
              const key = translatorKey(t);
              const blocked = translatorBlocked(t);
              const chips = translatorChips(t);
              const reasonId = `${name}-${key}`;
              return (
                <label key={key} className={cx('radio-card hd-option', blocked && 'is-blocked')}>
                  <input
                    type="radio"
                    name={name}
                    value={key}
                    // "Дубляж, Premium": the chips are part of the choice
                    aria-label={[t.name, ...chips.map((chip) => chip.label)].join(', ')}
                    aria-describedby={blocked ? reasonId : undefined}
                    checked={selected !== null && translatorKey(selected) === key}
                    disabled={blocked !== null}
                    onChange={() => onSelect(t)}
                  />
                  <span className="flex min-w-0 flex-col gap-1">
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      <span className="font-medium" lang="ru">
                        {t.name}
                      </span>
                      {chips.map((chip) => (
                        <Chip key={chip.label} tone={chip.tone}>
                          {chip.label}
                        </Chip>
                      ))}
                    </span>
                    {blocked && (
                      <span className="t-small text-fg-3" id={reasonId}>
                        {blocked}
                      </span>
                    )}
                  </span>
                </label>
              );
            })}
          </div>
        </>
      )}
    </section>
  );
}

interface QualityStepProps {
  status: 'pending' | 'error' | 'success';
  streams: HDRezkaStreams | undefined;
  error: unknown;
  onRetry: () => void;
  selected: HDRezkaQuality | null;
  onSelect: (quality: HDRezkaQuality) => void;
  /** A show's: sizes are an episode's. */
  perEpisode?: boolean;
}

/** Step 3: the quality, with sizes; premium and HLS-only ones can't be picked. Then the stream's subtitles. */
export function QualityStep({ status, streams, error, onRetry, selected, onSelect, perEpisode = false }: QualityStepProps) {
  const headingId = useId();
  const name = useId();
  return (
    <section className="flex flex-col gap-3" aria-labelledby={headingId}>
      <h3 className="t-title-3" id={headingId}>
        Quality
      </h3>
      {status === 'pending' ? (
        <LoadingRegion label="Loading qualities" className="hd-qualities">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} height={62} />
          ))}
        </LoadingRegion>
      ) : status === 'error' || !streams ? (
        <StepError error={error} onRetry={onRetry} />
      ) : (
        <>
          <div role="radiogroup" aria-labelledby={headingId} className="hd-qualities">
            {streams.qualities.map((q) => {
              const usable = qualityUsable(q);
              return (
                <label key={q.label} className={cx('radio-card hd-quality', !usable && 'is-blocked')}>
                  <input
                    type="radio"
                    name={name}
                    value={q.label}
                    aria-label={`${q.label}, ${qualityNote(q, perEpisode)}`}
                    checked={selected?.label === q.label}
                    disabled={!usable}
                    onChange={() => onSelect(q)}
                  />
                  <span className="flex min-w-0 flex-col">
                    <span className="font-semibold">{q.label}</span>
                    {q.premium ? (
                      <Chip tone="accent" className="self-start">
                        Premium
                      </Chip>
                    ) : (
                      <span className="t-small text-fg-2 tabular-nums">{qualityNote(q, perEpisode)}</span>
                    )}
                  </span>
                </label>
              );
            })}
          </div>
          <p className="t-small flex items-center gap-2 text-fg-2">
            <Captions size={16} className="shrink-0 text-fg-3" aria-hidden="true" />
            {subtitlesLine(streams.subtitles)}
          </p>
        </>
      )}
    </section>
  );
}
