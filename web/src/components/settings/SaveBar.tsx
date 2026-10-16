import { Button } from '@/components/ui';

interface SaveBarProps {
  saving: boolean;
  onSave: () => void;
  /** Back to what was loaded. */
  onDiscard: () => void;
  /** When saved settings take effect, e.g. "Saved settings apply to the next search." Hidden on phone. */
  note: string;
  /** Why the last save failed; shown in place of the note. */
  error?: string | null;
}

/**
 * The settings save bar (HANDOFF §4.2): render it only while something
 * changed. It floats above the tab bar on phone and sits bottom-centre,
 * 640 px wide, on desktop (settings.css).
 */
export function SaveBar({ saving, onSave, onDiscard, note, error }: SaveBarProps) {
  return (
    <div className="set-savebar" role="region" aria-label="Unsaved changes">
      <div className="grow">
        <strong>Unsaved changes</strong>
        {error ? (
          <span role="alert" className="t-small text-error">
            {error}
          </span>
        ) : (
          <span className="t-small text-fg-3 max-sm:hidden">{note}</span>
        )}
      </div>
      <Button variant="ghost" disabled={saving} onClick={onDiscard}>
        Discard
      </Button>
      <Button variant="primary" loading={saving} onClick={onSave}>
        Save
      </Button>
    </div>
  );
}
