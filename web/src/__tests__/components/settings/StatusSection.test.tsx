import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { StatusSection } from '@/components/settings/StatusSection';
import { systemStatus } from '@/mocks/fixtures/system';

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

const answer = (status: number, body: unknown) => ({ ok: status >= 200 && status < 300, status, json: async () => body }) as Response;

function renderSection() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <StatusSection />
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe('StatusSection', () => {
  it('a failed re-check replaces "All good" with the error and keeps the last rows', async () => {
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce(answer(200, systemStatus()))
      .mockResolvedValueOnce(answer(503, { error: { code: 'backend_unreachable', message: 'Can’t reach momoshtrem.' } }));
    renderSection();

    expect(await screen.findByText('All good')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Re-check' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Can’t reach momoshtrem');
    expect(alert).toHaveTextContent('Below is the last successful check');
    expect(screen.queryByText('All good')).not.toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Services' })).toBeInTheDocument();
    expect(global.fetch).toHaveBeenLastCalledWith('/api/status?refresh=1', expect.anything());
  });

  it('shows only the error when the first check fails', async () => {
    global.fetch = jest.fn().mockResolvedValue(answer(500, { error: { code: 'internal_error', message: 'Database is locked.' } }));
    renderSection();

    expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t check the services');
    expect(screen.queryByRole('list', { name: 'Services' })).not.toBeInTheDocument();
  });
});
