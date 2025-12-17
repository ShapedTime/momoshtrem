import { NextRequest, NextResponse } from 'next/server';
import { jackettClient } from '@/lib/api/jackett';
import { isAppError, ValidationError } from '@/lib/errors';
import type { JackettSearchResponse } from '@/types/jackett';
import type { TorznabCategory } from '@/config/jackett';

export async function GET(
  request: NextRequest
): Promise<NextResponse<JackettSearchResponse | { error: string }>> {
  try {
    const { searchParams } = new URL(request.url);
    const query = searchParams.get('q');
    const category = searchParams.get('category') as TorznabCategory | null;

    if (!query || !query.trim()) {
      throw new ValidationError('Search query is required');
    }

    const results = await jackettClient.search(query, category || undefined);

    return NextResponse.json({
      results,
      query: query.trim(),
    });
  } catch (error) {
    console.error('Jackett search error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Torrent search failed' },
      { status: 500 }
    );
  }
}
