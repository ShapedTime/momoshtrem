import { NextResponse, type NextRequest } from 'next/server';
import { requireAuth } from '@/lib/api/auth-guard';
import type { ApiErrorBody, DiscoverPage, DiscoverRelaxed } from '@/lib/api/contract';
import { tmdbClient } from '@/lib/api/tmdb';
import { MAX_TMDB_PAGE } from '@/lib/discover/tmdb-query';
import { parseDiscoverSearch } from '@/lib/discover/url';
import { isAppError } from '@/lib/errors';

export const dynamic = 'force-dynamic';

const error = (status: number, code: string, message: string) => NextResponse.json<ApiErrorBody>({ error: { code, message } }, { status });

/**
 * GET /api/discover (#100; web-side, not PLAN §6; contract in
 * lib/api/contract/discover.ts): one page of TMDB discover for the Discover
 * page's filters, or with `relax=1` the total without each active filter.
 */
export async function GET(request: NextRequest): Promise<NextResponse<DiscoverPage | DiscoverRelaxed | ApiErrorBody>> {
  const authError = await requireAuth();
  if (authError) return authError;

  const search = request.nextUrl.searchParams;
  const pageParam = search.get('page') ?? '1';
  const page = /^\d+$/.test(pageParam) ? Number(pageParam) : NaN;
  if (!(page >= 1 && page <= MAX_TMDB_PAGE)) return error(400, 'bad_request', `page must be a whole number from 1 to ${MAX_TMDB_PAGE}.`);

  const filters = parseDiscoverSearch(search);
  try {
    if (search.get('relax') === '1') return NextResponse.json(await tmdbClient.discoverRelaxed(filters));
    return NextResponse.json(await tmdbClient.discover(filters, page));
  } catch (cause) {
    if (isAppError(cause)) return error(cause.statusCode, cause.code, cause.message);
    console.error('Discover failed:', cause);
    return error(500, 'internal_error', 'Discover failed unexpectedly.');
  }
}
