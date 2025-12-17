import { render, screen } from '@testing-library/react';
import { MediaCard, MediaCardSkeleton } from '@/components/media/MediaCard';
import type { MovieSearchResult, TVSearchResult } from '@/types/tmdb';

const mockMovie: MovieSearchResult = {
  id: 123,
  mediaType: 'movie',
  title: 'Test Movie',
  overview: 'A great test movie',
  posterPath: '/test-poster.jpg',
  backdropPath: '/test-backdrop.jpg',
  releaseDate: '2023-06-15',
  voteAverage: 8.5,
};

const mockTVShow: TVSearchResult = {
  id: 456,
  mediaType: 'tv',
  name: 'Test TV Show',
  overview: 'A great test show',
  posterPath: '/test-tv-poster.jpg',
  backdropPath: '/test-tv-backdrop.jpg',
  firstAirDate: '2022-03-20',
  voteAverage: 9.0,
};

const mockMovieNoPoster: MovieSearchResult = {
  ...mockMovie,
  id: 789,
  posterPath: null,
  voteAverage: 0,
};

describe('MediaCard', () => {
  describe('Movie rendering', () => {
    it('renders movie title', () => {
      render(<MediaCard media={mockMovie} />);
      expect(screen.getByText('Test Movie')).toBeInTheDocument();
    });

    it('links to correct movie page', () => {
      render(<MediaCard media={mockMovie} />);
      const link = screen.getByRole('link');
      expect(link).toHaveAttribute('href', '/movie/123');
    });

    it('displays release year', () => {
      render(<MediaCard media={mockMovie} />);
      expect(screen.getByText('2023')).toBeInTheDocument();
    });

    it('displays rating badge', () => {
      render(<MediaCard media={mockMovie} />);
      expect(screen.getByText('8.5')).toBeInTheDocument();
    });
  });

  describe('TV Show rendering', () => {
    it('renders TV show name', () => {
      render(<MediaCard media={mockTVShow} />);
      expect(screen.getByText('Test TV Show')).toBeInTheDocument();
    });

    it('links to correct TV show page', () => {
      render(<MediaCard media={mockTVShow} />);
      const link = screen.getByRole('link');
      expect(link).toHaveAttribute('href', '/tv/456');
    });

    it('displays first air year', () => {
      render(<MediaCard media={mockTVShow} />);
      expect(screen.getByText('2022')).toBeInTheDocument();
    });
  });

  describe('Poster handling', () => {
    it('renders poster image when available', () => {
      render(<MediaCard media={mockMovie} />);
      const image = screen.getByRole('img', { name: 'Test Movie' });
      expect(image).toBeInTheDocument();
    });

    it('renders placeholder icon when no poster', () => {
      render(<MediaCard media={mockMovieNoPoster} />);
      // FilmIcon is rendered as SVG with aria-hidden
      const placeholder = document.querySelector('svg[aria-hidden="true"]');
      expect(placeholder).toBeInTheDocument();
    });
  });

  describe('Rating badge', () => {
    it('shows rating when > 0', () => {
      render(<MediaCard media={mockMovie} />);
      expect(screen.getByText('8.5')).toBeInTheDocument();
    });

    it('hides rating badge when 0', () => {
      render(<MediaCard media={mockMovieNoPoster} />);
      // Should not find any rating text
      expect(screen.queryByText('0.0')).not.toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    it('has accessible link with focus styles', () => {
      render(<MediaCard media={mockMovie} />);
      const link = screen.getByRole('link');
      expect(link).toHaveClass('focus-visible:ring-2');
    });

    it('image has alt text', () => {
      render(<MediaCard media={mockMovie} />);
      const image = screen.getByRole('img');
      expect(image).toHaveAttribute('alt', 'Test Movie');
    });

    it('respects motion-reduce preference', () => {
      render(<MediaCard media={mockMovie} />);
      // The poster container should have motion-reduce class
      const container = document.querySelector('.motion-reduce\\:transition-none');
      expect(container).toBeInTheDocument();
    });
  });

  describe('Priority loading', () => {
    it('sets priority on image when specified', () => {
      render(<MediaCard media={mockMovie} priority />);
      const image = screen.getByRole('img');
      // Next.js Image with priority sets fetchPriority="high"
      expect(image).toHaveAttribute('fetchPriority', 'high');
    });
  });
});

describe('MediaCardSkeleton', () => {
  it('renders skeleton placeholder', () => {
    render(<MediaCardSkeleton />);
    const skeleton = document.querySelector('.animate-pulse');
    expect(skeleton).toBeInTheDocument();
  });

  it('has correct aspect ratio container', () => {
    render(<MediaCardSkeleton />);
    const container = document.querySelector('.aspect-\\[2\\/3\\]');
    expect(container).toBeInTheDocument();
  });

  it('respects motion-reduce preference', () => {
    render(<MediaCardSkeleton />);
    const skeleton = document.querySelector('.motion-reduce\\:animate-none');
    expect(skeleton).toBeInTheDocument();
  });
});
