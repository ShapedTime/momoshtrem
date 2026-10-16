'use client';

import { type CSSProperties, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { RefreshCw, Sparkle } from 'lucide-react';
import { Banner, Button, LoadingRegion, SegmentedControl, Skeleton, Switch, useToast } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import type { QualityProfile } from '@/lib/api/contract';
import { qualityProfileQuery, searchApi, searchKeys } from '@/lib/api/endpoints/search';
import {
  FIELD_LABELS,
  formatGbPerHour,
  GB_PER_HOUR,
  isQualityField,
  MAX_RESOLUTIONS,
  maxAllowed,
  PREFERRED_RESOLUTIONS,
  type QualityField,
  qualityPreview,
  sameProfile,
  sizeExample,
  withPreferredResolution,
} from '@/lib/settings/quality';
import { SaveBar } from './SaveBar';
import { rowDescribedBy, SettingRow, SettingsCard } from './SettingsCard';
import { TagField } from './TagField';

/** Where a 422 about each field shows (and what to scroll to). */
const FIELD_TARGET: Record<QualityField, string> = {
  preferred_resolution: 'q-pref',
  max_resolution: 'q-max',
  max_gb_per_hour: 'q-gbh',
  prefer_x265: 'q-x265',
  prefer_hdr: 'q-hdr',
  prefer_dolby_vision: 'q-dv',
  avoid_terms: 'q-avoid-input',
  preferred_groups: 'q-groups-input',
};

const SCALE = [1, 5, 10, 15, 20];
const NO_LIMIT = 'none';

const SWITCHES: Array<{ field: 'prefer_x265' | 'prefer_hdr' | 'prefer_dolby_vision'; help: string }> = [
  { field: 'prefer_x265', help: 'Smaller files at the same quality. Apple TV 4K plays it natively.' },
  { field: 'prefer_hdr', help: 'HDR10 and HDR10+ when available.' },
  { field: 'prefer_dolby_vision', help: 'Infuse plays Dolby Vision on Apple TV 4K.' },
];

/** The sticky "Best match would prefer…" line, built from the profile on screen. */
function QualityPreviewCard({ profile }: { profile: QualityProfile }) {
  const { main, sub } = qualityPreview(profile);
  return (
    <div className="set-preview" role="status" aria-labelledby="q-preview-h">
      <Sparkle size={18} aria-hidden="true" />
      <div className="flex min-w-0 grow flex-col gap-px">
        <span id="q-preview-h" className="t-small text-fg-2">
          Best match would prefer
        </span>
        <strong>{main}</strong>
        {sub && <span className="t-small text-fg-3">{sub}</span>}
      </div>
    </div>
  );
}

function QualitySkeleton() {
  return (
    <LoadingRegion label="Loading your quality profile" className="flex flex-col gap-4 lg:gap-5">
      <Skeleton height={76} className="rounded-lg" />
      <Skeleton height={360} className="rounded-lg" />
      <Skeleton height={270} className="rounded-lg" />
      <Skeleton height={190} className="rounded-lg" />
      <Skeleton height={190} className="rounded-lg" />
    </LoadingRegion>
  );
}

/**
 * Quality profile (screens/settings/quality): what "Best match" means in
 * torrent search. Edits stay local until Save sends the whole profile
 * (PUT /settings/quality-profile); a 422 shows next to the field it names.
 */
export function QualitySection() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const query = useQuery(qualityProfileQuery());
  // Null until something is edited, so a refetch shows through while nothing is
  const [draft, setDraft] = useState<QualityProfile | null>(null);

  const save = useMutation({
    mutationFn: (profile: QualityProfile) => searchApi.putQualityProfile(profile),
    onSuccess: (saved) => {
      queryClient.setQueryData(searchKeys.qualityProfile, saved);
      setDraft(null);
      toast({ tone: 'ready', title: 'Settings saved' });
    },
    onError: (error) => {
      if (isApiError(error) && isQualityField(error.field)) {
        document.getElementById(FIELD_TARGET[error.field])?.scrollIntoView?.({ block: 'center' });
      }
    },
  });

  if (query.isPending) return <QualitySkeleton />;

  if (query.isError) {
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load your quality profile"
        action={
          <Button size="sm" onClick={() => void query.refetch()}>
            <RefreshCw size={15} aria-hidden="true" /> Try again
          </Button>
        }
      >
        {query.error.message}
      </Banner>
    );
  }

  const saved = query.data;
  const profile = draft ?? saved;
  const dirty = draft !== null && !sameProfile(draft, saved);

  const apiError = isApiError(save.error) ? save.error : null;
  const invalid = apiError?.status === 422;
  const fieldError = apiError && invalid && isQualityField(apiError.field) ? { field: apiError.field, message: apiError.message } : null;
  const errorFor = (field: QualityField) => (fieldError?.field === field ? fieldError.message : null);
  // A 422 about one field shows there; the bar points at it. Anything else shows in the bar.
  const barError = fieldError
    ? `Check “${FIELD_LABELS[fieldError.field]}” above.`
    : save.error
      ? invalid
        ? save.error.message
        : `Couldn’t save: ${save.error.message} Try again in a moment.`
      : null;

  const change = (next: QualityProfile) => {
    setDraft(next);
    if (save.error) save.reset();
  };

  const gb = profile.max_gb_per_hour;
  const range = { '--v': `${((gb - GB_PER_HOUR.min) / (GB_PER_HOUR.max - GB_PER_HOUR.min)) * 100}%` } as CSSProperties;

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <QualityPreviewCard profile={profile} />

      <SettingsCard id="q-res" title="Resolution and size">
        <SettingRow id="q-pref" label="Preferred resolution" help="Ranked first when seeders are healthy." error={errorFor('preferred_resolution')} stack>
          <SegmentedControl
            aria-label="Preferred resolution"
            aria-describedby={rowDescribedBy('q-pref', errorFor('preferred_resolution'))}
            className="max-sm:w-full"
            options={PREFERRED_RESOLUTIONS.map((value) => ({ value, label: value }))}
            value={profile.preferred_resolution}
            onChange={(value) => change(withPreferredResolution(profile, value))}
          />
        </SettingRow>
        <SettingRow id="q-max" label="Maximum resolution" help="Anything above is hidden from results." error={errorFor('max_resolution')} stack>
          <SegmentedControl
            aria-label="Maximum resolution"
            aria-describedby={rowDescribedBy('q-max', errorFor('max_resolution'))}
            className="max-sm:w-full"
            options={MAX_RESOLUTIONS.map((value) => ({
              value: value ?? NO_LIMIT,
              label: value ?? 'No limit',
              disabled: !maxAllowed(value, profile.preferred_resolution),
            }))}
            value={profile.max_resolution ?? NO_LIMIT}
            onChange={(value) => change({ ...profile, max_resolution: MAX_RESOLUTIONS.find((max) => (max ?? NO_LIMIT) === value) ?? null })}
          />
        </SettingRow>
        <SettingRow id="q-gbh" label="Max size per hour" labelFor="q-gbh-input" help={sizeExample(gb)} error={errorFor('max_gb_per_hour')} stack>
          <div className="set-range">
            <input
              type="range"
              id="q-gbh-input"
              min={GB_PER_HOUR.min}
              max={GB_PER_HOUR.max}
              step={GB_PER_HOUR.step}
              value={gb}
              style={range}
              aria-valuetext={formatGbPerHour(gb)}
              aria-describedby={rowDescribedBy('q-gbh', errorFor('max_gb_per_hour'))}
              onChange={(event) => change({ ...profile, max_gb_per_hour: Number(event.target.value) })}
            />
            <output htmlFor="q-gbh-input">{formatGbPerHour(gb)}</output>
          </div>
          <div className="set-range-scale text-fg-3" aria-hidden="true">
            {SCALE.map((value) => (
              <span key={value} style={{ '--at': (value - GB_PER_HOUR.min) / (GB_PER_HOUR.max - GB_PER_HOUR.min) } as CSSProperties}>
                {value}
              </span>
            ))}
          </div>
        </SettingRow>
      </SettingsCard>

      <SettingsCard id="q-video" title="Video">
        {SWITCHES.map(({ field, help }) => {
          const id = FIELD_TARGET[field];
          return (
            <SettingRow key={field} id={id} label={FIELD_LABELS[field]} labelFor={`${id}-input`} help={help} error={errorFor(field)}>
              <Switch
                id={`${id}-input`}
                describedBy={rowDescribedBy(id, errorFor(field))}
                checked={profile[field]}
                onCheckedChange={(checked) => change({ ...profile, [field]: checked })}
              />
            </SettingRow>
          );
        })}
      </SettingsCard>

      <SettingsCard id="q-avoid" title="Terms to avoid" description="Releases with these words are hidden. CAM and TS are never worth it.">
        <TagField
          id="q-avoid"
          label="Terms to avoid"
          placeholder="Add a term, e.g. HDTS"
          noun="terms"
          values={profile.avoid_terms}
          onChange={(avoid_terms) => change({ ...profile, avoid_terms })}
          serverError={errorFor('avoid_terms')}
        />
      </SettingsCard>

      <SettingsCard id="q-groups" title="Preferred groups" description="Get a small boost in ranking. They never override seeders or your size limit.">
        <TagField
          id="q-groups"
          label="Preferred groups"
          placeholder="Add a group, e.g. Erai-raws"
          noun="groups"
          values={profile.preferred_groups}
          onChange={(preferred_groups) => change({ ...profile, preferred_groups })}
          serverError={errorFor('preferred_groups')}
        />
      </SettingsCard>

      {dirty && (
        <SaveBar
          saving={save.isPending}
          note="Saved settings apply to the next search."
          error={barError}
          onDiscard={() => {
            setDraft(null);
            save.reset();
          }}
          onSave={() => save.mutate(profile)}
        />
      )}
    </div>
  );
}
