import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MembersGate } from '@/components/members/MembersGate';
import { HouseholdSection } from '@/components/settings/HouseholdSection';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { apiJson } from '@/lib/api/client';
import type { Member, MemberColor } from '@/lib/api/contract';
import { leavingPage, page, resetPage } from '@/lib/members/page';

/** Settings › Household (#247): the list, Add and Edit in the sheet, and Remove with its confirm. */

const person = (id: number, name: string, color: MemberColor, trakt: Member['trakt'] = { state: 'connected', username: name.toLowerCase() }): Member => ({
  id,
  name,
  color,
  created_at: '2026-01-01T00:00:00Z',
  trakt,
});
const TEYMUR = person(1, 'Teymur', 'teal');
const SAM = person(2, 'Sam', 'rose');

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown): Response =>
  ({ ok: status < 300, status, json: async () => body, clone: () => answer(status, body) }) as Response;
const error = (status: number, code: string, field?: string) => answer(status, { error: { code, message: code, ...(field ? { field } : {}) } });

interface Call {
  method: string;
  url: string;
  body?: unknown;
}

/**
 * A small household backend: GET, POST, PATCH and DELETE on /api/members, by
 * the contract's rules that matter here. `override` answers a call first.
 */
type Override = (call: Call) => Response | Promise<Response> | undefined;

function stubBackend(initial: Member[], override?: Override) {
  let items = [...initial];
  const calls: Call[] = [];
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const call: Call = { method: init?.method ?? 'GET', url: String(input), body: init?.body ? JSON.parse(String(init.body)) : undefined };
    if (call.method !== 'GET') calls.push(call);
    const answered = override?.(call);
    if (answered) return answered;
    const body = call.body as { name?: string; color?: MemberColor } | undefined;
    if (call.url === '/api/members' && call.method === 'GET') return answer(200, { items });
    if (call.url === '/api/members' && call.method === 'POST') {
      const name = body?.name ?? '';
      if (items.some((m) => m.name.toLowerCase() === name.toLowerCase())) return error(409, 'member_name_taken');
      const created = person(Math.max(...items.map((m) => m.id)) + 1, name, body?.color ?? 'lime', { state: 'off', username: null });
      items = [...items, created];
      return answer(201, created);
    }
    const id = Number(call.url.split('/').pop());
    if (call.method === 'PATCH') {
      items = items.map((m) => (m.id === id ? { ...m, ...body } : m));
      return answer(200, items.find((m) => m.id === id));
    }
    if (call.method === 'DELETE') {
      items = items.filter((m) => m.id !== id);
      return answer(204);
    }
    return answer(404);
  }) as typeof fetch;
  return calls;
}

/** The last render's client, for a test to refetch through (as focus or a poll would). */
let queryClient: QueryClient;

function renderSection(members: Member[], override?: Override) {
  document.cookie = 'momoshtrem_member=1; Path=/';
  const calls = stubBackend(members, override);
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <ConfirmProvider>
          <MembersGate initialMemberId={1}>
            <HouseholdSection />
          </MembersGate>
        </ConfirmProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
  return calls;
}

const sheet = (name: string) => screen.findByRole('dialog', { name });
const nameField = (dialog: HTMLElement) => within(dialog).getByRole('textbox', { name: 'Name' });

let assign: jest.SpyInstance;
let reload: jest.SpyInstance;
beforeEach(() => {
  // Where the section lives, so going to For You is a navigation, not a reload of this page
  window.history.pushState({}, '', '/settings?section=household');
  assign = jest.spyOn(page, 'assign').mockImplementation(() => undefined);
  reload = jest.spyOn(page, 'reload').mockImplementation(() => undefined);
});
afterEach(() => {
  window.history.pushState({}, '', '/');
  global.fetch = originalFetch;
  document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
  resetPage();
  jest.restoreAllMocks();
});

describe('HouseholdSection', () => {
  it('lists everyone: who’s watching here, their Trakt, and whose needs signing in again', async () => {
    renderSection([TEYMUR, SAM, person(3, 'Alex', 'periwinkle', { state: 'reauth', username: 'alex' })]);
    const rows = await screen.findAllByRole('listitem');
    expect(rows.map((row) => row.textContent)).toEqual([
      'TTeymurWatching nowTrakt · teymurEdit',
      'SSamTrakt · samEdit',
      'AAlexSign in againTrakt · sign in againEdit',
    ]);
    expect(within(rows[1]).getByRole('button', { name: 'Edit Sam' })).toBeInTheDocument();
    expect(screen.getByText(/^Everyone shares the library, downloads, Activity and settings\./)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add someone' })).toBeEnabled();
  });

  it('Add checks the name, says a taken one on the field, then offers to switch to them', async () => {
    const calls = renderSection([TEYMUR, SAM]);
    fireEvent.click(await screen.findByRole('button', { name: 'Add someone' }));
    const dialog = await sheet('Add someone');
    // The first colour nobody uses
    expect(within(dialog).getByRole('radio', { name: 'Orchid' })).toBeChecked();

    fireEvent.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(await within(dialog).findByText('Give them a name.')).toBeInTheDocument();
    expect(nameField(dialog)).toHaveAttribute('aria-invalid', 'true');
    fireEvent.change(nameField(dialog), { target: { value: 'x'.repeat(33) } });
    expect(within(dialog).getByText('Keep it to 32 characters.')).toBeInTheDocument();
    expect(calls).toEqual([]);

    fireEvent.change(nameField(dialog), { target: { value: ' sam ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(await within(dialog).findByText('Someone here is already called Sam.')).toBeInTheDocument();
    expect(nameField(dialog)).toHaveFocus();

    fireEvent.change(nameField(dialog), { target: { value: 'Alex' } });
    expect(within(dialog).queryByText('Someone here is already called Sam.')).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('radio', { name: 'Lime' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add' }));
    const toast = await screen.findByText('Alex was added');
    expect(calls.at(-1)).toEqual({ method: 'POST', url: '/api/members', body: { name: 'Alex', color: 'lime' } });
    expect(await screen.findByText('No Trakt yet')).toBeInTheDocument();

    fireEvent.click(within(toast.closest('[role="status"]') as HTMLElement).getByRole('button', { name: 'Switch to Alex' }));
    expect(document.cookie).toContain('momoshtrem_member=3');
    expect(assign).toHaveBeenCalledWith('/');
  });

  it('a household of six can’t add anyone', async () => {
    renderSection([TEYMUR, SAM, person(3, 'Alex', 'periwinkle'), person(4, 'Mina', 'lime'), person(5, 'Jo', 'orchid'), person(6, 'Riley', 'sand')]);
    expect(await screen.findByRole('button', { name: 'Add someone' })).toBeDisabled();
    expect(screen.getByText('A household can have up to six people.')).toBeInTheDocument();
  });

  it('Edit sends only what changed, and the list follows', async () => {
    const calls = renderSection([TEYMUR, SAM]);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Sam' }));
    const dialog = await sheet('Edit Sam');
    expect(nameField(dialog)).toHaveValue('Sam');
    expect(within(dialog).getByRole('radio', { name: 'Rose' })).toBeChecked();

    fireEvent.change(nameField(dialog), { target: { value: '  Samira ' } });
    fireEvent.click(within(dialog).getByRole('radio', { name: 'Lime' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Samira was updated')).toBeInTheDocument();
    expect(calls).toEqual([{ method: 'PATCH', url: '/api/members/2', body: { name: 'Samira', color: 'lime' } }]);
    expect(await screen.findByRole('button', { name: 'Edit Samira' })).toBeInTheDocument();
  });

  it('a save that doesn’t reach the server says so in the sheet', async () => {
    renderSection([TEYMUR, SAM], (call) => (call.method === 'PATCH' ? answer(502, { error: { code: 'bad_gateway', message: 'down' } }) : undefined));
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Sam' }));
    const dialog = await sheet('Edit Sam');
    fireEvent.click(within(dialog).getByRole('radio', { name: 'Sand' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Couldn’t save. Check your connection and try again.');
  });

  it('Remove is confirmed with what goes and what stays, then the row goes (no Undo)', async () => {
    const calls = renderSection([TEYMUR, SAM]);
    const edit = await screen.findByRole('button', { name: 'Edit Sam' });
    // As a click in a browser would: the opener has focus
    edit.focus();
    fireEvent.click(edit);
    fireEvent.click(within(await sheet('Edit Sam')).getByRole('button', { name: 'Remove Sam' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Remove Sam from the household?' });
    expect(confirm).toHaveTextContent(
      'Sam’s Trakt connection and imported history, For You picks, Ask threads and picks feedback are deleted from momoshtrem. The library, downloads and settings stay. Nothing changes on Trakt.'
    );
    fireEvent.click(within(confirm).getByRole('button', { name: 'Remove Sam' }));
    expect(await screen.findByText('Sam was removed from the household')).toBeInTheDocument();
    expect(calls).toEqual([{ method: 'DELETE', url: '/api/members/2', body: undefined }]);
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Edit Sam' })).not.toBeInTheDocument());
    expect(screen.queryByRole('button', { name: 'Undo' })).not.toBeInTheDocument();
    // Sam's Edit went with his row: focus lands on Add someone, not on the page
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add someone' })).toHaveFocus());
  });

  it('an Add that fills the household lands focus on the People heading (Add is off)', async () => {
    renderSection([TEYMUR, SAM, person(3, 'Alex', 'periwinkle'), person(4, 'Mina', 'lime'), person(5, 'Jo', 'orchid')]);
    const add = await screen.findByRole('button', { name: 'Add someone' });
    add.focus();
    fireEvent.click(add);
    const dialog = await sheet('Add someone');
    fireEvent.change(nameField(dialog), { target: { value: 'Riley' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(await screen.findByText('Riley was added')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add someone' })).toBeDisabled());
    await waitFor(() => expect(screen.getByRole('heading', { name: 'People' })).toHaveFocus());
  });

  it('member_limit (the list was stale): the cap in the sheet, and the list catches up', async () => {
    const six = [TEYMUR, SAM, person(3, 'Alex', 'periwinkle'), person(4, 'Mina', 'lime'), person(5, 'Jo', 'orchid'), person(6, 'Riley', 'sand')];
    let full = false;
    renderSection([TEYMUR, SAM], (call) => {
      if (call.method === 'POST') {
        // Someone else filled it meanwhile
        full = true;
        return error(409, 'member_limit');
      }
      return call.method === 'GET' && full ? answer(200, { items: six }) : undefined;
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Add someone' }));
    const dialog = await sheet('Add someone');
    fireEvent.change(nameField(dialog), { target: { value: 'Kai' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('A household can have up to six people.');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add someone', hidden: true })).toBeDisabled());
  });

  it('removing yourself forgets who’s watching and starts over at For You', async () => {
    renderSection([TEYMUR, SAM]);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    fireEvent.click(within(await sheet('Edit Teymur')).getByRole('button', { name: 'Remove Teymur' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Remove Teymur from the household?' });
    expect(confirm).toHaveTextContent('You’ll continue as Sam.');
    fireEvent.click(within(confirm).getByRole('button', { name: 'Remove Teymur' }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/'));
    expect(document.cookie).not.toContain('momoshtrem_member=1');
  });

  it('removing yourself: a poll meeting unknown_member while the DELETE runs doesn’t reload over it', async () => {
    let finish: (response: Response) => void = () => undefined;
    renderSection([TEYMUR, SAM], (call) => {
      if (call.method === 'DELETE') return new Promise<Response>((resolve) => (finish = resolve));
      // The server stops knowing Teymur as the delete starts
      return call.url === '/api/torrents/summary' ? error(400, 'unknown_member') : undefined;
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    const dialog = await sheet('Edit Teymur');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove Teymur' }));
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove Teymur' }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Remove Teymur' })).toHaveAttribute('aria-busy', 'true'));
    // While it's out, the sheet stays
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled();

    await expect(apiJson('/api/torrents/summary')).rejects.toMatchObject({ code: 'unknown_member' });
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');

    finish(answer(204));
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/'));
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).not.toContain('momoshtrem_member=1');
  });

  it('removing yourself: the list refetched during the DELETE no longer has you, yet nothing reloads, and the 204 goes to For You', async () => {
    let finish: (response: Response) => void = () => undefined;
    let deleting = false;
    renderSection([TEYMUR, SAM], (call) => {
      if (call.method === 'DELETE') {
        deleting = true;
        return new Promise<Response>((resolve) => (finish = resolve));
      }
      // The server stops listing Teymur as the delete starts
      return call.method === 'GET' && call.url === '/api/members' && deleting ? answer(200, { items: [SAM] }) : undefined;
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    const dialog = await sheet('Edit Teymur');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove Teymur' }));
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove Teymur' }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Remove Teymur' })).toHaveAttribute('aria-busy', 'true'));

    // A refetch (focus, or the gate's own) while the DELETE is out: the gate keeps the app, with no stale reload
    await act(() => queryClient.refetchQueries({ queryKey: ['members'] }));
    // The list has rendered without Teymur (his row went)…
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Edit Teymur', hidden: true })).not.toBeInTheDocument());
    // …and the gate with it, holding the app
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');
    expect(screen.getByRole('heading', { name: 'People', hidden: true })).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Remove Teymur' })).toBeInTheDocument();

    finish(answer(204));
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/'));
    expect(reload).not.toHaveBeenCalled();
    expect(document.cookie).not.toContain('momoshtrem_member=1');
  });

  it('removing yourself fails: the page stays, and is no longer leaving', async () => {
    renderSection([TEYMUR, SAM], (call) => (call.method === 'DELETE' ? error(500, 'internal_error') : undefined));
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    const dialog = await sheet('Edit Teymur');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove Teymur' }));
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove Teymur' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Couldn’t remove Teymur. Check your connection and try again.');
    expect(leavingPage()).toBe(false);
    expect(assign).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');
  });

  it('removing yourself fails after a refetch left you out: the list is put back, you stay, and the banner says why', async () => {
    let fail: () => void = () => undefined;
    let phase: 'before' | 'deleting' | 'failed' = 'before';
    // After the failure the fresh list is slow, so the gate first renders with what the cache holds
    let answerFresh: () => void = () => undefined;
    renderSection([TEYMUR, SAM], (call) => {
      if (call.method === 'DELETE') {
        phase = 'deleting';
        return new Promise<Response>((resolve) => {
          fail = () => {
            phase = 'failed';
            resolve(error(504, 'gateway_timeout'));
          };
        });
      }
      if (call.method !== 'GET' || call.url !== '/api/members' || phase === 'before') return undefined;
      // The server leaves Teymur out while the delete runs, and lists him again once it didn't happen
      if (phase === 'deleting') return answer(200, { items: [SAM] });
      return new Promise<Response>((resolve) => (answerFresh = () => resolve(answer(200, { items: [TEYMUR, SAM] }))));
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    const dialog = await sheet('Edit Teymur');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove Teymur' }));
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove Teymur' }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Remove Teymur' })).toHaveAttribute('aria-busy', 'true'));
    await act(() => queryClient.refetchQueries({ queryKey: ['members'] }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Edit Teymur', hidden: true })).not.toBeInTheDocument());

    fail();
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Couldn’t remove Teymur. Check your connection and try again.');
    // The list as the DELETE found it is back before the fresh one answers: Teymur's row, and no stale reload
    expect(await screen.findByRole('button', { name: 'Edit Teymur', hidden: true })).toBeInTheDocument();
    await act(() => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(reload).not.toHaveBeenCalled();
    answerFresh();
    await act(() => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(screen.getByRole('button', { name: 'Edit Teymur', hidden: true })).toBeInTheDocument();
    expect(reload).not.toHaveBeenCalled();
    expect(assign).not.toHaveBeenCalled();
    expect(document.cookie).toContain('momoshtrem_member=1');
    expect(document.cookie).not.toContain('momoshtrem_member=2');
    expect(leavingPage()).toBe(false);
  });

  it('the last person can’t be removed', async () => {
    renderSection([TEYMUR]);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Teymur' }));
    const remove = within(await sheet('Edit Teymur')).getByRole('button', { name: 'Remove Teymur' });
    expect(remove).toBeDisabled();
    expect(remove).toHaveAccessibleDescription('The household needs at least one person.');
  });

  it('a backend from before households says it needs updating', async () => {
    renderSection([], (call) => (call.url === '/api/members' ? error(404, 'not_found') : undefined));
    expect(await screen.findByText('Households need a newer momoshtrem')).toBeInTheDocument();
    expect(screen.getByText('Update the server to add people with their own Trakt and For You.')).toBeInTheDocument();
  });

  it('a list that didn’t load can be tried again', async () => {
    let failing = true;
    renderSection([TEYMUR, SAM], (call) => (call.url === '/api/members' && failing ? error(500, 'internal_error') : undefined));
    const banner = await screen.findByRole('alert');
    expect(banner).toHaveTextContent('Couldn’t load the household');
    failing = false;
    fireEvent.click(within(banner).getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('button', { name: 'Edit Sam' })).toBeInTheDocument();
  });
});
