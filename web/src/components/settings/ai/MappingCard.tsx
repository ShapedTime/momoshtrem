import { Switch } from '@/components/ui';
import type { ClassifierModel } from '@/lib/api/contract';
import { type AiDraft, type AiField, classifierHelp } from '@/lib/settings/ai';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';

interface MappingCardProps {
  draft: AiDraft;
  /** The model the classifier really uses (`classifier_model`), named in the help. */
  classifierModel: ClassifierModel;
  onChange: (next: Partial<AiDraft>) => void;
  errorFor: (field: AiField) => string | null;
}

/** Episode mapping (#5's classifier): both switches save with or without an API key. */
export function MappingCard({ draft, classifierModel, onChange, errorFor }: MappingCardProps) {
  return (
    <SettingsCard id="ai-map" title="Episode mapping">
      <SettingRow
        id="ai-classifier"
        label="Ask AI about tricky files"
        labelFor="ai-classifier-input"
        help={classifierHelp(classifierModel)}
        error={errorFor('classifier_enabled')}
      >
        <Switch
          id="ai-classifier-input"
          describedBy={rowDescribedBy('ai-classifier', errorFor('classifier_enabled'))}
          checked={draft.classifier_enabled}
          onCheckedChange={(classifier_enabled) => onChange({ classifier_enabled })}
        />
      </SettingRow>
      <SettingRow
        id="ai-autoapply"
        label="Auto-apply high-confidence mappings"
        labelFor="ai-autoapply-input"
        help="Applied without review. Medium and low confidence always wait for you."
        error={errorFor('auto_apply_high_confidence')}
      >
        <Switch
          id="ai-autoapply-input"
          describedBy={rowDescribedBy('ai-autoapply', errorFor('auto_apply_high_confidence'))}
          checked={draft.auto_apply_high_confidence}
          onCheckedChange={(auto_apply_high_confidence) => onChange({ auto_apply_high_confidence })}
        />
      </SettingRow>
    </SettingsCard>
  );
}
