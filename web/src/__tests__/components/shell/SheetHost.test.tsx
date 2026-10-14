import { fireEvent, render, screen } from '@testing-library/react';
import { SheetHost } from '@/components/shell/SheetHost';
import { getSheet } from '@/lib/sheets/registry';
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
});

function Probe() {
  const sheets = useSheet();
  return (
    <>
      <button onClick={() => sheets.open('infuse-setup')}>open</button>
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
