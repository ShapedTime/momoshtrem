import { Banner, Button, SegmentedControl } from '@/components/ui';
import type { StorageSettings } from '@/lib/api/contract';
import { configLine, customLimitHelp, type LimitMode, limitHelp, type StorageDraft, type StorageField } from '@/lib/settings/storage';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';
import { SizeInput } from './SizeInput';

interface LimitCardProps {
  draft: StorageDraft;
  saved: StorageSettings;
  onChange: (next: Partial<StorageDraft>) => void;
  errorFor: (field: StorageField) => string | null;
}

const MODES: Array<{ value: Exclude<LimitMode, 'config'>; label: string }> = [
  { value: 'auto', label: 'Auto' },
  { value: 'custom', label: 'Custom' },
];

/**
 * How much of the disk the piece cache may use: Auto (75% of the disk) or a
 * custom size. While config.yaml's `global_cache_size` sets it, neither is
 * chosen and a banner offers "Use auto". Save sends `limit_bytes` only when
 * this control changed.
 */
export function LimitCard({ draft, saved, onChange, errorFor }: LimitCardProps) {
  const custom = draft.limitMode === 'custom';
  // A problem with the limit shows on the size while it's custom, else on the choice
  const modeError = custom ? null : errorFor('limit_bytes');
  const sizeError = custom ? errorFor('limit_bytes') : null;
  const useAuto = () => {
    onChange({ limitMode: 'auto' });
    // The banner and its button go: focus lands on the choice it made
    requestAnimationFrame(() => document.querySelector<HTMLElement>('#st-limit button[aria-pressed="true"]')?.focus());
  };
  return (
    <SettingsCard id="st-limit-card" title="Limit" description="Streaming, kept and smart downloads share it.">
      {draft.limitMode === 'config' && (
        <div className="set-card-note">
          <Banner
            title={configLine(saved)}
            action={
              <Button variant="secondary" size="sm" onClick={useAuto}>
                Use auto
              </Button>
            }
          >
            config.yaml’s <code className="t-mono text-fg-1">global_cache_size</code> sets this limit until you choose one here. Auto follows the disk.
          </Banner>
        </div>
      )}

      <SettingRow id="st-limit" label="Storage limit" help={limitHelp(draft.limitMode, saved)} error={modeError} stack>
        <SegmentedControl
          aria-label="Storage limit"
          aria-describedby={rowDescribedBy('st-limit', modeError)}
          options={MODES}
          value={draft.limitMode}
          onChange={(limitMode) => onChange({ limitMode })}
        />
      </SettingRow>

      {custom && (
        <SettingRow id="st-limit-custom" label="Custom limit" labelFor="st-limit-custom-input" help={customLimitHelp(saved)} error={sizeError}>
          <SizeInput
            id="st-limit-custom-input"
            value={draft.limit}
            error={sizeError}
            describedBy={rowDescribedBy('st-limit-custom', sizeError)}
            onChange={(limit) => onChange({ limit })}
          />
        </SettingRow>
      )}
    </SettingsCard>
  );
}
