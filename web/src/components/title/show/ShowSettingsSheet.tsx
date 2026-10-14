'use client';

import { type FormEvent, useId, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Banner, Button, LoadingRegion, Select, Sheet, Skeleton, SkeletonText, Switch, useToast } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import type { ClassifierMode, EpisodeOrdering, ShowSettingsPatch, ShowTitle } from '@/lib/api/contract';
import { episodeGroupsQuery, libraryKeys, titleKeys, titleQuery, titlesApi } from '@/lib/api/endpoints';
import type { SheetComponentProps } from '@/lib/sheets/registry';

const ORDERINGS: Array<{ value: EpisodeOrdering; label: string; description: string }> = [
  { value: 'aired', label: 'Aired', description: 'Seasons and episodes as TMDB lists them. Right for most shows.' },
  { value: 'absolute', label: 'Absolute (anime)', description: 'One running number: #29 instead of S01E29. Matches fansub releases like SubsPlease.' },
  { value: 'episode_group', label: 'TMDB episode group', description: 'Use an alternative order, e.g. “Seasons (production)”.' },
];

const CLASSIFIER_MODES: Array<{ value: ClassifierMode; label: string }> = [
  { value: 'auto', label: 'Rules, then AI for tricky files (recommended)' },
  { value: 'rules_only', label: 'Rules only' },
  { value: 'ai_always', label: 'Always ask AI' },
];

/** The PATCH for the form's values: a group id only goes with group ordering. */
export function settingsPatch(values: { ordering: EpisodeOrdering; groupId: string; includeSpecials: boolean; classifierMode: ClassifierMode }): ShowSettingsPatch {
  return {
    episode_ordering: values.ordering,
    episode_group_id: values.ordering === 'episode_group' ? values.groupId || null : null,
    include_specials: values.includeSpecials,
    classifier_mode: values.classifierMode,
  };
}

interface FormProps {
  show: ShowTitle;
  formId: string;
  /** A 422 (no valid group for group ordering) shows at the group picker; anything else above the buttons. */
  error: { field: string | null; form: string | null };
  onSubmit: (patch: ShowSettingsPatch) => void;
}

function ShowSettingsForm({ show, formId, error, onSubmit }: FormProps) {
  const ids = useId();
  const [ordering, setOrdering] = useState(show.episode_ordering);
  const [groupId, setGroupId] = useState(show.episode_group_id ?? '');
  const [includeSpecials, setIncludeSpecials] = useState(show.include_specials);
  const [classifierMode, setClassifierMode] = useState(show.classifier_mode);
  const groupMode = ordering === 'episode_group';
  const formError = error.form ?? (groupMode ? null : error.field);
  const groups = useQuery({ ...episodeGroupsQuery(show.library.library_id ?? 0), enabled: groupMode && show.library.library_id !== null });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit(settingsPatch({ ordering, groupId, includeSpecials, classifierMode }));
  };

  return (
    <form id={formId} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <fieldset className="fs flex flex-col gap-3">
        <legend className="label mb-3">Episode ordering</legend>
        {ORDERINGS.map((option) => (
          <label key={option.value} className="radio-card">
            <input
              type="radio"
              name={`${ids}-ordering`}
              value={option.value}
              checked={ordering === option.value}
              onChange={() => setOrdering(option.value)}
            />
            <span className="flex flex-col gap-1">
              <span className="font-semibold">{option.label}</span>
              <span className="t-small text-fg-2">{option.description}</span>
            </span>
          </label>
        ))}
        <Select
          aria-label="Episode group"
          value={groupId}
          disabled={!groupMode}
          error={groupMode ? (error.field ?? undefined) : undefined}
          onChange={(event) => setGroupId(event.target.value)}
        >
          <option value="">{groupMode && groups.isPending ? 'Loading episode groups…' : 'Choose an episode group…'}</option>
          {groups.data?.items.map((group) => (
            <option key={group.id} value={group.id}>
              {group.name} · {group.episode_count} episodes
            </option>
          ))}
        </Select>
      </fieldset>

      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col">
          <label htmlFor={`${ids}-specials`} className="font-medium">
            Include specials
          </label>
          <span className="t-small text-fg-3" id={`${ids}-specials-help`}>
            Season 0: recaps, OVAs, mini anime
          </span>
        </div>
        <Switch id={`${ids}-specials`} checked={includeSpecials} onCheckedChange={setIncludeSpecials} />
      </div>

      <Select
        label="File matching"
        help="AI only sees file names and episode lists."
        value={classifierMode}
        onChange={(event) => setClassifierMode(CLASSIFIER_MODES.find((mode) => mode.value === event.target.value)?.value ?? classifierMode)}
      >
        {CLASSIFIER_MODES.map((mode) => (
          <option key={mode.value} value={mode.value}>
            {mode.label}
          </option>
        ))}
      </Select>
      {formError && (
        <Banner tone="error" role="alert" title="Couldn’t save">
          {formError}
        </Banner>
      )}
    </form>
  );
}

/**
 * Show settings (`?sheet=show-settings&tmdb=…`): episode ordering (aired,
 * absolute or a TMDB episode group), specials and file matching. Save sends
 * PATCH /shows/{id}, then refetches the show and its seasons.
 */
export function ShowSettingsSheet({ params, close }: SheetComponentProps) {
  const formId = useId();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const tmdbId = Number(params.get('tmdb'));
  const valid = Number.isInteger(tmdbId) && tmdbId > 0;
  const query = useQuery({ ...titleQuery('tv', tmdbId), enabled: valid });
  const show = query.data?.media_type === 'tv' ? query.data : undefined;
  const libraryId = show?.library.library_id ?? null;

  const save = useMutation({
    mutationFn: (patch: ShowSettingsPatch) => titlesApi.patchShow(libraryId ?? 0, patch),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: titleKeys.detail('tv', tmdbId) }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
      ]);
      toast({ tone: 'ready', title: 'Show settings saved', body: show?.title });
      close();
    },
  });
  const invalid = isApiError(save.error) && save.error.status === 422;
  const error = {
    field: invalid ? (save.error?.message ?? null) : null,
    form: save.error && !invalid ? `${save.error.message} Try again in a moment.` : null,
  };

  return (
    <Sheet
      open
      onOpenChange={(open) => !open && close()}
      title={show ? `Show settings · ${show.title}` : 'Show settings'}
      size="narrow"
      footer={
        libraryId !== null && (
          <>
            <Button className="grow" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" className="grow" type="submit" form={formId} loading={save.isPending}>
              Save
            </Button>
          </>
        )
      }
    >
      {valid && query.isPending ? (
        <LoadingRegion label="Loading the show’s settings" className="flex flex-col gap-3">
          <Skeleton shape="line" width={120} />
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} height={88} className="rounded-md" />
          ))}
          <SkeletonText lines={2} />
        </LoadingRegion>
      ) : show && libraryId !== null ? (
        <ShowSettingsForm show={show} formId={formId} error={error} onSubmit={(patch) => save.mutate(patch)} />
      ) : (
        <p className="text-fg-2">Add the show to your library to choose how its episodes are numbered and matched.</p>
      )}
    </Sheet>
  );
}
