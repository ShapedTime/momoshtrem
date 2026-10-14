import { render, screen } from '@testing-library/react';
import { RatingBadge } from '@/components/ui/RatingBadge';

describe('RatingBadge', () => {
  describe('card variant (default)', () => {
    it('renders rating with one decimal place', () => {
      render(<RatingBadge rating={8.567} />);
      expect(screen.getByText('8.6')).toBeInTheDocument();
    });

    it('has absolute positioning for overlay', () => {
      const { container } = render(<RatingBadge rating={7.5} />);
      const badge = container.firstChild;
      expect(badge).toHaveClass('absolute');
    });

    it('has backdrop blur styling', () => {
      const { container } = render(<RatingBadge rating={7.5} />);
      const badge = container.firstChild;
      expect(badge).toHaveClass('backdrop-blur-sm');
    });
  });

  describe('hero variant', () => {
    it('renders rating with star icon', () => {
      render(<RatingBadge rating={9.0} variant="hero" />);
      expect(screen.getByText('9.0')).toBeInTheDocument();
      // Star icon should be present
      const starIcon = document.querySelector('svg');
      expect(starIcon).toBeInTheDocument();
    });

    it('uses inline flex layout', () => {
      const { container } = render(<RatingBadge rating={8.0} variant="hero" />);
      const badge = container.firstChild;
      expect(badge).toHaveClass('flex', 'items-center');
    });

    it('does not have absolute positioning', () => {
      const { container } = render(<RatingBadge rating={8.0} variant="hero" />);
      const badge = container.firstChild;
      expect(badge).not.toHaveClass('absolute');
    });
  });

  describe('formatting', () => {
    it('formats whole numbers with decimal', () => {
      render(<RatingBadge rating={10} />);
      expect(screen.getByText('10.0')).toBeInTheDocument();
    });

    it('formats zero', () => {
      render(<RatingBadge rating={0} />);
      expect(screen.getByText('0.0')).toBeInTheDocument();
    });

    it('rounds to one decimal place', () => {
      render(<RatingBadge rating={7.777} />);
      expect(screen.getByText('7.8')).toBeInTheDocument();
    });
  });
});
