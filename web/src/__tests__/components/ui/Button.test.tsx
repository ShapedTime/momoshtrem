import { fireEvent, render, screen } from '@testing-library/react';
import { Button, ButtonLink, IconButton } from '@/components/ui/Button';

describe('Button', () => {
  it('renders children and handles clicks', () => {
    const onClick = jest.fn();
    render(<Button onClick={onClick}>Find a torrent</Button>);
    fireEvent.click(screen.getByRole('button', { name: 'Find a torrent' }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('defaults to a secondary, medium, non-submitting button', () => {
    render(<Button>Subtitles</Button>);
    const button = screen.getByRole('button');
    expect(button).toHaveClass('btn', 'btn-secondary');
    expect(button).not.toHaveClass('btn-sm', 'btn-lg', 'btn-block');
    expect(button).toHaveAttribute('type', 'button');
  });

  it.each([
    ['primary', 'btn-primary'],
    ['ghost', 'btn-ghost'],
    ['glass', 'btn-glass'],
    ['ready', 'btn-ready'],
    ['danger', 'btn-danger'],
    ['danger-quiet', 'btn-danger-quiet'],
  ] as const)('applies the %s variant', (variant, className) => {
    render(<Button variant={variant}>x</Button>);
    expect(screen.getByRole('button')).toHaveClass(className);
  });

  it('applies sizes and block width only when asked', () => {
    const { rerender } = render(<Button size="sm">x</Button>);
    expect(screen.getByRole('button')).toHaveClass('btn-sm');
    rerender(
      <Button size="lg" block>
        x
      </Button>
    );
    expect(screen.getByRole('button')).toHaveClass('btn-lg', 'btn-block');
  });

  it('is busy but keeps its label while loading, and ignores clicks', () => {
    const onClick = jest.fn();
    render(
      <Button loading onClick={onClick}>
        Adding
      </Button>
    );
    const button = screen.getByRole('button', { name: 'Adding' });
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toHaveClass('is-loading');
    fireEvent.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('still accepts the deprecated isLoading prop', () => {
    render(<Button isLoading>Saving</Button>);
    expect(screen.getByRole('button')).toHaveAttribute('aria-busy', 'true');
  });

  it('can be disabled', () => {
    render(<Button disabled>x</Button>);
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('submits forms when typed as submit', () => {
    const onSubmit = jest.fn((event: React.FormEvent) => event.preventDefault());
    render(
      <form onSubmit={onSubmit}>
        <Button type="submit">Sign in</Button>
      </form>
    );
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(onSubmit).toHaveBeenCalled();
  });
});

describe('IconButton', () => {
  it('is square, ghost by default and named by aria-label', () => {
    render(
      <IconButton aria-label="Close">
        <svg />
      </IconButton>
    );
    const button = screen.getByRole('button', { name: 'Close' });
    expect(button).toHaveClass('btn-icon', 'btn-ghost');
  });
});

describe('ButtonLink', () => {
  it('renders a link styled as a button', () => {
    render(
      <ButtonLink href="/library" variant="primary">
        Browse
      </ButtonLink>
    );
    const link = screen.getByRole('link', { name: 'Browse' });
    expect(link).toHaveAttribute('href', '/library');
    expect(link).toHaveClass('btn', 'btn-primary');
  });
});
