import type { Metadata } from 'next';
import { Suspense } from 'react';
import { ForYouView } from '@/components/for-you';
import './for-you.css';

export const metadata: Metadata = { title: 'For You' };

/** `/`: For You, "What should I watch tonight, and is it ready?" (PLAN F5, #135). */
export default function ForYouPage() {
  // The hero draws its own phone bar over the artwork, account menu included
  return (
    <Suspense fallback={null}>
      <ForYouView />
    </Suspense>
  );
}
