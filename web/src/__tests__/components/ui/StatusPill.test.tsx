import { render, screen } from '@testing-library/react';
import { StatusPill, Progress, Steps } from '@/components/ui/Status';
import { STATUS, isStatusKey } from '@/lib/status';

describe('status vocabulary', () => {
  it('uses one word per state across surfaces', () => {
    expect(STATUS.ready.label).toBe('Ready');
    expect(STATUS.needs_torrent.label).toBe('Needs a torrent');
    expect(STATUS.needs_review.label).toBe('Needs review');
    expect(isStatusKey('ready_infuse')).toBe(true);
    expect(isStatusKey('In Library')).toBe(false);
  });
});

describe('StatusPill', () => {
  it('pairs the tone with an icon and a word (never colour alone)', () => {
    const { container } = render(<StatusPill status="downloading" />);
    const pill = container.querySelector('.pill');
    expect(pill).toHaveClass('pill--downloading');
    expect(pill?.querySelector('svg')).not.toBeNull();
    expect(screen.getByText('Downloading')).toBeInTheDocument();
  });

  it('accepts a short label and a progress ring', () => {
    const { container } = render(<StatusPill status="downloading" label="64%" pct={64} onArt />);
    expect(screen.getByText('64%')).toBeInTheDocument();
    expect(container.querySelector('.mini-ring')).not.toBeNull();
    expect(container.querySelector('.pill')).toHaveClass('pill--on-art');
  });
});

describe('Progress', () => {
  it('exposes value and name to assistive tech', () => {
    render(<Progress label="Frieren download" value={64.4} />);
    const bar = screen.getByRole('progressbar', { name: 'Frieren download' });
    expect(bar).toHaveAttribute('aria-valuenow', '64');
  });

  it('omits the value when indeterminate', () => {
    render(<Progress label="Searching" indeterminate />);
    expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow');
  });
});

describe('Steps', () => {
  it('marks the active step', () => {
    render(
      <Steps
        steps={[
          { label: 'Fetching metadata', state: 'done' },
          { label: 'Matching files', state: 'active' },
          { label: 'Ready to review', state: 'pending' },
        ]}
      />
    );
    expect(screen.getByText('Matching files').closest('li')).toHaveAttribute('aria-current', 'step');
  });
});
