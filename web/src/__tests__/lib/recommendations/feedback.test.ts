import { dismissToast, feedbackChange, feedbackRequest, isDismissing, recFeedbackKey, shownSignal, toggledSignal } from '@/lib/recommendations/feedback';
import { inOrder, localSignal, resetLocalSignals, setLocalSignal } from '@/lib/recommendations/feedback-store';

/** Recommendation feedback (#155): what dims, what's sent, and what Undo sends back. */

describe('feedback rules', () => {
  it('dislike, not interested and seen dismiss; like and nothing never do', () => {
    expect(isDismissing('dislike')).toBe(true);
    expect(isDismissing('not_interested')).toBe(true);
    expect(isDismissing('seen')).toBe(true);
    expect(isDismissing('like')).toBe(false);
    expect(isDismissing(null)).toBe(false);
    expect(isDismissing(undefined)).toBe(false);
  });

  it('shows what this page set last, else the server’s', () => {
    expect(shownSignal(undefined, 'like')).toBe('like');
    expect(shownSignal(undefined, null)).toBeNull();
    expect(shownSignal('dislike', 'like')).toBe('dislike');
    // Cleared here (a second tap on 👍) beats the server's older like
    expect(shownSignal(null, 'like')).toBeNull();
  });

  it('choosing the signal that’s on clears it; any other replaces it', () => {
    expect(toggledSignal(null, 'like')).toBe('like');
    expect(toggledSignal('like', 'like')).toBeNull();
    expect(toggledSignal('like', 'dislike')).toBe('dislike');
    expect(toggledSignal('dislike', 'seen')).toBe('seen');
  });

  it('a signal is a POST; none is a DELETE', () => {
    expect(feedbackRequest('seen')).toEqual({ method: 'POST', signal: 'seen' });
    expect(feedbackRequest(null)).toEqual({ method: 'DELETE' });
  });

  describe('Undo restores the previous signal', () => {
    it('a first signal: Undo DELETEs', () => {
      expect(feedbackChange(null, 'dislike')).toEqual({
        from: null,
        to: 'dislike',
        request: { method: 'POST', signal: 'dislike' },
        undo: { method: 'DELETE' },
        dismisses: true,
      });
    });

    it('over a like: Undo POSTs the like again', () => {
      const change = feedbackChange('like', 'not_interested');
      expect(change?.request).toEqual({ method: 'POST', signal: 'not_interested' });
      expect(change?.undo).toEqual({ method: 'POST', signal: 'like' });
      expect(change?.dismisses).toBe(true);
    });

    it('over another dismissing signal: Undo brings that one back', () => {
      expect(feedbackChange('dislike', 'seen')?.undo).toEqual({ method: 'POST', signal: 'dislike' });
    });

    it('a like neither dims nor offers Undo; un-liking DELETEs', () => {
      expect(feedbackChange(null, 'like')?.dismisses).toBe(false);
      expect(feedbackChange('like', null)).toMatchObject({ request: { method: 'DELETE' }, undo: { method: 'POST', signal: 'like' }, dismisses: false });
    });

    it('nothing changes, nothing is sent', () => {
      expect(feedbackChange('like', 'like')).toBeNull();
      expect(feedbackChange(null, null)).toBeNull();
    });
  });

  it('the toast says what happened to the title', () => {
    expect(dismissToast('dislike', 'Andor')).toEqual({ title: 'Got it, fewer like this', body: 'Andor won’t be picked again.' });
    expect(dismissToast('not_interested', 'Andor')?.body).toBe('Andor is hidden from your picks and Ask.');
    expect(dismissToast('seen', 'Andor')?.body).toBe('Andor counts as watched.');
    expect(dismissToast('like', 'Andor')).toBeNull();
    expect(dismissToast(null, 'Andor')).toBeNull();
  });
});

describe('feedback store', () => {
  beforeEach(() => resetLocalSignals());

  it('keeps one signal per title, null once cleared', () => {
    const key = recFeedbackKey('tv', 83867);
    expect(key).toBe('tv:83867');
    expect(localSignal(key)).toBeUndefined();
    setLocalSignal(key, 'dislike');
    expect(localSignal(key)).toBe('dislike');
    setLocalSignal(key, null);
    expect(localSignal(key)).toBeNull();
    expect(localSignal(recFeedbackKey('movie', 83867))).toBeUndefined();
  });

  it('sends a title’s requests in order, even after one fails', async () => {
    const log: string[] = [];
    let release: () => void = () => {};
    const slow = new Promise<void>((resolve) => (release = resolve));
    const first = inOrder('tv:1', async () => {
      await slow;
      log.push('dislike');
      throw new Error('offline');
    });
    const undo = inOrder('tv:1', async () => log.push('undo'));
    const other = inOrder('tv:2', async () => log.push('other title'));
    await other;
    expect(log).toEqual(['other title']);
    release();
    await expect(first).rejects.toThrow('offline');
    await undo;
    expect(log).toEqual(['other title', 'dislike', 'undo']);
  });
});
