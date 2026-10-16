'use client';

import { useState } from 'react';
import { Ellipsis, ThumbsDown, ThumbsUp } from 'lucide-react';
import { IconButton } from '@/components/ui';
import { cx } from '@/lib/utils/cx';
import { FeedbackSheet } from './FeedbackSheet';
import type { RecFeedbackControls } from './useRecFeedback';

export interface FeedbackRowProps {
  /** From `useRecFeedback(item, source)`. */
  feedback: RecFeedbackControls;
  /** Already in the library: the sheet leaves out Add to library. */
  inLibrary: boolean;
  /** `.rec-feedback` is the three buttons; PosterCard's `footer` slot places it under the meta. Add a class to place it elsewhere (an Ask card). */
  className?: string;
}

/**
 * A recommendation's feedback (HANDOFF §4.4): 👍 and 👎 inline, both toggles
 * with `aria-pressed`, and "…" for the sheet (Not interested, Already seen,
 * Add to library). Visible without hover. 👎 dims the card through
 * `feedback.dismissed`; 👍 never does.
 */
export function FeedbackRow({ feedback, inLibrary, className }: FeedbackRowProps) {
  const [open, setOpen] = useState(false);
  // The sheet mounts on first use: a shelf of cards doesn't carry a closed sheet each
  const [opened, setOpened] = useState(false);
  const { title } = feedback.target;

  return (
    <div className={cx('rec-feedback', className)} role="group" aria-label={`Feedback on ${title}`}>
      <IconButton size="sm" aria-label="More like this" aria-pressed={feedback.liked} onClick={() => feedback.toggle('like')}>
        <ThumbsUp size={16} aria-hidden="true" />
      </IconButton>
      <IconButton size="sm" aria-label="Less like this" aria-pressed={feedback.disliked} onClick={() => feedback.toggle('dislike')}>
        <ThumbsDown size={16} aria-hidden="true" />
      </IconButton>
      <IconButton
        size="sm"
        aria-label="More feedback"
        aria-haspopup="dialog"
        onClick={() => {
          setOpened(true);
          setOpen(true);
        }}
      >
        <Ellipsis size={16} aria-hidden="true" />
      </IconButton>
      {opened && <FeedbackSheet open={open} onOpenChange={setOpen} feedback={feedback} inLibrary={inLibrary} />}
    </div>
  );
}
