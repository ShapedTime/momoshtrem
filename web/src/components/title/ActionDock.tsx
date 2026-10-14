import { Copy } from 'lucide-react';
import { IconButton, useCopy } from '@/components/ui';
import type { StageCopy, StageIntent } from '@/lib/title/stage-copy';
import { StageButton, type StageActionHandler } from './StageButton';

interface ActionDockProps {
  copy: StageCopy;
  onAction: StageActionHandler;
  busy?: StageIntent | null;
}

/**
 * Phone and tablet (< 1024): the stage's main button in thumb reach above
 * the tab bar, plus Copy path once the title is ready (HANDOFF §4.2).
 */
export function ActionDock({ copy, onAction, busy }: ActionDockProps) {
  const copyText = useCopy();
  const path = copy.dock.copyPath ? copy.path : null;
  return (
    <div className="action-dock">
      <StageButton spec={copy.dock.button} onAction={onAction} busy={busy} className="grow" />
      {path && (
        <IconButton
          variant="secondary"
          size="lg"
          aria-label="Copy WebDAV path"
          onClick={() => copyText(path.copy, 'Paste it into Infuse › Add files › WebDAV')}
        >
          <Copy size={20} aria-hidden="true" />
        </IconButton>
      )}
    </div>
  );
}
