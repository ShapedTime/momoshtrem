import { redirect } from 'next/navigation';
import { getConfiguredUsername, getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';
import { AppShell } from '@/components/shell/AppShell';
import { Providers } from '@/lib/query/Providers';

export default async function ProtectedLayout({ children }: { children: React.ReactNode }) {
  // Server-side session check (the proxy already redirects; this is a second layer)
  const session = await getSession();
  if (!session || !session.authenticated) {
    redirect(LOGIN_ROUTE);
  }

  return (
    <Providers>
      <AppShell username={getConfiguredUsername()}>{children}</AppShell>
    </Providers>
  );
}
