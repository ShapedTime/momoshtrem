import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { Providers } from '@/lib/query/Providers';
import { Gallery } from './Gallery';

export const metadata: Metadata = { title: 'UI gallery' };

/** Dev-only component gallery: every primitive and state from HANDOFF §3. */
export default function DevUiPage() {
  if (process.env.NODE_ENV === 'production') notFound();
  return (
    <Providers>
      <Gallery />
    </Providers>
  );
}
