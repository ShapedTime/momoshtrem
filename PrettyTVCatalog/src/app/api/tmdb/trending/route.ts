import { NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { TrendingResults } from '@/types/tmdb';

export async function GET(): Promise<NextResponse<TrendingResults | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const trending = await tmdbClient.getTrending();
    return NextResponse.json(trending);
  } catch (error) {
    console.error('TMDB trending error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch trending content' },
      { status: 500 }
    );
  }
}
