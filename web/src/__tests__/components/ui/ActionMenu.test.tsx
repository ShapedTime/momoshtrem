import { act, fireEvent, render, screen } from '@testing-library/react';
import { Trash2 } from 'lucide-react';
import { ActionMenu, IconButton } from '@/components/ui';

function setMedia(desktop: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: desktop,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}

function renderMenu(onRemove: () => void) {
  render(
    <ActionMenu
      label="More actions"
      sheetTitle="Severance"
      sheetDescription="18/18 ready · 25.2 GB"
      items={[
        { key: 'open', label: 'Open details', href: '/tv/95396' },
        { key: 'remove', label: 'Remove from library…', icon: Trash2, danger: true, separator: true, onSelect: onRemove },
      ]}
    >
      <IconButton aria-label="Severance: more actions">…</IconButton>
    </ActionMenu>
  );
}

describe('ActionMenu', () => {
  const original = window.matchMedia;
  afterEach(() => {
    window.matchMedia = original;
    jest.useRealTimers();
  });

  it('is a sheet on phone, and runs an item once the sheet has closed', () => {
    jest.useFakeTimers();
    setMedia(false);
    const onRemove = jest.fn();
    renderMenu(onRemove);
    const trigger = screen.getByRole('button', { name: 'Severance: more actions' });
    expect(trigger).toHaveAttribute('aria-haspopup', 'dialog');
    fireEvent.click(trigger);
    const sheet = screen.getByRole('dialog', { name: 'Severance' });
    expect(sheet).toHaveTextContent('18/18 ready · 25.2 GB');
    expect(screen.getByRole('link', { name: 'Open details' })).toHaveAttribute('href', '/tv/95396');
    const remove = screen.getByRole('button', { name: 'Remove from library…' });
    expect(remove).toHaveClass('menu-item--quiet-danger');
    fireEvent.click(remove);
    expect(onRemove).not.toHaveBeenCalled();
    act(() => jest.runAllTimers());
    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it('is a menu on desktop', () => {
    setMedia(true);
    renderMenu(jest.fn());
    const trigger = screen.getByRole('button', { name: 'Severance: more actions' });
    expect(trigger).toHaveAttribute('aria-haspopup', 'menu');
  });
});
