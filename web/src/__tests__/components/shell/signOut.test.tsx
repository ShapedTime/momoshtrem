import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { signOut, useSignOut } from '@/components/shell/signOut';

const originalFetch = global.fetch;
const fetchMock = jest.fn();

beforeEach(() => {
  global.fetch = fetchMock;
});

afterEach(() => {
  fetchMock.mockReset();
  global.fetch = originalFetch;
});

const answer = (status: number) => ({ ok: status >= 200 && status < 300, status }) as Response;

describe('signOut', () => {
  it('stays put when the server doesn’t sign you out', async () => {
    fetchMock.mockResolvedValue(answer(500));
    const clearCache = jest.fn();
    await expect(signOut(clearCache)).resolves.toBe(false);
    expect(fetchMock).toHaveBeenCalledWith('/api/auth', expect.objectContaining({ method: 'DELETE' }));
    expect(clearCache).not.toHaveBeenCalled();
  });

  it('stays put when the request never gets an answer', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
    const clearCache = jest.fn();
    await expect(signOut(clearCache)).resolves.toBe(false);
    expect(clearCache).not.toHaveBeenCalled();
  });
});

describe('useSignOut', () => {
  function LogOut() {
    const signOut = useSignOut();
    return <button onClick={() => signOut()}>Log out</button>;
  }

  it('says so when signing out fails', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ToastProvider>
          <LogOut />
        </ToastProvider>
      </QueryClientProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: 'Log out' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Couldn’t sign out');
    expect(alert).toHaveTextContent('Check your connection and try again.');
  });
});
