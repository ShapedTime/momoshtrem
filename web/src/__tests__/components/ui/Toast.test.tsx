import { act, fireEvent, render, screen } from '@testing-library/react';
import { ToastProvider, useToast, type ToastOptions } from '@/components/ui/Toast';

function Trigger({ options }: { options: ToastOptions }) {
  const { toast } = useToast();
  return <button onClick={() => toast(options)}>show</button>;
}

function renderWithToast(options: ToastOptions) {
  render(
    <ToastProvider>
      <Trigger options={options} />
    </ToastProvider>
  );
  fireEvent.click(screen.getByRole('button', { name: 'show' }));
}

describe('Toast', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it('shows a success toast and dismisses it after 5 s', () => {
    renderWithToast({ tone: 'ready', title: 'Added to library', body: 'The Good Place' });
    expect(screen.getByRole('status')).toHaveTextContent('Added to library');
    act(() => jest.advanceTimersByTime(5000));
    expect(screen.queryByText('Added to library')).not.toBeInTheDocument();
  });

  it('keeps errors until dismissed and announces them as alerts', () => {
    renderWithToast({ tone: 'error', title: 'Couldn’t reach momoshtrem' });
    act(() => jest.advanceTimersByTime(30_000));
    expect(screen.getByRole('alert')).toHaveTextContent('Couldn’t reach momoshtrem');
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('keeps sticky toasts and runs the single action', () => {
    const onClick = jest.fn();
    renderWithToast({ tone: 'ready', sticky: true, title: 'Ready', action: { label: 'How to play', onClick } });
    act(() => jest.advanceTimersByTime(30_000));
    fireEvent.click(screen.getByRole('button', { name: 'How to play' }));
    expect(onClick).toHaveBeenCalled();
    expect(screen.queryByText('Ready')).not.toBeInTheDocument();
  });

  it('shows one toast at a time: a new one replaces the current', () => {
    function Two() {
      const { toast } = useToast();
      return (
        <>
          <button onClick={() => toast({ title: 'First' })}>first</button>
          <button onClick={() => toast({ title: 'Second' })}>second</button>
        </>
      );
    }
    render(
      <ToastProvider>
        <Two />
      </ToastProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: 'first' }));
    fireEvent.click(screen.getByRole('button', { name: 'second' }));
    expect(screen.queryByText('First')).not.toBeInTheDocument();
    expect(screen.getByText('Second')).toBeInTheDocument();
  });

  it('stays for its own duration', () => {
    renderWithToast({ tone: 'info', title: 'Removed Dune', durationMs: 10_000 });
    act(() => jest.advanceTimersByTime(9_000));
    expect(screen.getByText('Removed Dune')).toBeInTheDocument();
    act(() => jest.advanceTimersByTime(1_000));
    expect(screen.queryByText('Removed Dune')).not.toBeInTheDocument();
  });

  it('runs onClose once, however the toast goes', () => {
    const closes: string[] = [];
    function Many() {
      const { toast } = useToast();
      const show = (title: string, extra: Partial<ToastOptions> = {}) => toast({ title, onClose: () => closes.push(title), ...extra });
      return (
        <>
          <button onClick={() => show('Timed')}>timed</button>
          <button onClick={() => show('Dismissed')}>dismissed</button>
          <button onClick={() => show('Acted', { action: { label: 'Undo', onClick: () => closes.push('undo clicked') } })}>acted</button>
          <button onClick={() => show('Replaced')}>replaced</button>
        </>
      );
    }
    render(
      <ToastProvider>
        <Many />
      </ToastProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: 'timed' }));
    act(() => jest.advanceTimersByTime(5000));
    fireEvent.click(screen.getByRole('button', { name: 'dismissed' }));
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    fireEvent.click(screen.getByRole('button', { name: 'acted' }));
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    fireEvent.click(screen.getByRole('button', { name: 'replaced' }));
    fireEvent.click(screen.getByRole('button', { name: 'timed' }));
    expect(closes).toEqual(['Timed', 'Dismissed', 'undo clicked', 'Acted', 'Replaced']);
  });

  it('throws outside a provider', () => {
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => render(<Trigger options={{ title: 'x' }} />)).toThrow('useToast must be used within a ToastProvider');
    spy.mockRestore();
  });
});
