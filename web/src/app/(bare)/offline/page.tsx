import type { Metadata } from 'next';
import { Suspense } from 'react';
import { OfflineView } from '@/components/system/OfflineView';

export const metadata: Metadata = { title: 'Not responding' };

export default function OfflinePage() {
  return (
    <Suspense fallback={null}>
      <OfflineView />
    </Suspense>
  );
}
