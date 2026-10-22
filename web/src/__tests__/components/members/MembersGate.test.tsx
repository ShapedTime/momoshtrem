import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { MembersGate, useMembers } from '@/components/members/MembersGate';
import { ToastProvider } from '@/components/ui';
import type { Member } from '@/lib/api/contract';
import { upNextQuery } from '@/lib/api/endpoints';
import { leavePage, page, resetPage, stayOnPage } from '@/lib/members/page';
import { switchMember } from '@/lib/members/switch';

/** Who's watching? (#239): nothing personal is asked before someone is chosen. */

const member = (id: number, name: string): Member => ({
  id,
  name,
  color: id === 1 ? 'teal' : 'rose',
  created_at: '2026-01-01T00:00:00Z',
  trakt: { state: 'connected', username: name.toLowerCase() },
});
const TWO = [member(1, 'Teymur'), member(2, 'Sam')];

const originalFetch = global.fetch;
const requests: string[] = [];
/** The member cookie each request went out with. */
const cookies: string[] = [];
type Answer = { status: number; body?: unknown } | 'never';

const answer = (status: number, body?: unknown): Response =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;

/** /api/members answers `members`; /api/me/up-next an empty list. Every request is logged. */
function stubFetch(members: Answer) {
  global.fetch = jest.fn((input: RequestInfo | URL) => {
    const url = String(input);
    requests.push(url);
    cookies.push(/momoshtrem_member=\d+/.exec(document.cookie)?.[0] ?? '');
    if (url === '/api/members') {
      if (members === 'never') return new Promise<Response>(() => undefined);
      return Promise.resolve(answer(members.status, members.body));
    }
    return Promise.resolve(answer(200, { items: [] }));
  }) as typeof fetch;
}

/** A personal page: its query must never run before a member is chosen. */
function Probe() {
  useQuery(upNextQuery());
  const { current, others } = useMembers();
  return (
    <p>
      App for {current?.name ?? 'nobody yet'}; others: {others.map((m) => m.name).join(', ') || 'none'}
    </p>
  );
}

function renderGate(initialMemberId: number | null, client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MembersGate initialMemberId={initialMemberId}>
          <Probe />
        </MembersGate>
      </ToastProvider>
    </QueryClientProvider>
  );
}

const clearCookie = () => {
  document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
};

let reload: jest.SpyInstance;
let assign: jest.SpyInstance;
beforeEach(() => {
  reload = jest.spyOn(page, 'reload').mockImplementation(() => undefined);
  assign = jest.spyOn(page, 'assign').mockImplementation(() => undefined);
});
afterEach(() => {
  global.fetch = originalFetch;
  requests.length = 0;
  cookies.length = 0;
  clearCookie();
  resetPage();
  jest.restoreAllMocks();
});

describe('MembersGate', () => {
  it('no cookie and two people: the picker, and no personal query is ever sent', async () => {
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(null);
    expect(await screen.findByRole('heading', { name: 'Who’s watching?' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Teymur' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sam' })).toBeInTheDocument();
    expect(screen.queryByText(/App for/)).not.toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(requests).toEqual(['/api/members']);
  });

  it('picking someone writes the cookie and reloads where you were going (a hash included)', async () => {
    stubFetch({ status: 200, body: { items: TWO } });
    window.history.replaceState(null, '', '/activity#act-dl-h');
    renderGate(null);
    fireEvent.click(await screen.findByRole('button', { name: 'Sam' }));
    expect(document.cookie).toContain('momoshtrem_member=2');
    expect(reload).toHaveBeenCalledTimes(1);
    expect(assign).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Sam' })).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByRole('button', { name: 'Teymur' })).toBeDisabled();
    // Its own cookie write isn't another tab's: no second reload over the first
    act(() => window.dispatchEvent(new Event('focus')));
    expect(reload).toHaveBeenCalledTimes(1);
    window.history.replaceState(null, '', '/');
  });

  it('back/forward cache restores a page after its switch: it reloads as whoever the cookie names now', async () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    window.history.replaceState(null, '', '/library');
    renderGate(1);
    await screen.findByText('App for Teymur; others: Sam');
    switchMember(2, () => undefined);
    expect(assign).toHaveBeenCalledWith('/');
    act(() => window.dispatchEvent(new Event('focus')));
    expect(reload).not.toHaveBeenCalled();
    // Back: the old page shows again, rendered for Teymur, with Sam's cookie
    act(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
    expect(reload).toHaveBeenCalledTimes(1);
    window.history.replaceState(null, '', '/');
  });

  it('back/forward cache restores a page that was leaving, with the cookie its own member’s again: it reloads anyway', async () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    window.history.replaceState(null, '', '/library');
    renderGate(1);
    await screen.findByText('App for Teymur; others: Sam');
    switchMember(2, () => undefined);
    // Switched back to Teymur elsewhere; the requests this page held back while leaving would never settle
    document.cookie = 'momoshtrem_member=1; Path=/';
    act(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
    expect(reload).toHaveBeenCalledTimes(1);
    window.history.replaceState(null, '', '/');
  });

  it('a page restored from back/forward cache that wasn’t leaving, its member unchanged: no reload', async () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(1);
    await screen.findByText('App for Teymur; others: Sam');
    act(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
    expect(reload).not.toHaveBeenCalled();
  });

  it('a switch between the server’s render and hydration reloads on mount, before anything is sent as the wrong person', async () => {
    document.cookie = 'momoshtrem_member=2; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(1);
    expect(reload).toHaveBeenCalledTimes(1);
    expect(requests).toEqual([]);
  });

  it('one person: chosen automatically, the cookie first, then the app', async () => {
    stubFetch({ status: 200, body: { items: TWO.slice(0, 1) } });
    renderGate(null);
    expect(await screen.findByText('App for Teymur; others: none')).toBeInTheDocument();
    expect(document.cookie).toContain('momoshtrem_member=1');
    await waitFor(() => expect(requests).toEqual(['/api/members', '/api/me/up-next']));
    expect(cookies).toEqual(['', 'momoshtrem_member=1']);
    expect(reload).not.toHaveBeenCalled();
  });

  it('a valid cookie renders the app before the list answers', () => {
    document.cookie = 'momoshtrem_member=2; Path=/';
    stubFetch('never');
    renderGate(2);
    expect(screen.getByText('App for nobody yet; others: none')).toBeInTheDocument();
  });

  it('then knows who’s watching once the list confirms them', async () => {
    document.cookie = 'momoshtrem_member=2; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(2);
    expect(await screen.findByText('App for Sam; others: Teymur')).toBeInTheDocument();
  });

  it('a cookie that names nobody is forgotten, and the page reloads', async () => {
    document.cookie = 'momoshtrem_member=9; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(9);
    await waitFor(() => expect(reload).toHaveBeenCalled());
    expect(document.cookie).not.toContain('momoshtrem_member');
  });

  it('removing yourself (the page leaving): a list without you keeps the app as it was; calling it off hands back to the list', async () => {
    // TanStack tells its observers on the next tick: let the gate render what the list says
    const tick = () => act(() => new Promise((resolve) => setTimeout(resolve, 20)));
    document.cookie = 'momoshtrem_member=1; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderGate(1, client);
    await screen.findByText('App for Teymur; others: Sam');

    // The DELETE is out: the server stops listing Teymur, and a refetch says so
    leavePage();
    stubFetch({ status: 200, body: { items: TWO.slice(1) } });
    await act(() => client.refetchQueries({ queryKey: ['members'] }));
    await tick();
    expect(client.getQueryData(['members'])).toEqual({ items: TWO.slice(1) });
    expect(screen.getByText('App for Teymur; others: Sam')).toBeInTheDocument();
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');

    // It failed: the sheet puts the list back as the DELETE found it, then calls the leave off. Teymur stays
    act(() => {
      client.setQueryData(['members'], { items: TWO });
      stayOnPage();
    });
    await tick();
    expect(screen.getByText('App for Teymur; others: Sam')).toBeInTheDocument();
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');

    // A fresh list decides. One that still lacks him (it went through, only the answer was lost): the stale path
    await act(() => client.refetchQueries({ queryKey: ['members'] }));
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    expect(document.cookie).toContain('momoshtrem_member=2');
  });

  it('a stale cookie with one person left: that person, and a reload', async () => {
    document.cookie = 'momoshtrem_member=9; Path=/';
    stubFetch({ status: 200, body: { items: TWO.slice(0, 1) } });
    renderGate(9);
    await waitFor(() => expect(reload).toHaveBeenCalled());
    expect(document.cookie).toContain('momoshtrem_member=1');
  });

  it('another tab switched person: coming back reloads', async () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    stubFetch({ status: 200, body: { items: TWO } });
    renderGate(1);
    await screen.findByText('App for Teymur; others: Sam');
    act(() => window.dispatchEvent(new Event('focus')));
    expect(reload).not.toHaveBeenCalled();
    document.cookie = 'momoshtrem_member=2; Path=/';
    act(() => window.dispatchEvent(new Event('focus')));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it('no cookie and the list failed: the picker’s error, with Try again', async () => {
    stubFetch({ status: 502, body: { error: { code: 'backend_unreachable', message: 'Can’t reach momoshtrem.' } } });
    renderGate(null);
    expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t load who’s watching');
    expect(screen.getByRole('alert')).toHaveTextContent('momoshtrem didn’t answer. Check that it’s running, then try again.');
    stubFetch({ status: 200, body: { items: TWO } });
    fireEvent.click(screen.getByRole('button', { name: /Try again/ }));
    expect(await screen.findByRole('button', { name: 'Sam' })).toBeInTheDocument();
  });

  it('a backend without household members (404): the app, as before', async () => {
    stubFetch({ status: 404, body: { error: { code: 'not_found', message: 'No such API route.' } } });
    renderGate(null);
    expect(await screen.findByText('App for nobody yet; others: none')).toBeInTheDocument();
    expect(document.cookie).not.toContain('momoshtrem_member');
  });
});
