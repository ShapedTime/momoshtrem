'use client';

import { useRef, useState } from 'react';
import { CircleAlert, Plus, X } from 'lucide-react';
import { Input } from '@/components/ui';
import { addTag } from '@/lib/settings/quality';

interface TagFieldProps {
  /** The input is `${id}-input`. */
  id: string;
  /** Names the list and the input ("Add to Terms to avoid"). */
  label: string;
  placeholder: string;
  noun: 'terms' | 'groups';
  values: string[];
  onChange: (values: string[]) => void;
  /** The server's 422 for this list. */
  serverError?: string | null;
}

/** Removable mono tags plus an input where Enter adds one (Settings › Quality). */
export function TagField({ id, label, placeholder, noun, values, onChange, serverError }: TagFieldProps) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const shownError = error ?? serverError ?? null;

  function add() {
    const result = addTag(values, text, noun);
    if (!result.ok) {
      setError(result.error);
      return;
    }
    setText('');
    setError(null);
    if (result.list !== values) onChange(result.list);
  }

  function remove(index: number) {
    onChange(values.filter((_, i) => i !== index));
    // The removed button takes focus with it: move to the tag that took its place, else the input
    requestAnimationFrame(() => {
      const buttons = listRef.current?.querySelectorAll('button');
      const next = buttons && buttons.length > 0 ? buttons[Math.min(index, buttons.length - 1)] : null;
      (next ?? inputRef.current)?.focus();
    });
  }

  return (
    <div className="set-tags">
      {values.length > 0 && (
        <ul ref={listRef} className="chips" aria-label={label}>
          {values.map((value, index) => (
            <li key={`${value}-${index}`} className="set-tag">
              <span>{value}</span>
              <button type="button" aria-label={`Remove ${value}`} onClick={() => remove(index)}>
                <X size={13} aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {/* The error is rendered here, not through Input's `error`: that wraps the input, remounting it and dropping focus */}
      <div className="field max-w-[360px]">
        <Input
          ref={inputRef}
          id={`${id}-input`}
          aria-label={`Add to ${label}`}
          aria-invalid={shownError ? true : undefined}
          aria-describedby={shownError ? `${id}-error` : undefined}
          placeholder={placeholder}
          leadingIcon={<Plus size={16} aria-hidden="true" />}
          enterKeyHint="done"
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            setError(null);
          }}
          onKeyDown={(event) => {
            if (event.key !== 'Enter' || event.nativeEvent.isComposing) return;
            event.preventDefault();
            add();
          }}
        />
        {shownError && (
          // Announce what Enter couldn't add; a server error is announced by the save bar
          <p id={`${id}-error`} role={error ? 'alert' : undefined} className="error-text">
            <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0" />
            <span>{shownError}</span>
          </p>
        )}
      </div>
    </div>
  );
}
