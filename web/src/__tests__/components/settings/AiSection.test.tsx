import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MembersGate } from '@/components/members/MembersGate';
import { ToastProvider } from '@/components/ui';
import { AiSection } from '@/components/settings/AiSection';
import type { AiSettings, AiSettingsPatch, AiUsage, Member } from '@/lib/api/contract';
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

  it('keeps a budget it can’t send here, and words a server 422 on the field, focusing it', async () => {
    const puts = renderSection({
      put: () => answer(422, { error: { code: 'validation_error', message: 'monthly_budget_usd must be 0 or more, or null for no cap', field: 'monthly_budget_usd' } }),
    });
    const input = await screen.findByRole('textbox', { name: 'Budget' });
    expect(input).toHaveValue('15.00');

    fireEvent.change(input, { target: { value: 'lots' } });
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(within(bar()!).getByRole('alert')).toHaveTextContent('Check “Budget” above.');
    expect(input).toHaveAccessibleDescription(/^Enter an amount in dollars/);
    expect(input).toHaveFocus();

    fireEvent.change(input, { target: { value: '-5' } });
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(input).toHaveAccessibleDescription(/^The budget can’t be negative\. Enter 0 or more, or leave it empty for no cap\./);
    expect(puts).toEqual([]);

    fireEvent.change(input, { target: { value: '20' } });
    within(bar()!).getByRole('button', { name: 'Save' }).focus();
    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(await within(bar()!).findByRole('alert')).toHaveTextContent('Check “Budget” above.');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription(/^momoshtrem didn’t accept that budget\. Enter 0 or more, or leave it empty for no cap\./);
    expect(input).toHaveFocus();
    expect(puts).toEqual([{ monthly_budget_usd: 20 }]);
  });

  it('sends only what was edited, so a change made elsewhere stays', async () => {
    let served: AiSettings = { ...AI_SETTINGS };
    const puts: AiSettingsPatch[] = [];
    global.fetch = jest.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        const body = JSON.parse(String(init.body)) as AiSettingsPatch;
        puts.push(body);
        served = { ...served, ...body };
        return Promise.resolve(answer(200, served));
      }
      if (String(input).endsWith('/api/settings/ai/usage')) return Promise.resolve(answer(200, usage()));
      return Promise.resolve(answer(200, served));
    }) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <ToastProvider>
          <AiSection />
        </ToastProvider>
      </QueryClientProvider>
    );
    fireEvent.click(within(await screen.findByRole('group', { name: 'Recommendation schedule' })).getByRole('button', { name: 'Weekly' }));
    // Another device switches the model meanwhile; this page refetches
    served = { ...served, rec_model: 'claude-sonnet-5-5' };
    await act(() => client.refetchQueries({ queryKey: ['settings', 'ai'], exact: true }));
    await waitFor(() => expect(screen.getByRole('radio', { name: 'Sonnet 5.5' })).toBeChecked());

    fireEvent.click(within(bar()!).getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Settings saved')).toBeInTheDocument();
    expect(puts).toEqual([{ rec_schedule: 'weekly' }]);
    expect(served.rec_model).toBe('claude-sonnet-5-5');
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

describe('AiSection in a household (#239)', () => {
  const person = (id: number, name: string, color: Member['color']): Member => ({
    id,
    name,
    color,
    created_at: '2026-01-01T00:00:00Z',
    trakt: { state: 'connected', username: name.toLowerCase() },
  });
  const BY_MEMBER = [
    { member_id: 1, name: 'Teymur', cost_usd: 2.03, requests: 48 },
    { member_id: 2, name: 'Sam', cost_usd: 0.68, requests: 16 },
  ];

  /** As Teymur (1), with /api/members answering `members`. */
  function renderInHousehold(members: Member[], usageBody: AiUsage) {
    document.cookie = 'momoshtrem_member=1; Path=/';
    global.fetch = jest.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/members') return Promise.resolve(answer(200, { items: members }));
      if (url.endsWith('/api/settings/ai/usage')) return Promise.resolve(answer(200, usageBody));
      return Promise.resolve(answer(200, AI_SETTINGS));
    }) as typeof fetch;
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ToastProvider>
          <MembersGate initialMemberId={1}>
            <AiSection />
          </MembersGate>
        </ToastProvider>
      </QueryClientProvider>
    );
  }

  afterEach(() => {
    document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
  });

  it('says whose runs these are, and splits picks and Ask by person', async () => {
    renderInHousehold([person(1, 'Teymur', 'teal'), person(2, 'Sam', 'rose')], usage({ by_member: BY_MEMBER }));
    const picks = await screen.findByRole('region', { name: 'Personal picks' });
    expect(await within(picks).findByText('Runs and Run now are Teymur’s. The schedule, model and budget are shared.')).toBeInTheDocument();
    const split = screen.getByRole('list', { name: 'Picks and Ask by person' });
    expect(within(split).getAllByRole('listitem').map((row) => row.textContent)).toEqual(['TTeymur$2.03', 'SSam$0.68']);
    expect(split.querySelector('.member-avatar[data-color="rose"]')).not.toBeNull();
  });

  it('alone, or from a backend without the split, neither shows', async () => {
    renderInHousehold([person(1, 'Teymur', 'teal')], usage({ by_member: BY_MEMBER.slice(0, 1) }));
    await screen.findByRole('region', { name: 'Personal picks' });
    expect(await screen.findByRole('img', { name: /^Spent \$3\.12/ })).toBeInTheDocument();
    expect(screen.queryByText(/Runs and Run now are/)).not.toBeInTheDocument();
    expect(screen.queryByRole('list', { name: 'Picks and Ask by person' })).not.toBeInTheDocument();
  });

  it('two people but no by_member (an older backend): the note, no split', async () => {
    renderInHousehold([person(1, 'Teymur', 'teal'), person(2, 'Sam', 'rose')], usage());
    expect(await screen.findByText('Runs and Run now are Teymur’s. The schedule, model and budget are shared.')).toBeInTheDocument();
    expect(screen.queryByRole('list', { name: 'Picks and Ask by person' })).not.toBeInTheDocument();
  });
});
