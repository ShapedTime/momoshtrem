import { Copy } from 'lucide-react';
import { IconButton, useCopy } from '@/components/ui';
import type { StageCopy, StageIntent } from '@/lib/title/stage-copy';
import { STAGE_ICONS, StageButton, type StageActionHandler } from './StageButton';

interface ActionDockProps {
  copy: StageCopy;
  onAction: StageActionHandler;
  busy?: StageIntent | null;
}

/**
 * Phone and tablet (< 1024): the stage's main button in thumb reach above
 * the tab bar, plus one icon action (HANDOFF §4.2): Copy path once the title
 * is ready, or Stream from HDRezka while it needs a file.
 */
export function ActionDock({ copy, onAction, busy }: ActionDockProps) {
  const copyText = useCopy();
  const path = copy.dock.copyPath ? copy.path : null;
  const icon = copy.dock.icon;
  const Icon = icon ? STAGE_ICONS[icon.icon] : null;
  return (
    <div className="action-dock">
      <StageButton spec={copy.dock.button} onAction={onAction} busy={busy} className="grow" />
      {icon && Icon && (
        <IconButton variant="secondary" size="lg" aria-label={icon.label} loading={busy === icon.intent} onClick={() => onAction(icon.intent)}>
          <Icon size={20} aria-hidden="true" />
        </IconButton>
      )}
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
