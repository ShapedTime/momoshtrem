import type { Metadata } from 'next';
import { Suspense } from 'react';
import { ActivityView } from '@/components/activity';
import './activity.css';

export const metadata: Metadata = { title: 'Activity' };

/** /activity: is everything working? What streams, what downloads, and the torrents behind the library (PLAN F7, #102). */
export default function ActivityPage() {
  return (
    <Suspense fallback={null}>
      <ActivityView />
    </Suspense>
  );
}
