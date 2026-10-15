import { fireEvent, render, screen } from '@testing-library/react';
import { SheetHost } from '@/components/shell/SheetHost';
import { getSheet, SHEETS, type SheetComponentProps } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';

const mockRouter = { push: jest.fn(), replace: jest.fn(), back: jest.fn() };
let mockSearch = '';

jest.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
  usePathname: () => '/library',
  useSearchParams: () => new URLSearchParams(mockSearch),
}));

afterEach(() => {
  mockSearch = '';
  jest.clearAllMocks();
});

describe('getSheet', () => {
  it('finds registered sheets', () => {
    expect(getSheet('infuse-setup')?.title).toBe('Play it in Infuse');
  });

  it.each(['toString', 'constructor', '__proto__', 'hasOwnProperty', 'nope'])('ignores %s', (name) => {
    expect(getSheet(name)).toBeUndefined();
  });

  it('ignores a missing name', () => {
    expect(getSheet(null)).toBeUndefined();
  });
});

describe('SheetHost', () => {
  it('renders nothing for an Object.prototype name', () => {
    mockSearch = 'sheet=toString';
    const { container } = render(<SheetHost />);
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('lets a self-rendering sheet draw itself and close with its params', () => {
    function EpisodeProbe({ params, close }: SheetComponentProps) {
      return (
        <button type="button" onClick={close}>
          Episode {params.get('episode')}
        </button>
      );
    }
    SHEETS['probe'] = { params: ['episode'], Component: EpisodeProbe };
    try {
      mockSearch = 'sheet=probe&episode=29';
      render(<SheetHost />);
      fireEvent.click(screen.getByRole('button', { name: 'Episode 29' }));
      expect(mockRouter.replace).toHaveBeenCalledWith('/library', { scroll: false });
    } finally {
      delete SHEETS['probe'];
    }
  });
});

function Probe() {
  const sheets = useSheet();
  return (
    <>
      <button onClick={() => sheets.open('infuse-setup')}>open</button>
      <button onClick={() => sheets.open('torrent', { movie: '157336' })}>open torrents</button>
      <button onClick={() => sheets.swap('hdrezka')}>swap</button>
      <button onClick={() => sheets.close()}>close</button>
    </>
  );
}

describe('useSheet', () => {
  it('goes Back when closing a sheet it opened', () => {
    const { rerender } = render(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'open' }));
    expect(mockRouter.push).toHaveBeenCalledWith('/library?sheet=infuse-setup', { scroll: false });
    mockSearch = 'sheet=infuse-setup';
    rerender(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'close' }));
    expect(mockRouter.back).toHaveBeenCalled();
    expect(mockRouter.replace).not.toHaveBeenCalled();
  });

  it('swaps a sibling sheet in place, keeping its params, and Back still closes it', () => {
    const { rerender } = render(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'open torrents' }));
    mockSearch = 'sheet=torrent&movie=157336';
    rerender(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'swap' }));
    expect(mockRouter.replace).toHaveBeenCalledWith('/library?sheet=hdrezka&movie=157336', { scroll: false });
    mockSearch = 'sheet=hdrezka&movie=157336';
    rerender(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'close' }));
    expect(mockRouter.back).toHaveBeenCalled();
  });

  it('forgets the sheet it opened once Back closed it, so a later deep link closes in place', () => {
    const { rerender } = render(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'open' }));
    mockSearch = 'sheet=infuse-setup';
    rerender(<Probe />);
    // Back
    mockSearch = '';
    rerender(<Probe />);
    // Deep link to the same sheet
    mockSearch = 'sheet=infuse-setup';
    rerender(<Probe />);
    fireEvent.click(screen.getByRole('button', { name: 'close' }));
    expect(mockRouter.back).not.toHaveBeenCalled();
    expect(mockRouter.replace).toHaveBeenCalledWith('/library', { scroll: false });
  });
});
