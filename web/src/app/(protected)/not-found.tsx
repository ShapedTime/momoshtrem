import type { Metadata } from 'next';
import { NotFoundView } from '@/components/system/NotFoundView';

export const metadata: Metadata = { title: 'Not found' };

export default function ProtectedNotFound() {
  return <NotFoundView />;
}
