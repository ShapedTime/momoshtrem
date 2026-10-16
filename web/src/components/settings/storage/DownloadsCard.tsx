import { SegmentedControl, Select, Switch } from '@/components/ui';
import type { StorageSettings } from '@/lib/api/contract';
import {
  DAYS_LABELS,
  EPISODES_AHEAD,
  KEEP_DAYS,
  keepDefaultsLine,
  keptCapHelp,
  reserveLine,
  smartCapHelp,
  smartHelp,
  type StorageDraft,
  type StorageField,
  untilWatchedHelp,
} from '@/lib/settings/storage';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';
import { SizeInput } from './SizeInput';

interface DownloadsCardProps {
  draft: StorageDraft;
  saved: StorageSettings;
  onChange: (next: Partial<StorageDraft>) => void;
  errorFor: (field: StorageField) => string | null;
}

const NO_MAXIMUM = 'none';

/**
 * Kept and smart downloads: each one's space inside the limit (empty is the
 * default share), what a new Keep defaults to, and smart downloads with how
 * far ahead they look.
 */
export function DownloadsCard({ draft, saved, onChange, errorFor }: DownloadsCardProps) {
  const keptError = errorFor('kept.max_bytes');
  const smartError = errorFor('smart.max_bytes');
  return (
    <SettingsCard id="st-downloads" title="Downloads" description={reserveLine(saved)}>
      <SettingRow id="st-kept-max" label="Space for kept downloads" labelFor="st-kept-max-input" help={keptCapHelp(saved)} error={keptError}>
        <SizeInput
          id="st-kept-max-input"
          placeholder="Default"
          value={draft.keptMax}
          error={keptError}
          describedBy={rowDescribedBy('st-kept-max', keptError)}
          onChange={(keptMax) => onChange({ keptMax })}
        />
      </SettingRow>

      <SettingRow
        id="st-until-watched"
        label="Keep until watched"
        labelFor="st-until-watched-input"
        help={untilWatchedHelp(draft.untilWatched)}
        error={errorFor('kept.default_until_watched')}
      >
        <Switch
          id="st-until-watched-input"
          describedBy={rowDescribedBy('st-until-watched', errorFor('kept.default_until_watched'))}
          checked={draft.untilWatched}
          onCheckedChange={(untilWatched) => onChange({ untilWatched })}
        />
      </SettingRow>

      <SettingRow
        id="st-days"
        label="Keep for at most"
        labelFor="st-days-input"
        help={`${keepDefaultsLine(draft.untilWatched, draft.days)} You can pick another for each keep.`}
        error={errorFor('kept.default_days')}
      >
        <Select
          id="st-days-input"
          className="w-40"
          value={draft.days === null ? NO_MAXIMUM : String(draft.days)}
          aria-describedby={rowDescribedBy('st-days', errorFor('kept.default_days'))}
          onChange={(event) => onChange({ days: KEEP_DAYS.find((days) => String(days) === event.target.value) ?? null })}
        >
          {KEEP_DAYS.map((days) => (
            <option key={days} value={days}>
              {DAYS_LABELS[days]}
            </option>
          ))}
          <option value={NO_MAXIMUM}>{DAYS_LABELS.none}</option>
        </Select>
      </SettingRow>

      <SettingRow id="st-smart" label="Smart downloads" labelFor="st-smart-input" help={smartHelp(draft.smartEnabled)} error={errorFor('smart.enabled')}>
        <Switch
          id="st-smart-input"
          describedBy={rowDescribedBy('st-smart', errorFor('smart.enabled'))}
          checked={draft.smartEnabled}
          onCheckedChange={(smartEnabled) => onChange({ smartEnabled })}
        />
      </SettingRow>

      <SettingRow id="st-ahead" label="Episodes ahead" help="How many of each show’s next episodes to fetch." error={errorFor('smart.episodes_ahead')} stack>
        <SegmentedControl
          aria-label="Episodes ahead"
          aria-describedby={rowDescribedBy('st-ahead', errorFor('smart.episodes_ahead'))}
          options={EPISODES_AHEAD.map((count) => ({ value: String(count), label: String(count) }))}
          value={String(draft.episodesAhead)}
          onChange={(value) => onChange({ episodesAhead: Number(value) })}
        />
      </SettingRow>

      <SettingRow id="st-smart-max" label="Space for smart downloads" labelFor="st-smart-max-input" help={smartCapHelp(saved)} error={smartError}>
        <SizeInput
          id="st-smart-max-input"
          placeholder="Default"
          value={draft.smartMax}
          error={smartError}
          describedBy={rowDescribedBy('st-smart-max', smartError)}
          onChange={(smartMax) => onChange({ smartMax })}
        />
      </SettingRow>
    </SettingsCard>
  );
}
