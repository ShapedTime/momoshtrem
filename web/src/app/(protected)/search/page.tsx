import type { Metadata } from 'next';
import { Suspense } from 'react';
import { TitleSearchView } from '@/components/title-search';
import './search.css';

export const metadata: Metadata = { title: 'Search' };

/** /search: shows, movies and people on TMDB, your library's matches first (PLAN F6, #101). */
export default function SearchPage() {
  return (
    <Suspense fallback={null}>
      <TitleSearchView />
    </Suspense>
  );
}
