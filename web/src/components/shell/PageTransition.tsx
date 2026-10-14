'use client';

import * as React from 'react';

/**
 * Crossfades page navigations with React's <ViewTransition> (Next's
 * `experimental.viewTransition`). Only the canary React that Next vendors
 * exports it, so fall back to a Fragment elsewhere (e.g. Jest). Reduced
 * motion disables the animation in CSS.
 */
const ViewTransition: React.ComponentType<{ children: React.ReactNode }> =
  (React as unknown as { ViewTransition?: React.ComponentType<{ children: React.ReactNode }> }).ViewTransition ?? React.Fragment;

export function PageTransition({ children }: { children: React.ReactNode }) {
  return <ViewTransition>{children}</ViewTransition>;
}
