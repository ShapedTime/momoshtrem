'use client';

import { Ban, Check, CheckCheck, type LucideIcon, Plus, ThumbsDown, ThumbsUp } from 'lucide-react';
import { Sheet } from '@/components/ui';
import type { RecSignal } from '@/lib/api/contract';
import { useAddRecTitle } from './useAddRecTitle';
import type { RecFeedbackControls } from './useRecFeedback';

interface FeedbackOption {
  signal: RecSignal;
  label: string;
  icon: LucideIcon;
  hint?: string;
}

const OPTIONS: FeedbackOption[] = [
  { signal: 'like', label: 'More like this', icon: ThumbsUp },
  { signal: 'dislike', label: 'Less like this', icon: ThumbsDown },
  { signal: 'not_interested', label: 'Not interested', icon: Ban, hint: 'Hides it everywhere' },
  { signal: 'seen', label: 'Already seen', icon: CheckCheck, hint: 'Counts as watched' },
];

export interface FeedbackSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  feedback: RecFeedbackControls;
  /** Already in the library: Add to library is left out. */
  inLibrary: boolean;
}

/** Run once the sheet has closed and handed focus back, so the toast that follows isn't fighting it. */
const later = (action: () => void) => window.setTimeout(action, 0);

/**
 * "…" on a recommendation (HANDOFF §4.4, home.js's feedback sheet): More like
 * this, Less like this, Not interested, Already seen, and Add to library.
 * The signal that's on is pressed, and choosing it again clears it. Shared by
 * For You's shelves and hero and Ask's cards.
 */
export function FeedbackSheet({ open, onOpenChange, feedback, inLibrary }: FeedbackSheetProps) {
  const add = useAddRecTitle();
  const { target } = feedback;
  const choose = (action: () => void) => {
    onOpenChange(false);
    later(action);
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={target.title} description="Tune your picks" size="narrow">
      <div className="menu menu--sheet feedback-menu">
        {OPTIONS.map(({ signal, label, icon: Icon, hint }) => {
          const on = feedback.signal === signal;
          return (
            <button key={signal} type="button" className="menu-item" aria-pressed={on} onClick={() => choose(() => feedback.toggle(signal))}>
              <Icon size={20} aria-hidden="true" />
              <span>{label}</span>
              {on ? <Check size={18} className="hint" aria-hidden="true" /> : hint && <span className="hint">{hint}</span>}
            </button>
          );
        })}
        {!inLibrary && (
          <>
            <div className="sep" aria-hidden="true" />
            <button type="button" className="menu-item" onClick={() => choose(() => add.mutate(target))}>
              <Plus size={20} aria-hidden="true" />
              <span>Add to library</span>
            </button>
          </>
        )}
      </div>
    </Sheet>
  );
}
