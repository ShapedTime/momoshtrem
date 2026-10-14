'use client';

import { useCallback, useRef } from 'react';

/**
 * Radix only returns focus to a `*.Trigger`. Our overlays are controlled
 * (opened from menus, the palette or the URL), so capture whatever had focus
 * when the overlay opened and return focus there when it closes.
 * Spread the result onto a Radix/Vaul `Content`. With `focusContainer`, the
 * overlay itself takes focus on open instead of its first control.
 */
export function useRestoreFocus({ focusContainer = false }: { focusContainer?: boolean } = {}) {
  const openerRef = useRef<HTMLElement | null>(null);

  const onOpenAutoFocus = useCallback(
    (event: Event) => {
      // Fires before focus moves into the overlay, so this is still the opener
      openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      // Sheets focus themselves rather than their first control (the close button)
      if (focusContainer && event.target instanceof HTMLElement) {
        event.preventDefault();
        event.target.focus();
      }
    },
    [focusContainer]
  );

  const onCloseAutoFocus = useCallback((event: Event) => {
    const opener = openerRef.current;
    openerRef.current = null;
    if (opener && opener.isConnected) {
      event.preventDefault();
      opener.focus();
    }
  }, []);

  return { onOpenAutoFocus, onCloseAutoFocus };
}
