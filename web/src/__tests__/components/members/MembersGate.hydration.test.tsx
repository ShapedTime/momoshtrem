import { TextEncoder } from 'node:util';
import { act, useState } from 'react';
import { hydrateRoot } from 'react-dom/client';
import { waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MembersGate, useMembers } from '@/components/members/MembersGate';
import { ShellContext } from '@/components/shell/ShellContext';
import { UserMenu } from '@/components/shell/UserMenu';
import { ToastProvider } from '@/components/ui';
import type { Member } from '@/lib/api/contract';

/**
 * #148: the gate's first render matches the server's, with or without a
 * member cookie. Mock-mode e2e can't see this (MockGate renders nothing on
 * the server), so it's checked here: the server's HTML, then hydration with
 * an empty QueryClient while /members answers.
 */

const TWO: Member[] = [
  { id: 1, name: 'Teymur', color: 'teal', created_at: '2026-01-01T00:00:00Z', trakt: { state: 'connected', username: 'teymur' } },
  { id: 2, name: 'Sam', color: 'rose', created_at: '2026-01-01T00:00:00Z', trakt: { state: 'connected', username: 'sam' } },
];

const originalFetch = global.fetch;
const answer = (body: unknown): Response => ({ ok: true, status: 200, json: async () => body, clone: () => answer(body) }) as Response;
const shell = { openPalette: () => undefined, setOverArt: () => undefined };

function WhoAmI() {
  const { current } = useMembers();
  return <p>{current ? `Watching: ${current.name}` : 'Watching: unknown'}</p>;
}

function Tree({ memberId }: { memberId: number | null }) {
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false } } }));
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MembersGate initialMemberId={memberId}>
          <ShellContext.Provider value={shell}>
            <UserMenu />
            <WhoAmI />
          </ShellContext.Provider>
        </MembersGate>
      </ToastProvider>
    </QueryClientProvider>
  );
}

/** jsdom gets react-dom's browser server build, which needs these to load (renderToString itself is synchronous). */
class TimeoutMessageChannel {
  port1: { onmessage: ((event: { data: unknown }) => void) | null; close: () => void } = { onmessage: null, close: () => undefined };
  port2 = { postMessage: (data: unknown) => setTimeout(() => this.port1.onmessage?.({ data })), close: () => undefined };
}

let renderToString: typeof import('react-dom/server').renderToString;
beforeAll(async () => {
  Object.assign(globalThis, { MessageChannel: TimeoutMessageChannel, TextEncoder });
  ({ renderToString } = await import('react-dom/server'));
});

beforeEach(() => {
  global.fetch = jest.fn(async () => answer({ items: TWO })) as typeof fetch;
});
afterEach(() => {
  global.fetch = originalFetch;
  document.body.innerHTML = '';
  document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
});

async function hydrate(memberId: number | null) {
  const container = document.createElement('div');
  container.innerHTML = renderToString(<Tree memberId={memberId} />);
  document.body.appendChild(container);
  const serverHtml = container.innerHTML;
  const onRecoverableError = jest.fn();
  let root: ReturnType<typeof hydrateRoot> | undefined;
  await act(async () => {
    root = hydrateRoot(container, <Tree memberId={memberId} />, { onRecoverableError });
  });
  return { container, serverHtml, onRecoverableError, unmount: () => act(() => root?.unmount()) };
}

describe('MembersGate hydration', () => {
  it('with a cookie: the app on the server, hydrated without a mismatch, then who’s watching', async () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    const { container, serverHtml, onRecoverableError, unmount } = await hydrate(1);
    expect(serverHtml).toContain('Watching: unknown');
    expect(serverHtml).toContain('aria-label="Account menu: settings, log out"');
    expect(onRecoverableError).not.toHaveBeenCalled();
    await waitFor(() => expect(container).toHaveTextContent('Watching: Teymur'));
    expect(container.querySelector('button.avatar-btn')).toHaveAttribute('aria-label', 'Account menu, Teymur: switch, settings, log out');
    unmount();
  });

  it('without one: nothing personal on the server, hydrated without a mismatch, then the picker', async () => {
    const { container, serverHtml, onRecoverableError, unmount } = await hydrate(null);
    expect(serverHtml).not.toContain('Watching:');
    expect(onRecoverableError).not.toHaveBeenCalled();
    await waitFor(() => expect(container.querySelector('h1')).toHaveTextContent('Who’s watching?'));
    unmount();
  });
});
