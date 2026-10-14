'use client';

import { type FormEvent, useState } from 'react';
import { Hash } from 'lucide-react';
import { Input } from '@/components/ui';
import { type JumpTarget, parseJump } from '@/lib/title/episodes';

interface JumpToEpisodeProps {
  /** How to read the input (parseJump's context). */
  context: Parameters<typeof parseJump>[1];
  onJump: (target: Extract<JumpTarget, { ok: true }>) => void;
}

/** "Jump to episode": 29, 1100 or S02E05; Enter goes there, anything else gets a hint. */
export function JumpToEpisode({ context, onJump }: JumpToEpisodeProps) {
  const [value, setValue] = useState('');
  const [hint, setHint] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const target = parseJump(value, context);
    if (!target.ok) {
      setHint(target.hint);
      return;
    }
    setHint(null);
    onJump(target);
  };

  return (
    <form className="jump" onSubmit={submit} role="search" aria-label="Jump to episode">
      <Input
        inputMode="numeric"
        enterKeyHint="go"
        autoComplete="off"
        placeholder="Jump to episode"
        aria-label="Jump to episode number"
        leadingIcon={<Hash size={16} aria-hidden="true" />}
        value={value}
        error={hint ?? undefined}
        onChange={(event) => {
          setValue(event.target.value);
          if (hint) setHint(null);
        }}
      />
    </form>
  );
}
