import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { SearchResult } from '@/types/tmdb';

export async function GET(
  request: NextRequest
): Promise<NextResponse<SearchResult[] | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { searchParams } = new URL(request.url);
    const query = searchParams.get('q');

    if (!query || !query.trim()) {
      throw new ValidationError('Search query is required');
    }

    const results = await tmdbClient.search(query);
    return NextResponse.json(results);
  } catch (error) {
    console.error('TMDB search error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json({ error: 'Search failed' }, { status: 500 });
  }
}
