'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { Banner, Button, LoadingRegion, Skeleton, useToast } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import type { StorageSettingsPatch } from '@/lib/api/contract';
import { downloadKeys } from '@/lib/api/endpoints/downloads';
import { isLimitBelowCommittedError, settingsApi, settingsKeys, storageSettingsQuery } from '@/lib/api/endpoints/settings';
import {
  belowCommittedCopy,
  FIELD_LABELS,
  fieldErrorCopy,
  firstError,
  hasChanges,
  isStorageField,
  type StorageDraft,
  type StorageField,
  storageChanges,
  withEdits,
} from '@/lib/settings/storage';
import { SaveBar } from './SaveBar';
import { BackgroundCard } from './storage/BackgroundCard';
import { DiskCard } from './storage/DiskCard';
import { DownloadsCard } from './storage/DownloadsCard';
import { LimitCard } from './storage/LimitCard';

/** Where a problem with each field shows: its row (scrolled to) and its control (focused). */
const FIELD_TARGET: Record<StorageField, { row: string; control: string }> = {
  limit_bytes: { row: 'st-limit', control: '#st-limit button' },
  'kept.max_bytes': { row: 'st-kept-max', control: '#st-kept-max-input' },
  'kept.default_until_watched': { row: 'st-until-watched', control: '#st-until-watched-input' },
  'kept.default_days': { row: 'st-days', control: '#st-days-input' },
  'smart.enabled': { row: 'st-smart', control: '#st-smart-input' },
  'smart.episodes_ahead': { row: 'st-ahead', control: '#st-ahead button[aria-pressed="true"]' },
  'smart.max_bytes': { row: 'st-smart-max', control: '#st-smart-max-input' },
  'background.enabled': { row: 'st-bg', control: '#st-bg-input' },
  'background.pause_while_streaming': { row: 'st-pause', control: '#st-pause-input' },
  'background.windows': { row: 'st-windows', control: '#st-windows input[aria-invalid="true"], #st-windows-add' },
};

/** Bring a field with a problem into view and move focus to it (from Save). The limit's is on its size while that's custom. */
function showField(field: StorageField) {
  const custom = field === 'limit_bytes' ? document.getElementById('st-limit-custom') : null;
  const { row, control } = custom ? { row: 'st-limit-custom', control: '#st-limit-custom-input' } : FIELD_TARGET[field];
  document.getElementById(row)?.scrollIntoView?.({ block: 'center' });
  document.querySelector<HTMLElement>(control)?.focus({ preventScroll: true });
}

function StorageSkeleton() {
  return (
    <LoadingRegion label="Loading the storage settings" className="flex flex-col gap-4 lg:gap-5">
      <Skeleton height={190} className="rounded-lg" />
      <Skeleton height={200} className="rounded-lg" />
      <Skeleton height={470} className="rounded-lg" />
      <Skeleton height={290} className="rounded-lg" />
    </LoadingRegion>
  );
}

/**
 * Storage (PLAN §6 "Storage (#77)"; no design screen, its copy is PLAN's
 * "Storage and downloads: UI copy"): where the disk goes, the piece cache's
 * limit, the space for kept and smart downloads, and when background
 * downloads run. Edits stay local until Save sends only what changed (PUT
 * /settings/storage is partial, and `limit_bytes` goes only when the limit
 * control changed); a 422 shows next to the field it names, and a 409
 * `limit_below_committed` in the bar.
 */
export function StorageSection() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const settings = useQuery(storageSettingsQuery());
  // Only the fields touched here: the rest follows the server, so a stale page never sends them back
  const [edits, setEdits] = useState<Partial<StorageDraft>>({});
  // Sizes and windows that don't read show once Save was pressed, not while they're typed
  const [triedSave, setTriedSave] = useState(false);

  const save = useMutation({
    mutationFn: (patch: StorageSettingsPatch) => settingsApi.putStorage(patch),
    onSuccess: (saved) => {
      queryClient.setQueryData(settingsKeys.storage, saved);
      setEdits({});
      setTriedSave(false);
      toast({ tone: 'ready', title: 'Settings saved' });
      // The caps and the waits move with the settings
      void queryClient.invalidateQueries({ queryKey: downloadKeys.all });
    },
    onError: (error) => {
      if (isApiError(error) && isStorageField(error.field)) showField(error.field);
    },
  });

  if (settings.isPending) return <StorageSkeleton />;

  if (settings.isError) {
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load the storage settings"
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
  const changes = storageChanges(edits, saved);
  const dirty = hasChanges(changes);

  const apiError = isApiError(save.error) ? save.error : null;
  const invalid = apiError?.status === 422;
  const serverField = apiError && invalid && isStorageField(apiError.field) ? apiError.field : null;
  const draftErrors = triedSave ? changes.errors : {};
  const errorFor = (field: StorageField): string | null => {
    const draftError = draftErrors[field];
    if (draftError) return draftError;
    return serverField === field && apiError ? fieldErrorCopy(field, apiError.message) : null;
  };
  // A field's problem shows there; the bar points at it. Anything else shows in the bar.
  const problemField = firstError(draftErrors) ?? serverField;
  // The limit's problem shows on its size while that's custom
  const problemLabel = problemField === 'limit_bytes' && current.limitMode === 'custom' ? 'Custom limit' : problemField && FIELD_LABELS[problemField];
  const barError = problemLabel
    ? `Check “${problemLabel}” above.`
    : isLimitBelowCommittedError(save.error)
      ? belowCommittedCopy(save.error.details.committed_kept_bytes)
      : save.error
        ? invalid
          ? save.error.message
          : `Couldn’t save: ${save.error.message} Try again in a moment.`
        : null;

  const change = (next: Partial<StorageDraft>) => {
    setEdits((previous) => ({ ...previous, ...next }));
    if (save.error) save.reset();
  };

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <DiskCard settings={saved} />
      <LimitCard draft={current} saved={saved} onChange={change} errorFor={errorFor} />
      <DownloadsCard draft={current} saved={saved} onChange={change} errorFor={errorFor} />
      <BackgroundCard
        draft={current}
        onChange={change}
        errorFor={errorFor}
        invalidWindow={draftErrors['background.windows'] ? changes.windowIndex : null}
      />

      {dirty && (
        <SaveBar
          saving={save.isPending}
          note="Saved settings apply right away; a lower limit trims the cache step by step."
          error={barError}
          onDiscard={() => {
            setEdits({});
            setTriedSave(false);
            save.reset();
          }}
          onSave={() => {
            const field = firstError(changes.errors);
            if (field) {
              setTriedSave(true);
              // After the render that marks it
              requestAnimationFrame(() => showField(field));
              return;
            }
            save.mutate(changes.patch);
          }}
        />
      )}
    </div>
  );
}
