'use client';

import { type FormEvent, type KeyboardEvent, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { ArrowUp, Square } from 'lucide-react';
import { IconButton } from '@/components/ui';
import { DESKTOP_QUERY } from '@/hooks/useMediaQuery';
import { ASK_MESSAGE_MAX, askMessageLength } from '@/lib/recommendations/ask-href';
import { cx } from '@/lib/utils/cx';

/** The counter shows once a message gets near the limit. */
const COUNT_FROM = ASK_MESSAGE_MAX - 60;
const MAX_HEIGHT = 160;

interface AskComposerProps {
  placeholder: string;
  /** Asks are paused, or Ask isn't set up: the field explains why in its placeholder. */
  disabled: boolean;
  /** A turn is streaming: Send becomes Stop. */
  working: boolean;
  /** Under the field on desktop: what Noir reads and what an ask costs. */
  note: string;
  /** True when the message was taken; the field keeps it otherwise. */
  onSend: (message: string) => boolean;
  onStop: () => void;
  /** A new conversation on desktop starts with the field focused (a phone keyboard would cover the examples). */
  focusOnDesktop?: boolean;
}

/**
 * The composer (ask.js `ask-composer`): Enter sends, Shift+Enter adds a
 * line, Esc stops a streaming answer. It grows with the text up to 160 px.
 */
export function AskComposer({ placeholder, disabled, working, note, onSend, onStop, focusOnDesktop = false }: AskComposerProps) {
  const [value, setValue] = useState('');
  const field = useRef<HTMLTextAreaElement | null>(null);
  const length = askMessageLength(value.trim());
  const tooLong = length > ASK_MESSAGE_MAX;
  const canSend = !disabled && !working && length > 0 && !tooLong;

  // Grow with the text (and shrink back once it's sent)
  useLayoutEffect(() => {
    const element = field.current;
    if (!element) return;
    element.style.height = 'auto';
    element.style.height = `${Math.min(element.scrollHeight, MAX_HEIGHT)}px`;
  }, [value]);

  useEffect(() => {
    if (focusOnDesktop && window.matchMedia(DESKTOP_QUERY).matches) field.current?.focus({ preventScroll: true });
  }, [focusOnDesktop]);

  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    if (!canSend) return;
    if (onSend(value)) setValue('');
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      submit();
    } else if (event.key === 'Escape' && working) {
      event.preventDefault();
      onStop();
    }
  };

  return (
    <form className="ask-composer" onSubmit={submit} autoComplete="off">
      <div className="ask-composer-in">
        <label className="sr-only" htmlFor="ask-input">
          Ask momoshtrem
        </label>
        <textarea
          id="ask-input"
          ref={field}
          rows={1}
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          enterKeyHint="send"
          disabled={disabled}
          aria-describedby={length >= COUNT_FROM ? 'ask-count' : undefined}
        />
        {length >= COUNT_FROM && (
          <span id="ask-count" className={cx('ask-count', tooLong && 'is-over')}>
            {length}/{ASK_MESSAGE_MAX}
          </span>
        )}
        {working ? (
          <IconButton variant="secondary" className="ask-send" aria-label="Stop" onClick={onStop}>
            <Square size={16} fill="currentColor" aria-hidden="true" />
          </IconButton>
        ) : (
          // Bright while the field is empty, as the design has it; an empty or too long message just doesn't go
          <IconButton type="submit" variant="primary" className="ask-send" aria-label="Send" disabled={disabled}>
            <ArrowUp size={20} strokeWidth={2.2} aria-hidden="true" />
          </IconButton>
        )}
      </div>
      <p className="ask-note t-small text-fg-3">{note}</p>
    </form>
  );
}
