import type { Metadata } from 'next';
import { Suspense } from 'react';
import { DiscoverView } from '@/components/discover';
import './discover.css';

export const metadata: Metadata = { title: 'Discover' };

/** /discover: everything on TMDB, filtered your way, with the filters in the URL (PLAN F6, #100). */
export default function DiscoverPage() {
  return (
    <Suspense fallback={null}>
      <DiscoverView />
    </Suspense>
  );
}
