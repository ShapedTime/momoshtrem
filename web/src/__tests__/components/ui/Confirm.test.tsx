import { useState } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ConfirmProvider, useConfirm } from '@/components/ui';

function Asker({ checkbox }: { checkbox: boolean }) {
  const confirm = useConfirm();
  const [answer, setAnswer] = useState('');
  return (
    <>
      <button
        onClick={async () => {
          const base = { title: 'Remove Dune from your library?', description: 'This removes the movie.', confirmLabel: 'Remove' };
          if (checkbox) {
            const result = await confirm({ ...base, checkbox: { label: 'Also delete downloaded data', defaultChecked: true } });
            setAnswer(JSON.stringify(result));
          } else {
            setAnswer(String(await confirm(base)));
          }
        }}
      >
        ask
      </button>
      <output>{answer}</output>
    </>
  );
}

const renderAsker = (checkbox: boolean) =>
  render(
    <ConfirmProvider>
      <Asker checkbox={checkbox} />
    </ConfirmProvider>
  );

describe('useConfirm', () => {
  it('resolves a boolean without a checkbox', async () => {
    renderAsker(false);
    fireEvent.click(screen.getByRole('button', { name: 'ask' }));
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('true'));
  });

  it('with a checkbox, resolves how it was left', async () => {
    renderAsker(true);
    fireEvent.click(screen.getByRole('button', { name: 'ask' }));
    const box = screen.getByRole('checkbox', { name: 'Also delete downloaded data' });
    expect(box).toBeChecked();
    fireEvent.click(box);
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('{"confirmed":true,"checked":false}'));
  });

  it('cancelling says so, with the checkbox state', async () => {
    renderAsker(true);
    fireEvent.click(screen.getByRole('button', { name: 'ask' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('{"confirmed":false,"checked":true}'));
  });
});
