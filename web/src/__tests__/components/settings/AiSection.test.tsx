import { fireEvent, render, screen, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { AiSection } from '@/components/settings/AiSection';
import type { AiSettings, AiSettingsPatch, AiUsage } from '@/lib/api/contract';
import { AI_SETTINGS } from '@/mocks/fixtures/settings';

/** Settings › AI & picks (#159): partial saves, the budget's errors, and no key. */

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

afterEach(() => {
  global.fetch = originalFetch;
});

function usage(overrides: Partial<AiUsage> = {}): AiUsage {
  return {
    configured: true,
    month: '2026-10',
    resets_at: '2026-11-01T00:00:00Z',
    total_usd: 3.12,
    budget_usd: 15,
    budget_reached: false,
    by_feature: [
      { feature: 'recs', cost_usd: 2.18, requests: 41 },
      { feature: 'ask', cost_usd: 0.53, requests: 23 },
      { feature: 'classifier', cost_usd: 0.41, requests: 96 },
    ],
    recs: { last_run: null, active_run: null, next_run_at: null },
    estimates: { run_usd: { 'claude-opus-5-5': 0.36, 'claude-sonnet-5-5': null }, ask_usd: null },
    asks_today: 0,
    ask_daily_limit: 30,
    ...overrides,
  };
}

/** GET /settings/ai answers the fixture, the usage `usageBody`; PUT answers `put(body)`. */
function renderSection({ put = (body: AiSettingsPatch) => answer(200, { ...AI_SETTINGS, ...body }), usageBody = usage() } = {}) {
  const puts: AiSettingsPatch[] = [];
  global.fetch = jest.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as AiSettingsPatch;
      puts.push(body);
      return Promise.resolve(put(body));
    }
    if (url.endsWith('/api/settings/ai/usage')) return Promise.resolve(answer(200, usageBody));
    return Promise.resolve(answer(200, AI_SETTINGS satisfies AiSettings));
  }) as typeof fetch;
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <AiSection />
      </ToastProvider>
    </QueryClientProvider>
  );
  return puts;
}

const bar = () => screen.queryByRole('region', { name: 'Unsaved changes' });

describe('AiSection', () => {
  it('shows the bar only while something changed, and PUTs only the changed fields', async () => {
    const puts = renderSection();
    const weekly = within(await screen.findByRole('group', { name: 'Recommendation schedule' })).getByRole('button', { name: 'Weekly' });
    expect(screen.getByRole('radio', { name: 'Sonnet 5.5' })).toHaveAccessibleDescription(/No estimate until its first run/);
    expect(bar()).not.toBeInTheDocument();

    fireEvent.click(weekly);
    expect(bar()).toBeInTheDocument();
    // Back to what was loaded: nothing to save
    fireEvent.click(within(screen.getByRole('group', { name: 'Recommendation schedule' })).getByRole('button', { name: 'Nightly' }));
    expect(bar()).not.toBeInTheDocument();

    fireEvent.click(weekly);
    fireEvent.click(screen.getByRole('switch', { name: 'Ask AI about tricky files' }));
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));

    expect(await screen.findByText('Settings saved')).toBeInTheDocument();
    expect(bar()).not.toBeInTheDocument();
    expect(puts).toEqual([{ rec_schedule: 'weekly', classifier_enabled: false }]);
  });

  it('keeps a budget it can’t read, and shows a 422 on the field', async () => {
    const puts = renderSection({
      put: () => answer(422, { error: { code: 'validation_error', message: 'monthly_budget_usd must be 0 or more, or null for no cap', field: 'monthly_budget_usd' } }),
    });
    const input = await screen.findByRole('textbox', { name: 'Budget' });
    expect(input).toHaveValue('15.00');

    fireEvent.change(input, { target: { value: 'lots' } });
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(within(bar()!).getByRole('alert')).toHaveTextContent('Check “Budget” above.');
    expect(input).toHaveAccessibleDescription(/^Enter an amount in dollars/);
    expect(puts).toEqual([]);

    fireEvent.change(input, { target: { value: '-5' } });
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(await within(bar()!).findByRole('alert')).toHaveTextContent('Check “Budget” above.');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription(/^monthly_budget_usd must be 0 or more/);
    expect(puts).toEqual([{ monthly_budget_usd: -5 }]);
  });

  it('without a key, turns the picks controls off and still saves the classifier', async () => {
    const puts = renderSection({ usageBody: usage({ configured: false }) });
    expect(await screen.findByText('No Anthropic key on this server')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run now' })).toBeDisabled();
    expect(screen.getByRole('switch', { name: 'Write personal picks' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'Sonnet 5.5' })).toBeDisabled();

    fireEvent.click(screen.getByRole('switch', { name: 'Auto-apply high-confidence mappings' }));
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Settings saved')).toBeInTheDocument();
    expect(puts).toEqual([{ auto_apply_high_confidence: false }]);
  });
});
