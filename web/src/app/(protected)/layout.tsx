import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';
import { getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';
import { MembersGate } from '@/components/members/MembersGate';
import { AppShell } from '@/components/shell/AppShell';
import { MEMBER_COOKIE, parseMemberId } from '@/lib/members/cookie';
import { Providers } from '@/lib/query/Providers';

export default async function ProtectedLayout({ children }: { children: React.ReactNode }) {
  // Server-side session check (the proxy already redirects; this is a second layer)
  const session = await getSession();
  if (!session || !session.authenticated) {
    redirect(LOGIN_ROUTE);
  }

  // Who's watching on this device (#239): with a cookie the shell renders now; without one the gate asks first
  const memberId = parseMemberId((await cookies()).get(MEMBER_COOKIE)?.value);

  return (
    <Providers>
      <MembersGate initialMemberId={memberId}>
        <AppShell>{children}</AppShell>
      </MembersGate>
    </Providers>
  );
}
