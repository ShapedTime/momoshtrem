import { Play } from 'lucide-react';
import { Banner, Button, ButtonLink, Chip, SegmentedControl, Steps, Switch } from '@/components/ui';
import type { AiSettings, AiUsage, RunProgress } from '@/lib/api/contract';
import { RUN_STEPS, stepRows } from '@/lib/recommendations/steps';
import {
  type AiDraft,
  type AiField,
  MODEL_CARDS,
  MODEL_NAMES,
  picksHelp,
  REC_MODELS,
  runEstimate,
  type RunNowProblem,
  runProgressLine,
  SCHEDULE_LABELS,
  scheduleHelp,
  SCHEDULES,
} from '@/lib/settings/ai';
import { cx } from '@/lib/utils/cx';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';

interface PicksCardProps {
  draft: AiDraft;
  saved: AiSettings;
  /** Undefined while it loads or when it failed: the help leaves the run out, and the cards the estimates. */
  usage: AiUsage | undefined;
  /** ANTHROPIC_API_KEY is set; without it every control here is off. */
  configured: boolean;
  now: number;
  onChange: (next: Partial<AiDraft>) => void;
  errorFor: (field: AiField) => string | null;
  /** The run being written, while it's queued or running. */
  run: RunProgress | null;
  starting: boolean;
  problem: RunNowProblem | null;
  onRunNow: () => void;
  /** With two or more people (#239): whose runs these are, and what's shared. */
  memberNote: string | null;
}

/** The run's named steps while it works (HANDOFF §4.3), with its model and cost so far. */
function RunStatus({ run }: { run: RunProgress }) {
  const steps = stepRows(run.steps, RUN_STEPS, 'run').map(({ label, state, detail }) => ({ label, state, detail: detail ?? undefined }));
  return (
    <div className="set-run" role="status">
      <p className="flex flex-wrap items-baseline gap-x-2">
        <strong className="font-semibold text-fg-1">Writing your picks</strong>
        <span className="t-small text-fg-3">{runProgressLine(run)}</span>
      </p>
      <Steps inline steps={steps} />
    </div>
  );
}

/**
 * Personal picks (screens/settings/ai-*): the master switch, the schedule,
 * the model, and Run now. The design's "Write picks automatically" is the
 * master switch (`rec_enabled`), so it reads "Write personal picks": off
 * means no shelves and no runs at all, while the schedule says when. The
 * runs are the viewer's and the settings the household's (#239).
 */
export function PicksCard({ draft, saved, usage, configured, now, onChange, errorFor, run, starting, problem, onRunNow, memberNote }: PicksCardProps) {
  const off = !configured;
  return (
    <SettingsCard
      id="ai-picks"
      title="Personal picks"
      description={memberNote}
      aside={
        <Button variant="ghost" size="sm" loading={starting} disabled={off || run !== null} onClick={onRunNow}>
          <Play size={15} aria-hidden="true" /> Run now
        </Button>
      }
    >
      {problem && (
        <div className="set-card-note">
          <Banner
            tone="warning"
            role="alert"
            title={problem.title}
            action={
              problem.link && (
                <ButtonLink href={problem.link.href} variant="secondary" size="sm">
                  {problem.link.label}
                </ButtonLink>
              )
            }
          >
            {problem.body}
          </Banner>
        </div>
      )}
      {run && (
        <div className="set-card-note">
          <RunStatus run={run} />
        </div>
      )}

      <SettingRow id="ai-recs" label="Write personal picks" labelFor="ai-recs-input" help={picksHelp(draft.rec_enabled)} error={errorFor('rec_enabled')}>
        <Switch
          id="ai-recs-input"
          disabled={off}
          describedBy={rowDescribedBy('ai-recs', errorFor('rec_enabled'))}
          checked={draft.rec_enabled}
          onCheckedChange={(rec_enabled) => onChange({ rec_enabled })}
        />
      </SettingRow>

      <SettingRow
        id="ai-schedule"
        label="Schedule"
        help={scheduleHelp({ draft, saved, configured, recs: usage?.recs, now })}
        error={errorFor('rec_schedule')}
        stack
      >
        <SegmentedControl
          aria-label="Recommendation schedule"
          aria-describedby={rowDescribedBy('ai-schedule', errorFor('rec_schedule'))}
          className="max-sm:w-full"
          options={SCHEDULES.map((value) => ({ value, label: SCHEDULE_LABELS[value], disabled: off }))}
          value={draft.rec_schedule}
          onChange={(rec_schedule) => onChange({ rec_schedule })}
        />
      </SettingRow>

      <SettingRow id="ai-model" label="Model" help="Used for picks and Ask momoshtrem." error={errorFor('rec_model')} stack>
        <div className="set-models" role="radiogroup" aria-labelledby="ai-model-label" aria-describedby={rowDescribedBy('ai-model', errorFor('rec_model'))}>
          {REC_MODELS.map((model) => {
            const card = MODEL_CARDS[model];
            const id = `ai-model-${model}`;
            return (
              <label key={model} className={cx('set-model', off && 'is-disabled')}>
                <input
                  type="radio"
                  name="rec_model"
                  value={model}
                  checked={draft.rec_model === model}
                  disabled={off}
                  aria-labelledby={`${id}-name`}
                  aria-describedby={`${id}-desc`}
                  onChange={() => onChange({ rec_model: model })}
                />
                <span className="box">
                  <span className="flex items-center justify-between gap-2">
                    <strong id={`${id}-name`}>{MODEL_NAMES[model]}</strong>
                    <Chip tone={card.accent ? 'accent' : 'default'}>{card.tag}</Chip>
                  </span>
                  <span id={`${id}-desc`} className="flex flex-col gap-1">
                    <span className="t-small text-fg-2">{card.blurb}</span>{' '}
                    <span className="t-small tabular-nums text-fg-3">{runEstimate(usage?.estimates, model)}</span>
                  </span>
                </span>
              </label>
            );
          })}
        </div>
      </SettingRow>
    </SettingsCard>
  );
}
