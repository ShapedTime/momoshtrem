import { useId } from 'react';
import { Play } from 'lucide-react';
import { CopyField, FilePath, Progress } from '@/components/ui';
import type { StageCopy, StageIntent } from '@/lib/title/stage-copy';
import { StageButton, type StageActionHandler } from './StageButton';
import { StageSteps } from './StageSteps';

/** Facts, then the bold lead, then the hint: "9 seeders. You can start watching in Infuse now; it streams…". */
function StageHint({ copy }: { copy: StageCopy }) {
  const facts = copy.facts.join(' · ');
  if (!facts && !copy.lead && !copy.hint) return null;
  return (
    <p className="hint t-small text-fg-2">
      {facts}
      {facts && (copy.lead || copy.hint) ? '. ' : ''}
      {copy.lead && <strong>{copy.lead}</strong>}
      {copy.lead && copy.hint ? ' ' : ''}
      {copy.hint}
    </p>
  );
}

interface PrimaryPanelProps {
  copy: StageCopy;
  onAction: StageActionHandler;
  /** The intent whose action is running. */
  busy?: StageIntent | null;
  /** Names the progress bar: "Interstellar download". */
  titleName: string;
}

/**
 * The state machine panel (HANDOFF §5.3): steps, what's true now, and the
 * one next thing to do. On phone its buttons move to the ActionDock; the
 * panel keeps the state, steps and hint.
 */
export function PrimaryPanel({ copy, onAction, busy, titleName }: PrimaryPanelProps) {
  const headingId = useId();
  const heading = (
    <h2 className="pp-title" id={headingId}>
      {copy.title}
    </h2>
  );
  return (
    <section className="primary-panel glass" aria-labelledby={headingId}>
      <StageSteps step={copy.step} />
      {copy.stage === 'ready' ? (
        <div className="ready-head">
          <span className="ic" aria-hidden="true">
            <Play size={18} fill="currentColor" />
          </span>
          <div className="flex min-w-0 flex-col gap-0.5">
            {heading}
            <StageHint copy={copy} />
          </div>
        </div>
      ) : copy.stage === 'downloading' ? (
        <>
          <div className="flex items-center justify-between gap-3">
            {heading}
            <span className="pp-pct">{copy.pct}%</span>
          </div>
          <Progress value={copy.pct ?? 0} size="lg" label={`${titleName} download`} />
          <StageHint copy={copy} />
        </>
      ) : (
        <>
          {heading}
          <StageHint copy={copy} />
        </>
      )}
      {copy.path && (
        <div className="flex min-w-0 flex-col gap-2">
          <span className="t-micro text-fg-3">WebDAV path</span>
          <CopyField value={copy.path.copy} label="WebDAV path" hint="Paste it into Infuse › Add files › WebDAV">
            <FilePath path={copy.path.display} />
          </CopyField>
        </div>
      )}
      <div className="hidden flex-col gap-2 lg:flex">
        <StageButton spec={copy.primary} onAction={onAction} busy={busy} block />
        {copy.secondary && <StageButton spec={copy.secondary} onAction={onAction} busy={busy} block />}
      </div>
    </section>
  );
}
