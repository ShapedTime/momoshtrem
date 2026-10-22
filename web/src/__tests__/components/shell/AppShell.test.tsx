import type { ReactNode } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { AppShell } from '@/components/shell/AppShell';

// Only the keyboard handler is under test: stub the chrome around it
jest.mock('@/components/shell/TopBar', () => ({ TopBar: () => null }));
jest.mock('@/components/shell/PhoneBar', () => ({ PhoneBar: () => null }));
jest.mock('@/components/shell/TabBar', () => ({ TabBar: () => null }));
jest.mock('@/components/shell/SheetHost', () => ({ SheetHost: () => null }));
jest.mock('@/components/shell/useActiveTorrentCount', () => ({ useActiveTorrentCount: () => 0 }));
jest.mock('@/components/system/ConnectionBanner', () => ({ ConnectionBanner: () => null }));
jest.mock('@/components/shell/CommandPalette', () => ({
  CommandPalette: ({ open }: { open: boolean }) => (open ? <div role="dialog" aria-label="Search or jump to" /> : null),
}));

const palette = () => screen.queryByRole('dialog', { name: 'Search or jump to' });

function renderShell(children: ReactNode = null) {
  render(<AppShell>{children}</AppShell>);
}

describe('AppShell shortcuts', () => {
  it('⌘K toggles the palette and "/" opens it', () => {
    renderShell();
    fireEvent.keyDown(window, { key: 'k', metaKey: true });
    expect(palette()).toBeInTheDocument();
    fireEvent.keyDown(window, { key: 'k', metaKey: true });
    expect(palette()).not.toBeInTheDocument();
    fireEvent.keyDown(document.body, { key: '/' });
    expect(palette()).toBeInTheDocument();
  });

  it('leaves "/" alone while typing', () => {
    renderShell(<input aria-label="Filter" />);
    fireEvent.keyDown(screen.getByRole('textbox', { name: 'Filter' }), { key: '/' });
    expect(palette()).not.toBeInTheDocument();
  });

  it('does nothing while a confirm dialog is open', () => {
    renderShell(<div role="alertdialog" aria-label="Remove Severance from your library?" />);
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    fireEvent.keyDown(document.body, { key: '/' });
    expect(palette()).not.toBeInTheDocument();
  });
});
