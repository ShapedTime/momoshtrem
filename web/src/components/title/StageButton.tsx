import { Activity, type LucideIcon, MonitorPlay, Plus, Search, Tv } from 'lucide-react';
import { Button, ButtonLink } from '@/components/ui';
import type { StageButton as StageButtonSpec, StageIcon, StageIntent } from '@/lib/title/stage-copy';

export const STAGE_ICONS: Record<StageIcon, LucideIcon> = { plus: Plus, search: Search, activity: Activity, tv: Tv, stream: MonitorPlay };

export type StageActionHandler = (intent: StageIntent) => void;

interface StageButtonProps {
  spec: StageButtonSpec;
  onAction: StageActionHandler;
  /** The intent whose action is running (shows a spinner on its button). */
  busy?: StageIntent | null;
  block?: boolean;
  className?: string;
}

/** One stage button: a link when it navigates, otherwise an action handed to `onAction`. */
export function StageButton({ spec, onAction, busy, block, className }: StageButtonProps) {
  const Icon = STAGE_ICONS[spec.icon];
  const content = (
    <>
      <Icon size={20} aria-hidden="true" /> {spec.label}
    </>
  );
  if (spec.href) {
    return (
      <ButtonLink href={spec.href} variant={spec.variant} size="lg" block={block} className={className}>
        {content}
      </ButtonLink>
    );
  }
  return (
    <Button
      variant={spec.variant}
      size={spec.variant === 'ghost' ? 'md' : 'lg'}
      block={block}
      className={className}
      loading={busy === spec.intent}
      onClick={() => onAction(spec.intent)}
    >
      {content}
    </Button>
  );
}
