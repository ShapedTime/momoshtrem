'use client';

import { useEffect, useState, useCallback } from 'react';
import { LibraryGrid } from '@/components/library';
import type { LibraryMovie, LibraryShow } from '@/types/momoshtrem';

interface LibraryState {
  movies: LibraryMovie[];
  shows: LibraryShow[];
  isLoading: boolean;
  error: string | null;
}

interface PosterPaths {
  [key: string]: string | null;
}

export default function LibraryPage() {
  const [state, setState] = useState<LibraryState>({
    movies: [],
    shows: [],
    isLoading: true,
    error: null,
  });
  const [posterPaths, setPosterPaths] = useState<PosterPaths>({});

  const fetchLibrary = useCallback(async () => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const [moviesRes, showsRes] = await Promise.all([
        fetch('/api/library/movies'),
        fetch('/api/library/shows'),
      ]);

      if (!moviesRes.ok || !showsRes.ok) {
        throw new Error('Failed to fetch library');
      }

      const [moviesData, showsData] = await Promise.all([
        moviesRes.json(),
        showsRes.json(),
      ]);

      const movies: LibraryMovie[] = moviesData.movies || [];
      const shows: LibraryShow[] = showsData.shows || [];

      setState({
        movies,
        shows,
        isLoading: false,
        error: null,
      });

      // Fetch poster paths from TMDB for each item
      fetchPosterPaths(movies, shows);
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch library',
      }));
    }
  }, []);

  const fetchPosterPaths = async (movies: LibraryMovie[], shows: LibraryShow[]) => {
    const paths: PosterPaths = {};

    // Fetch movie posters
    const moviePromises = movies.map(async (movie) => {
      try {
        const res = await fetch(`/api/tmdb/movie/${movie.tmdb_id}`);
        if (res.ok) {
          const data = await res.json();
          paths[`movie-${movie.tmdb_id}`] = data.posterPath || null;
        }
      } catch {
        // Ignore errors for individual poster fetches
      }
    });

    // Fetch show posters
    const showPromises = shows.map(async (show) => {
      try {
        const res = await fetch(`/api/tmdb/tv/${show.tmdb_id}`);
        if (res.ok) {
          const data = await res.json();
          paths[`tv-${show.tmdb_id}`] = data.posterPath || null;
        }
      } catch {
        // Ignore errors for individual poster fetches
      }
    });

    await Promise.all([...moviePromises, ...showPromises]);
    setPosterPaths(paths);
  };

  useEffect(() => {
    fetchLibrary();
  }, [fetchLibrary]);

  return (
    <main className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8 lg:py-12">
      {/* Page header */}
      <header className="mb-8">
        <h1 className="text-2xl sm:text-3xl font-bold text-white">My Library</h1>
        <p className="mt-2 text-text-secondary">
          Your saved movies and TV shows
        </p>
      </header>

      {/* Error state */}
      {state.error && (
        <div className="text-center py-12">
          <p className="text-accent-red mb-4">{state.error}</p>
          <button
            onClick={fetchLibrary}
            className="px-4 py-2 bg-white/10 hover:bg-white/20 text-white rounded-md transition-colors"
          >
            Try Again
          </button>
        </div>
      )}

      {/* Library grid */}
      {!state.error && (
        <LibraryGrid
          movies={state.movies}
          shows={state.shows}
          isLoading={state.isLoading}
          posterPaths={posterPaths}
          onRefresh={fetchLibrary}
        />
      )}
    </main>
  );
}
