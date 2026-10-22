'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { RefreshCw, Sparkles } from 'lucide-react';
import { useMembers } from '@/components/members/MembersGate';
import { Banner, Button, LoadingRegion, Skeleton, useToast } from '@/components/ui';
import { useNow } from '@/hooks/useNow';
import { isApiError } from '@/lib/api/client';
import type { AiSettingsPatch } from '@/lib/api/contract';
import { isActiveRun, recApi, recKeys, recRunQuery } from '@/lib/api/endpoints/recommendations';
import { aiSettingsQuery, aiUsageQuery, settingsApi, settingsKeys } from '@/lib/api/endpoints/settings';
import {
  aiChanges,
  type AiDraft,
  type AiField,
  budgetMonth,
  FIELD_LABELS,
  fieldErrorCopy,
  hasChanges,
  isAiField,
  memberSpend,
  picksMemberNote,
  runNowProblem,
  runToastBody,
  withEdits,
} from '@/lib/settings/ai';
import { BudgetCard } from './ai/BudgetCard';
import { MappingCard } from './ai/MappingCard';
import { PicksCard } from './ai/PicksCard';
import { SaveBar } from './SaveBar';

/** Where a problem with each field shows: its row (scrolled to) and its control (focused). */
const FIELD_TARGET: Record<AiField, { row: string; control: string }> = {
  rec_enabled: { row: 'ai-recs', control: '#ai-recs-input' },
  rec_model: { row: 'ai-model', control: '#ai-model input:checked' },
  rec_schedule: { row: 'ai-schedule', control: '#ai-schedule button[aria-pressed="true"]' },
  monthly_budget_usd: { row: 'ai-budget', control: '#ai-budget-input' },
  classifier_enabled: { row: 'ai-classifier', control: '#ai-classifier-input' },
  classifier_model: { row: 'ai-classifier', control: '#ai-classifier-input' },
  auto_apply_high_confidence: { row: 'ai-autoapply', control: '#ai-autoapply-input' },
};

/** Bring a field with a problem into view and move focus to it (from Save). */
function showField(field: AiField) {
  const { row, control } = FIELD_TARGET[field];
  document.getElementById(row)?.scrollIntoView?.({ block: 'center' });
  document.querySelector<HTMLElement>(control)?.focus({ preventScroll: true });
}

function AiSkeleton() {
  return (
    <LoadingRegion label="Loading AI & picks" className="flex flex-col gap-4 lg:gap-5">
      <Skeleton height={420} className="rounded-lg" />
      <Skeleton height={310} className="rounded-lg" />
      <Skeleton height={200} className="rounded-lg" />
    </LoadingRegion>
  );
}

/**
 * AI & picks (screens/settings/ai-*): personal picks (the master switch,
 * schedule, model, Run now), the monthly budget with this month's spend, and
 * episode mapping's classifier. Edits stay local until Save sends only what
 * changed (PUT /settings/ai is partial); a 422 shows next to the field it
 * names. Without an Anthropic key the picks controls are off, and the rest
 * still saves. With household members (#239) the runs are the viewer's, and
 * the budget splits picks and Ask by person.
 */
export function AiSection() {
  const queryClient = useQueryClient();
  const router = useRouter();
  const { toast } = useToast();
  const settings = useQuery(aiSettingsQuery());
  const usage = useQuery(aiUsageQuery());
  const household = useMembers();
  const now = useNow(60_000);
  // Only the fields touched here: the rest follows the server, so a stale page never sends them back
  const [edits, setEdits] = useState<Partial<AiDraft>>({});
  // A budget that doesn't parse shows once Save was pressed, not while it's typed
  const [triedSave, setTriedSave] = useState(false);
  const [startedRunId, setStartedRunId] = useState<number | null>(null);

  // The run being written: the usage's active one (a scheduled run counts), else the one Run now started
  const watchedRunId = usage.data?.recs.active_run?.id ?? startedRunId;
  const run = useQuery({ ...recRunQuery(watchedRunId ?? 0), enabled: watchedRunId !== null });
  const runEnded = run.data !== undefined && !isActiveRun(run.data);
  useEffect(() => {
    if (!runEnded) return;
    // Its cost, the last run and the estimates moved; For You has new picks
    void queryClient.invalidateQueries({ queryKey: settingsKeys.aiUsage });
    void queryClient.invalidateQueries({ queryKey: recKeys.home });
  }, [runEnded, run.data?.id, queryClient]);

  const refresh = useMutation({
    mutationFn: () => recApi.refresh(),
    onSuccess: ({ run_id }) => {
      setStartedRunId(run_id);
      void queryClient.invalidateQueries({ queryKey: settingsKeys.aiUsage });
      void queryClient.invalidateQueries({ queryKey: recKeys.home });
      toast({
        tone: 'info',
        icon: Sparkles,
        title: 'Writing your picks',
        // The run uses the saved model
        body: runToastBody(settings.data?.rec_model ?? 'claude-opus-5-5', usage.data?.estimates),
        action: { label: 'Watch', onClick: () => router.push('/') },
      });
    },
  });

  const save = useMutation({
    mutationFn: (patch: AiSettingsPatch) => settingsApi.putAi(patch),
    onSuccess: (saved) => {
      queryClient.setQueryData(settingsKeys.ai, saved);
      setEdits({});
      setTriedSave(false);
      // Why Run now couldn't start (picks off, the budget) may no longer hold
      refresh.reset();
      toast({ tone: 'ready', title: 'Settings saved' });
      // The next run, the budget and whether For You has picks follow the settings
      void queryClient.invalidateQueries({ queryKey: settingsKeys.aiUsage });
      void queryClient.invalidateQueries({ queryKey: recKeys.home });
    },
    onError: (error) => {
      if (isApiError(error) && isAiField(error.field)) showField(error.field);
    },
  });

  if (settings.isPending || usage.isPending) return <AiSkeleton />;

  if (settings.isError) {
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load the AI settings"
        action={
          <Button size="sm" onClick={() => void settings.refetch()}>
            <RefreshCw size={15} aria-hidden="true" /> Try again
          </Button>
        }
      >
        {settings.error.message}
      </Banner>
    );
  }

  const saved = settings.data;
  const current = withEdits(saved, edits);
  const changes = aiChanges(edits, saved);
  const dirty = hasChanges(changes);
  // Unknown when the usage didn't load: keep the controls on, the server still says no
  const configured = usage.data?.configured ?? true;

  const apiError = isApiError(save.error) ? save.error : null;
  const invalid = apiError?.status === 422;
  const serverField = apiError && invalid && isAiField(apiError.field) ? apiError.field : null;
  const budgetError = triedSave ? changes.budgetError : null;
  const errorFor = (field: AiField): string | null => {
    if (field === 'monthly_budget_usd' && budgetError) return budgetError;
    return serverField === field && apiError ? fieldErrorCopy(field, apiError.message) : null;
  };
  // A field's problem shows there; the bar points at it. Anything else shows in the bar.
  const problemField = budgetError ? 'monthly_budget_usd' : serverField;
  const barError = problemField
    ? `Check “${FIELD_LABELS[problemField]}” above.`
    : save.error
      ? invalid
        ? save.error.message
        : `Couldn’t save: ${save.error.message} Try again in a moment.`
      : null;

  const change = (next: Partial<AiDraft>) => {
    setEdits((previous) => ({ ...previous, ...next }));
    if (save.error) save.reset();
  };

  const activeRun = run.data ? (isActiveRun(run.data) ? run.data : null) : (usage.data?.recs.active_run ?? null);
  const problem = refresh.error
    ? runNowProblem(isApiError(refresh.error) ? refresh.error : { message: refresh.error.message }, usage.data ? budgetMonth(usage.data.month).resets : null)
    : null;

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {!configured && (
        <Banner title="No Anthropic key on this server">
          Personal picks, Ask momoshtrem and the mapping classifier need <code className="t-mono text-fg-1">ANTHROPIC_API_KEY</code> in the
          server’s .env. Add it and restart momoshtrem. These settings still save, and apply once the key is there.
        </Banner>
      )}

      <PicksCard
        draft={current}
        saved={saved}
        usage={usage.data}
        configured={configured}
        now={now}
        onChange={change}
        errorFor={errorFor}
        run={activeRun}
        starting={refresh.isPending}
        problem={problem}
        onRunNow={() => refresh.mutate()}
        memberNote={picksMemberNote(household.current, household.members.length)}
      />

      <BudgetCard
        usage={usage.data}
        people={usage.data ? memberSpend(usage.data, household.members) : null}
        onRetryUsage={() => void usage.refetch()}
        now={now}
        budget={current.budget}
        onBudget={(budget) => change({ budget })}
        error={errorFor('monthly_budget_usd')}
      />

      <MappingCard draft={current} classifierModel={saved.classifier_model} onChange={change} errorFor={errorFor} />

      {dirty && (
        <SaveBar
          saving={save.isPending}
          note="Saved settings apply from the next run or mapping."
          error={barError}
          onDiscard={() => {
            setEdits({});
            setTriedSave(false);
            save.reset();
          }}
          onSave={() => {
            if (changes.budgetError) {
              setTriedSave(true);
              showField('monthly_budget_usd');
              return;
            }
            save.mutate(changes.patch);
          }}
        />
      )}
    </div>
  );
}
