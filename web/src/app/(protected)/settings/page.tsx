import type { Metadata } from 'next';
import { Suspense } from 'react';
import { SettingsView } from '@/components/settings/SettingsView';

export const metadata: Metadata = { title: 'Settings' };

export default function SettingsPage() {
  return (
    <Suspense fallback={null}>
      <SettingsView />
    </Suspense>
  );
}
