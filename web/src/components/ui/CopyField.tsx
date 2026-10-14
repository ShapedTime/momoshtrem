'use client';

import { type ReactNode, useCallback } from 'react';
import { Copy } from 'lucide-react';
import { cx } from '@/lib/utils/cx';
import { Button } from './Button';
import { useToast } from './Toast';

/**
 * Copy text to the clipboard. The async Clipboard API only exists in secure
 * contexts, and momoshtrem often runs on plain HTTP on the LAN, so fall back
 * to a hidden textarea + execCommand.
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to the legacy path
  }
  const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  area.style.fontSize = '16px'; // iOS zooms into anything smaller
  // In an open sheet, a textarea on <body> loses focus (and the selection) to the sheet's focus trap
  (previous?.closest('[role="dialog"]') ?? document.body).appendChild(area);
  area.select();
  area.setSelectionRange(0, text.length);
  try {
    return document.execCommand('copy');
  } finally {
    area.remove();
    previous?.focus();
  }
}

/** Copy a value and say so in a toast (or say how to copy it by hand). */
export function useCopy() {
  const { toast } = useToast();
  return useCallback(
    async (value: string, hint?: string) => {
      const ok = await copyText(value);
      toast(
        ok
          ? { tone: 'ready', icon: Copy, title: 'Copied', body: hint }
          : { tone: 'error', title: 'Couldn’t copy', body: 'Select the text and copy it by hand.' }
      );
    },
    [toast]
  );
}

interface CopyFieldProps {
  /** What Copy copies. */
  value: string;
  /** Accessible name of the value, e.g. "WebDAV address". */
  label: string;
  /** Toast body after copying. */
  hint?: string;
  /** Shown instead of `value`, e.g. a path with dimmed folders while Copy takes the full URL. */
  children?: ReactNode;
  className?: string;
}

/** A read-only value (address, path) with a Copy button. */
export function CopyField({ value, label, hint, children, className }: CopyFieldProps) {
  const copy = useCopy();
  return (
    <div className={cx('copy-field', className)}>
      <code className="t-mono" aria-label={label}>
        {children ?? value}
      </code>
      <Button size="sm" onClick={() => copy(value, hint)}>
        <Copy size={15} aria-hidden="true" /> Copy
      </Button>
    </div>
  );
}
