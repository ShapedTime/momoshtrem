import { useState } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Sheet } from '@/components/ui/Sheet';

// Desktop presentation (Radix Dialog panel); jsdom can't run Vaul's drag physics.
beforeAll(() => {
  window.matchMedia = ((query: string) => ({
    matches: query.includes('min-width: 1024px'),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
});

function Harness({ nested = false }: { nested?: boolean }) {
  const [open, setOpen] = useState(false);
  const [inner, setInner] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      <Sheet open={open} onOpenChange={setOpen} title="Play it in Infuse" footer={<button onClick={() => setInner(true)}>Inner</button>}>
        <p>Body</p>
      </Sheet>
      {nested && (
        <Sheet open={inner} onOpenChange={setInner} nested title="Filters">
          <p>Nested body</p>
        </Sheet>
      )}
    </>
  );
}

describe('Sheet', () => {
  it('opens as a labelled modal dialog with a close button', async () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    const dialog = await screen.findByRole('dialog', { name: 'Play it in Infuse' });
    expect(dialog).toHaveClass('sheet');
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });

  it('closes on Escape and returns focus to the opener', async () => {
    render(<Harness />);
    const opener = screen.getByRole('button', { name: 'Open' });
    opener.focus();
    fireEvent.click(opener);
    const dialog = await screen.findByRole('dialog');
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    // Radix restores focus on the next tick after unmount
    await waitFor(() => expect(opener).toHaveFocus());
  });

  it('closes only the top sheet on Escape', async () => {
    render(<Harness nested />);
    fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Inner' }));
    const top = await screen.findByRole('dialog', { name: 'Filters' });
    fireEvent.keyDown(top, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Filters' })).not.toBeInTheDocument());
    expect(screen.getByRole('dialog', { name: 'Play it in Infuse' })).toBeInTheDocument();
  });
});
