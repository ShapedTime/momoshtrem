import type { Metadata } from 'next';
import { Suspense } from 'react';
import { LibraryView } from '@/components/library';
import './library.css';

export const metadata: Metadata = { title: 'Library' };

/** /library: everything you've added, as a grid or a list (PLAN F7, #97). */
export default function LibraryPage() {
  return (
    <Suspense fallback={null}>
      <LibraryView />
    </Suspense>
  );
}
