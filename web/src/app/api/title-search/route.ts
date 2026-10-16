import { NextResponse, type NextRequest } from 'next/server';
import { requireAuth } from '@/lib/api/auth-guard';
import type { ApiErrorBody, TitleSearchResult } from '@/lib/api/contract';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError } from '@/lib/errors';
import { cleanQuery, MAX_QUERY_LENGTH } from '@/lib/title-search/text';

export const dynamic = 'force-dynamic';

const error = (status: number, code: string, message: string) => NextResponse.json<ApiErrorBody>({ error: { code, message } }, { status });

/**
 * GET /api/title-search?q= (#101; web-side, not PLAN §6; contract in
 * lib/api/contract/title-search.ts): TMDB's movies, shows and people for
 * a query, ranked as one list, for the search page and the ⌘K palette.
 */
export async function GET(request: NextRequest): Promise<NextResponse<TitleSearchResult | ApiErrorBody>> {
  const authError = await requireAuth();
  if (authError) return authError;

  const query = cleanQuery(request.nextUrl.searchParams.get('q'));
  if (!query) return error(400, 'bad_request', `q must be 1 to ${MAX_QUERY_LENGTH} characters.`);

  try {
    return NextResponse.json(await tmdbClient.searchTitles(query));
  } catch (cause) {
    if (isAppError(cause)) return error(cause.statusCode, cause.code, cause.message);
    console.error('Title search failed:', cause);
    return error(500, 'internal_error', 'Search failed unexpectedly.');
  }
}
