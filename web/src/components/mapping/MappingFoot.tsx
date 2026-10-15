'use client';

import { useId } from 'react';
import { Check, CheckCheck, CircleAlert, Info, type LucideIcon, TriangleAlert } from 'lucide-react';
import { Button, ButtonLink } from '@/components/ui';
import type { FootState, FootTone } from '@/lib/mapping/review';
import { cx } from '@/lib/utils/cx';

const ICON: Record<FootTone, LucideIcon> = { conflict: TriangleAlert, blocked: CircleAlert, info: Info, ok: CheckCheck };

interface MappingFootProps {
  state: FootState;
  /** Apply's answer when the server still said no (a 409). */
  error: string | null;
  cancelHref: string;
  applying: boolean;
  onApply: () => void;
}

/**
 * The sticky foot (HANDOFF §4.2): what's left to do, Cancel (the job stays
 * open) and "Apply N mappings". While something blocks it the button looks
 * disabled but stays clickable: a click takes you to the first blocker.
 */
export function MappingFoot({ state, error, cancelHref, applying, onApply }: MappingFootProps) {
  const messageId = useId();
  const tone: FootTone = error ? 'conflict' : state.tone;
  const Icon = ICON[tone];
  return (
    <div className="map-foot">
      <div className="map-foot-in">
        <p className={cx('map-foot-msg', `is-${tone}`)} id={messageId} role={error ? 'alert' : undefined}>
          <Icon size={16} aria-hidden="true" />
          <span>{error ?? state.message}</span>
        </p>
        <div className="map-foot-actions">
          <ButtonLink href={cancelHref} variant="ghost">
            Cancel
          </ButtonLink>
          <Button
            variant="primary"
            className="map-apply"
            aria-disabled={state.blocked}
            aria-describedby={messageId}
            loading={applying}
            onClick={onApply}
          >
            <Check size={18} strokeWidth={2.2} aria-hidden="true" />
            {state.label}
          </Button>
        </div>
      </div>
    </div>
  );
}
