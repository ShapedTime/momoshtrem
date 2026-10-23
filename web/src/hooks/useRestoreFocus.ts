'use client';

import { useCallback, useRef } from 'react';

/**
 * Radix only returns focus to a `*.Trigger`. Our overlays are controlled
 * (opened from menus, the palette or the URL), so capture whatever had focus
 * when the overlay opened and return focus there when it closes.
 * Spread the result onto a Radix/Vaul `Content`. With `focusContainer`, the
 * overlay itself takes focus on open instead of its first control. When the
 * opener is gone or disabled by then (a removed row's Edit), `fallback` names
 * where focus lands instead.
 */
export function useRestoreFocus({ focusContainer = false, fallback }: { focusContainer?: boolean; fallback?: () => HTMLElement | null } = {}) {
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
    // Another overlay took over as this one closed (the phone's account sheet opening
    // the ⌘K palette): leave focus in it, or typing would land behind it
    const active = document.activeElement;
    const closing = event.target instanceof Node ? event.target : null;
    if (active instanceof HTMLElement && active.closest('[role="dialog"], [role="alertdialog"]') && !closing?.contains(active)) {
      event.preventDefault();
      return;
    }
    // The opener, unless nothing had focus (the body), it's gone, or it's disabled now
    if (opener && opener !== document.body && opener.isConnected && !opener.matches(':disabled')) {
      event.preventDefault();
      opener.focus();
      return;
    }
    const landing = fallback?.();
    if (landing) {
      event.preventDefault();
      landing.focus();
    }
  }, [fallback]);

  return { onOpenAutoFocus, onCloseAutoFocus };
}
