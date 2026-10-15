import { render, screen } from '@testing-library/react';
import { NeedsReviewBanner } from '@/components/title/show';

/** The way back to an open mapping review from a title page (#5). */
describe('NeedsReviewBanner', () => {
  it('a show: how many files need a look, and the review', () => {
    render(<NeedsReviewBanner count={2} jobId={101} />);
    expect(screen.getByText('2 files need a quick review')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/101');
  });

  it('a movie: one file to pick, and the review', () => {
    render(<NeedsReviewBanner movie count={2} jobId={301} />);
    expect(screen.getByText('Pick the file to use')).toBeInTheDocument();
    expect(screen.getByText(/More than one file in the torrent could be the movie/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/301');
  });

  it('nothing when nothing waits, or a movie has no job to go to', () => {
    const { container, rerender } = render(<NeedsReviewBanner count={0} jobId={101} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<NeedsReviewBanner movie count={2} jobId={null} />);
    expect(container).toBeEmptyDOMElement();
  });
});
