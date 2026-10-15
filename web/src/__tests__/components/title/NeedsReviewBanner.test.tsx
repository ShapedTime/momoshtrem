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

  // review_job_id stays set while a job waits for Apply (or its AI check) with nothing to decide (PLAN §6)
  it('a show: a job with nothing to decide still links to its review', () => {
    render(<NeedsReviewBanner count={0} jobId={101} />);
    expect(screen.getByText('Finish this torrent’s mapping')).toBeInTheDocument();
    expect(screen.getByText('Open it to check and apply. Infuse shows the episodes once it’s applied.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/101');
    expect(screen.queryByText(/quick review/)).not.toBeInTheDocument();
  });

  it('a movie: a job with nothing to decide still links to its review', () => {
    render(<NeedsReviewBanner movie count={0} jobId={301} />);
    expect(screen.getByText('Finish this torrent’s mapping')).toBeInTheDocument();
    expect(screen.getByText('Open it to check and apply. Infuse shows the movie once it’s applied.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review mapping' })).toHaveAttribute('href', '/mapping/301');
    expect(screen.queryByText('Pick the file to use')).not.toBeInTheDocument();
  });

  it('nothing when nothing waits, or a movie has no job to go to', () => {
    const { container, rerender } = render(<NeedsReviewBanner count={0} jobId={null} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<NeedsReviewBanner movie count={0} jobId={null} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<NeedsReviewBanner movie count={2} jobId={null} />);
    expect(container).toBeEmptyDOMElement();
  });
});
