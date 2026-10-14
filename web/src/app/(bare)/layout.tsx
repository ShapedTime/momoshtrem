import { redirect } from 'next/navigation';
import { getSession } from '@/lib/auth';
import { LOGIN_ROUTE } from '@/config/auth';
import { Providers } from '@/lib/query/Providers';

/** Signed-in pages without the app shell (the offline page). */
export default async function BareLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  if (!session?.authenticated) redirect(LOGIN_ROUTE);
  return (
    <Providers>
      <main id="main" className="main--bare">
        {children}
      </main>
    </Providers>
  );
}
