import Image from 'next/image';
import { TMDB_BASE_URL, TMDB_IMAGE_BASE_URL } from '@/config/tmdb';

const COLUMNS = 7;
const PER_COLUMN = 5;

/** This week's trending posters, cached for a day. Empty when TMDB is unreachable. */
async function trendingPosters(): Promise<string[]> {
  const apiKey = process.env.TMDB_API_KEY;
  if (!apiKey) return [];
  try {
    const response = await fetch(`${TMDB_BASE_URL}/trending/all/week?api_key=${encodeURIComponent(apiKey)}`, {
      next: { revalidate: 86_400 },
      signal: AbortSignal.timeout(3000),
    });
    if (!response.ok) return [];
    const data = (await response.json()) as { results?: Array<{ poster_path?: string | null }> };
    return (data.results ?? [])
      .map((item) => item.poster_path)
      .filter((path): path is string => Boolean(path))
      .map((path) => `${TMDB_IMAGE_BASE_URL}/w185${path}`);
  } catch {
    return [];
  }
}

/**
 * Dim, tilted, slowly drifting wall of posters behind the sign-in card
 * (decorative). Rendered in its own Suspense boundary so the form never
 * waits for TMDB.
 */
export async function PosterWall() {
  const posters = await trendingPosters();
  if (posters.length === 0) return null;
  const columns = Array.from({ length: COLUMNS }, (_, column) =>
    Array.from({ length: PER_COLUMN }, (_, row) => posters[(column * PER_COLUMN + row) % posters.length])
  );
  return (
    <div className="auth-wall" aria-hidden="true">
      <div className="auth-wall-in">
        {columns.map((column, i) => (
          <div key={i} className={`auth-col${i % 2 ? ' is-down' : ''}`}>
            {column.map((src, j) => (
              <div key={j} className="media media--poster">
                <Image src={src} alt="" fill sizes="185px" unoptimized loading="eager" />
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
