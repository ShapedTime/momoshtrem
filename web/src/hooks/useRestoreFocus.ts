'use client';

import { useCallback, useRef } from 'react';

/**
 * Radix only returns focus to a `*.Trigger`. Our overlays are controlled
 * (opened from menus, the palette or the URL), so capture whatever had focus
 * when the overlay opened and return focus there when it closes.
 * Spread the result onto a Radix/Vaul `Content`.
 */
export function useRestoreFocus() {
  const openerRef = useRef<HTMLElement | null>(null);

  const onOpenAutoFocus = useCallback(() => {
    // Fires before focus moves into the overlay, so this is still the opener
    openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  }, []);

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
