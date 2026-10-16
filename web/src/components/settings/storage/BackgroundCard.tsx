import { Plus, X } from 'lucide-react';
import { Button, IconButton, Switch } from '@/components/ui';
import type { DownloadWindow } from '@/lib/api/contract';
import {
  backgroundHelp,
  crossesMidnight,
  focusAfterRemove,
  MAX_WINDOWS,
  NEW_WINDOW,
  PAUSE_HELP,
  type StorageDraft,
  type StorageField,
  windowsHelp,
} from '@/lib/settings/storage';
import { rowDescribedBy, SettingRow, SettingsCard } from '../SettingsCard';

interface BackgroundCardProps {
  draft: StorageDraft;
  onChange: (next: Partial<StorageDraft>) => void;
  errorFor: (field: StorageField) => string | null;
  /** The window a problem is about, to mark its times. */
  invalidWindow: number | null;
}

interface WindowsFieldProps {
  windows: DownloadWindow[];
  onChange: (windows: DownloadWindow[]) => void;
  error: string | null;
  invalidWindow: number | null;
}

/** Up to 4 HH:MM–HH:MM windows in the server's time; one may cross midnight. Empty is any time. */
function WindowsField({ windows, onChange, error, invalidWindow }: WindowsFieldProps) {
  const describedBy = rowDescribedBy('st-windows', error);
  const set = (index: number, next: Partial<DownloadWindow>) => onChange(windows.map((window, i) => (i === index ? { ...window, ...next } : window)));
  const remove = (index: number) => {
    onChange(windows.filter((_, i) => i !== index));
    // Its button goes with the row: focus the next row's Remove, else the previous one's, else Add
    const target = focusAfterRemove(index, windows.length);
    requestAnimationFrame(() => document.getElementById(target === 'add' ? 'st-windows-add' : `st-window-${target}-remove`)?.focus());
  };
  return (
    <div className="set-windows">
      {windows.length > 0 && (
        <ul aria-label="Download windows">
          {windows.map((window, index) => {
            const n = index + 1;
            const invalid = invalidWindow === index || undefined;
            return (
              <li key={index} className="set-window">
                <input
                  id={`st-window-${index}-start`}
                  className="input"
                  type="time"
                  value={window.start}
                  aria-label={`Window ${n} starts`}
                  aria-invalid={invalid}
                  aria-describedby={describedBy}
                  onChange={(event) => set(index, { start: event.target.value })}
                />
                <span aria-hidden="true" className="text-fg-3">
                  –
                </span>
                <input
                  id={`st-window-${index}-end`}
                  className="input"
                  type="time"
                  value={window.end}
                  aria-label={`Window ${n} ends`}
                  aria-invalid={invalid}
                  aria-describedby={describedBy}
                  onChange={(event) => set(index, { end: event.target.value })}
                />
                <IconButton id={`st-window-${index}-remove`} aria-label={`Remove window ${n}`} onClick={() => remove(index)}>
                  <X size={18} aria-hidden="true" />
                </IconButton>
                {crossesMidnight(window) && <span className="set-window-next t-small text-fg-3">Ends the next day</span>}
              </li>
            );
          })}
        </ul>
      )}
      <Button
        id="st-windows-add"
        variant="secondary"
        size="sm"
        disabled={windows.length >= MAX_WINDOWS}
        aria-describedby={describedBy}
        onClick={() => onChange([...windows, { ...NEW_WINDOW }])}
      >
        <Plus size={15} aria-hidden="true" /> Add a window
      </Button>
    </div>
  );
}

/**
 * Background downloads: the kill switch for kept and smart ones, the pause
 * while anything streams, and the windows they may run in.
 */
export function BackgroundCard({ draft, onChange, errorFor, invalidWindow }: BackgroundCardProps) {
  const windowsError = errorFor('background.windows');
  return (
    <SettingsCard id="st-background" title="Background">
      <SettingRow
        id="st-bg"
        label="Background downloads"
        labelFor="st-bg-input"
        help={backgroundHelp(draft.backgroundEnabled)}
        error={errorFor('background.enabled')}
      >
        <Switch
          id="st-bg-input"
          describedBy={rowDescribedBy('st-bg', errorFor('background.enabled'))}
          checked={draft.backgroundEnabled}
          onCheckedChange={(backgroundEnabled) => onChange({ backgroundEnabled })}
        />
      </SettingRow>

      <SettingRow id="st-pause" label="Pause while streaming" labelFor="st-pause-input" help={PAUSE_HELP} error={errorFor('background.pause_while_streaming')}>
        <Switch
          id="st-pause-input"
          describedBy={rowDescribedBy('st-pause', errorFor('background.pause_while_streaming'))}
          checked={draft.pauseWhileStreaming}
          onCheckedChange={(pauseWhileStreaming) => onChange({ pauseWhileStreaming })}
        />
      </SettingRow>

      <SettingRow id="st-windows" label="Download windows" help={windowsHelp(draft.windows.length)} error={windowsError} stack>
        <WindowsField windows={draft.windows} onChange={(windows) => onChange({ windows })} error={windowsError} invalidWindow={invalidWindow} />
      </SettingRow>
    </SettingsCard>
  );
}
