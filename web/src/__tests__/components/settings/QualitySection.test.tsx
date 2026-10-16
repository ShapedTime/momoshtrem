import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { QualitySection } from '@/components/settings/QualitySection';
import type { QualityProfile } from '@/lib/api/contract';
import { QUALITY_PROFILE } from '@/mocks/fixtures/settings';

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body }) as Response;

afterEach(() => {
  global.fetch = originalFetch;
});

/** GET answers the fixture; PUT answers `put(body)`. */
function renderSection(put: (body: QualityProfile) => Response = (body) => answer(200, body)) {
  const puts: QualityProfile[] = [];
  global.fetch = jest.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as QualityProfile;
      puts.push(body);
      return Promise.resolve(put(body));
    }
    return Promise.resolve(answer(200, QUALITY_PROFILE));
  }) as typeof fetch;
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <QualitySection />
      </ToastProvider>
    </QueryClientProvider>
  );
  return puts;
}

const bar = () => screen.queryByRole('region', { name: 'Unsaved changes' });

describe('QualitySection', () => {
  it('shows the bar only while something changed, and saves the whole profile', async () => {
    const puts = renderSection();
    const preview = await screen.findByRole('status', { name: 'Best match would prefer' });
    expect(preview).toHaveTextContent('2160p · DV · ≤ 8 GB/h');
    expect(bar()).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Max size per hour'), { target: { value: '12.5' } });
    expect(preview).toHaveTextContent('2160p · DV · ≤ 12.5 GB/h');
    expect(screen.getByText('12.5 GB/h ≈ 35 GB for a 2 h 46 m film')).toBeInTheDocument();
    expect(bar()).toBeInTheDocument();

    // Back to what was loaded: nothing to save
    fireEvent.change(screen.getByLabelText('Max size per hour'), { target: { value: '8' } });
    expect(bar()).not.toBeInTheDocument();

    fireEvent.click(within(screen.getByRole('group', { name: 'Preferred resolution' })).getByRole('button', { name: '1080p' }));
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));

    expect(await screen.findByText('Settings saved')).toBeInTheDocument();
    expect(bar()).not.toBeInTheDocument();
    expect(puts).toEqual([{ ...QUALITY_PROFILE, preferred_resolution: '1080p' }]);
  });

  it('keeps the maximum at or above the preferred resolution', async () => {
    renderSection();
    const max = within(await screen.findByRole('group', { name: 'Maximum resolution' }));
    expect(max.getByRole('button', { name: '1080p' })).toBeDisabled();

    fireEvent.click(within(screen.getByRole('group', { name: 'Preferred resolution' })).getByRole('button', { name: '1080p' }));
    expect(max.getByRole('button', { name: '1080p' })).toBeEnabled();
    fireEvent.click(max.getByRole('button', { name: '1080p' }));
    fireEvent.click(within(screen.getByRole('group', { name: 'Preferred resolution' })).getByRole('button', { name: '2160p' }));
    expect(max.getByRole('button', { name: '2160p' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('adds and removes terms, refusing a duplicate in any case; Discard puts them back', async () => {
    renderSection();
    const input = await screen.findByRole('textbox', { name: 'Add to Terms to avoid' });
    fireEvent.change(input, { target: { value: ' hdts ' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(within(screen.getByRole('list', { name: 'Terms to avoid' })).getByText('hdts')).toBeInTheDocument();
    expect(input).toHaveValue('');

    fireEvent.change(input, { target: { value: 'cam' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(screen.getByRole('alert')).toHaveTextContent('CAM is already on the list.');
    expect(input).toHaveAttribute('aria-invalid', 'true');

    fireEvent.click(screen.getByRole('button', { name: 'Remove KORSUB' }));
    expect(screen.queryByText('KORSUB')).not.toBeInTheDocument();
    expect(screen.getByRole('status', { name: 'Best match would prefer' })).toHaveTextContent('never CAM, TS, HDCAM +3');

    fireEvent.click(within(bar()!).getByRole('button', { name: 'Discard' }));
    expect(bar()).not.toBeInTheDocument();
    expect(screen.getByText('KORSUB')).toBeInTheDocument();
    expect(screen.queryByText('hdts')).not.toBeInTheDocument();
  });

  it('shows a 422 next to the field it names, and the bar points at it', async () => {
    renderSection(() => answer(422, { error: { code: 'validation_error', message: 'Each term needs 1 to 30 characters.', field: 'avoid_terms' } }));
    fireEvent.click(within(await screen.findByRole('group', { name: 'Preferred resolution' })).getByRole('button', { name: '1080p' }));
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));

    expect(await within(bar()!).findByRole('alert')).toHaveTextContent('Check “Terms to avoid” above.');
    const input = screen.getByRole('textbox', { name: 'Add to Terms to avoid' });
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription('Each term needs 1 to 30 characters.');

    // Editing again clears it until the next save
    fireEvent.click(screen.getByRole('switch', { name: 'Prefer HDR' }));
    await waitFor(() => expect(input).not.toHaveAttribute('aria-invalid'));
  });

  it('says why the profile didn’t load, with Try again', async () => {
    global.fetch = jest.fn().mockResolvedValue(answer(500, { error: { code: 'internal_error', message: 'Database is locked.' } }));
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ToastProvider>
          <QualitySection />
        </ToastProvider>
      </QueryClientProvider>
    );
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Couldn’t load your quality profile');
    expect(within(alert).getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });
});
