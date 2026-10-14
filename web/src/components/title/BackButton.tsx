'use client';

import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { IconButton } from '@/components/ui';

/** The hero's phone-bar Back: where you came from, else For You. */
export function BackButton() {
  const router = useRouter();
  return (
    <IconButton variant="glass" aria-label="Back" onClick={() => (window.history.length > 1 ? router.back() : router.push('/'))}>
      <ChevronLeft size={22} aria-hidden="true" />
    </IconButton>
  );
}
