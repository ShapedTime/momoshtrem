import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { ToastProvider } from '@/components/ui';
import { Gallery } from './Gallery';

export const metadata: Metadata = { title: 'UI gallery' };

/** Dev-only component gallery: every primitive and state from HANDOFF §3. */
export default function DevUiPage() {
  if (process.env.NODE_ENV === 'production') notFound();
  return (
    <ToastProvider>
      <Gallery />
    </ToastProvider>
  );
}
